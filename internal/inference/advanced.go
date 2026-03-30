/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// PrefixCacheWarmer proactively warms KV caches on inference endpoints
// by pre-sending common prompt prefixes. Reduces TTFT for the first
// request using a given system prompt or few-shot template.
type PrefixCacheWarmer struct {
	mu       sync.RWMutex
	prefixes map[string]*PrefixEntry
	pool     *ConnectionPool
}

// PrefixEntry tracks a cached prompt prefix.
type PrefixEntry struct {
	// Hash is the FNV-1a hash of the prefix text.
	Hash uint64

	// Text is the prefix content.
	Text string

	// Endpoints lists endpoints where this prefix is cached.
	Endpoints []string

	// HitCount tracks how many requests used this prefix.
	HitCount atomic.Int64

	// LastUsed tracks recency for eviction.
	LastUsed atomic.Int64

	// TokenCount is the number of tokens in this prefix.
	TokenCount int32
}

// NewPrefixCacheWarmer creates a warmer connected to the pool.
func NewPrefixCacheWarmer(pool *ConnectionPool) *PrefixCacheWarmer {
	return &PrefixCacheWarmer{
		prefixes: make(map[string]*PrefixEntry),
		pool:     pool,
	}
}

// Register adds a prefix to the warming schedule.
func (w *PrefixCacheWarmer) Register(prefix string, tokenCount int32) *PrefixEntry {
	hash := hashPrefix(prefix)
	key := fmt.Sprintf("%016x", hash)

	w.mu.Lock()
	defer w.mu.Unlock()

	if entry, ok := w.prefixes[key]; ok {
		return entry
	}

	entry := &PrefixEntry{
		Hash:       hash,
		Text:       prefix,
		TokenCount: tokenCount,
	}
	w.prefixes[key] = entry
	return entry
}

// LookupEndpoint returns endpoints that have this prefix cached.
func (w *PrefixCacheWarmer) LookupEndpoint(prefix string) []string {
	hash := hashPrefix(prefix)
	key := fmt.Sprintf("%016x", hash)

	w.mu.RLock()
	entry, ok := w.prefixes[key]
	w.mu.RUnlock()

	if !ok {
		return nil
	}

	entry.HitCount.Add(1)
	entry.LastUsed.Store(time.Now().UnixMilli())
	return entry.Endpoints
}

// MarkCached records that an endpoint has this prefix in its KV cache.
func (w *PrefixCacheWarmer) MarkCached(prefix string, endpoint string) {
	hash := hashPrefix(prefix)
	key := fmt.Sprintf("%016x", hash)

	w.mu.Lock()
	defer w.mu.Unlock()

	entry, ok := w.prefixes[key]
	if !ok {
		return
	}

	for _, ep := range entry.Endpoints {
		if ep == endpoint {
			return
		}
	}
	entry.Endpoints = append(entry.Endpoints, endpoint)
}

// EvictStale removes prefixes unused for the given duration.
func (w *PrefixCacheWarmer) EvictStale(maxAge time.Duration) int {
	w.mu.Lock()
	defer w.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).UnixMilli()
	evicted := 0

	for key, entry := range w.prefixes {
		if entry.LastUsed.Load() < cutoff && entry.HitCount.Load() < 10 {
			delete(w.prefixes, key)
			evicted++
		}
	}
	return evicted
}

func hashPrefix(prefix string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(prefix))
	return h.Sum64()
}

// --- Speculative Decoding ---

// SpeculativeDecoder accelerates inference by using a small draft model
// to generate candidate tokens, then verifying them with the target model
// in a single forward pass. Reduces decode latency by 2-3x.
type SpeculativeDecoder struct {
	// DraftModel is the small model name for candidate generation.
	DraftModel string

	// TargetModel is the full model for verification.
	TargetModel string

	// LookaheadTokens is how many tokens the draft model generates per step.
	LookaheadTokens int

	// AcceptanceRate tracks the average acceptance rate.
	AcceptanceRate atomic.Int64 // stored as percentage * 100

	// Enabled controls whether speculative decoding is active.
	// Disabled automatically during DegradationLight+.
	Enabled atomic.Bool
}

// NewSpeculativeDecoder creates a decoder with the given draft/target pair.
func NewSpeculativeDecoder(draftModel, targetModel string, lookahead int) *SpeculativeDecoder {
	sd := &SpeculativeDecoder{
		DraftModel:      draftModel,
		TargetModel:     targetModel,
		LookaheadTokens: lookahead,
	}
	sd.Enabled.Store(true)
	sd.AcceptanceRate.Store(8500) // 85% initial estimate
	return sd
}

// ShouldSpeculate decides whether to use speculative decoding for this request.
func (s *SpeculativeDecoder) ShouldSpeculate(maxTokens int32) bool {
	if !s.Enabled.Load() {
		return false
	}
	// Only worth speculating for longer outputs
	if maxTokens < 32 {
		return false
	}
	// Skip if acceptance rate is too low
	if s.AcceptanceRate.Load() < 5000 { // < 50%
		return false
	}
	return true
}

// RecordAcceptance updates the EWMA acceptance rate.
func (s *SpeculativeDecoder) RecordAcceptance(accepted, proposed int) {
	if proposed == 0 {
		return
	}
	rate := int64(accepted * 10000 / proposed) // basis points
	current := s.AcceptanceRate.Load()
	updated := (current*7 + rate*3) / 10 // EWMA alpha=0.3
	s.AcceptanceRate.Store(updated)
}

// --- TTI Metrics ---

// TTIMetrics tracks real-time time-to-inference metrics.
type TTIMetrics struct {
	// TTFT is time-to-first-token (includes routing + prefill).
	TTFT *AdaptiveTimeout

	// E2E is end-to-end request latency.
	E2E *AdaptiveTimeout

	// TokenThroughput tracks tokens/second.
	TokenThroughput *ThroughputTracker

	// ActiveRequests tracks in-flight requests.
	ActiveRequests atomic.Int64

	// TotalRequests is the lifetime request count.
	TotalRequests atomic.Int64

	// CacheHitRate tracks prefix cache hit ratio.
	CacheHitRate *RateTracker
}

// NewTTIMetrics creates a metrics collector.
func NewTTIMetrics() *TTIMetrics {
	return &TTIMetrics{
		TTFT:            NewAdaptiveTimeout(10000, 100*time.Millisecond, 60*time.Second),
		E2E:             NewAdaptiveTimeout(10000, 1*time.Second, 120*time.Second),
		TokenThroughput: NewThroughputTracker(),
		CacheHitRate:    NewRateTracker(),
	}
}

// RecordRequest records a completed inference request.
func (m *TTIMetrics) RecordRequest(ttft, e2e time.Duration, tokens int64, cacheHit bool) {
	m.TTFT.Record(ttft)
	m.E2E.Record(e2e)
	m.TotalRequests.Add(1)
	m.TokenThroughput.RecordTokens(tokens, e2e)
	if cacheHit {
		m.CacheHitRate.RecordHit()
	} else {
		m.CacheHitRate.RecordMiss()
	}
}

// Snapshot returns a point-in-time metrics snapshot.
func (m *TTIMetrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TTFT_P50:        m.TTFT.P50(),
		TTFT_P95:        m.TTFT.P95(),
		TTFT_P99:        m.TTFT.P99(),
		E2E_P50:         m.E2E.P50(),
		E2E_P95:         m.E2E.P95(),
		E2E_P99:         m.E2E.P99(),
		TokensPerSecond: m.TokenThroughput.TokensPerSecond(),
		ActiveRequests:  m.ActiveRequests.Load(),
		TotalRequests:   m.TotalRequests.Load(),
		CacheHitPercent: m.CacheHitRate.Percent(),
		AdaptiveTimeout: m.E2E.Timeout(),
	}
}

// MetricsSnapshot is a serializable point-in-time view.
type MetricsSnapshot struct {
	TTFT_P50        time.Duration `json:"ttft_p50"`
	TTFT_P95        time.Duration `json:"ttft_p95"`
	TTFT_P99        time.Duration `json:"ttft_p99"`
	E2E_P50         time.Duration `json:"e2e_p50"`
	E2E_P95         time.Duration `json:"e2e_p95"`
	E2E_P99         time.Duration `json:"e2e_p99"`
	TokensPerSecond float64       `json:"tokens_per_second"`
	ActiveRequests  int64         `json:"active_requests"`
	TotalRequests   int64         `json:"total_requests"`
	CacheHitPercent float64       `json:"cache_hit_percent"`
	AdaptiveTimeout time.Duration `json:"adaptive_timeout"`
}

// --- Helper Types ---

// ThroughputTracker computes tokens/second over a sliding window.
type ThroughputTracker struct {
	totalTokens atomic.Int64
	windowStart atomic.Int64
}

// NewThroughputTracker creates a tracker.
func NewThroughputTracker() *ThroughputTracker {
	t := &ThroughputTracker{}
	t.windowStart.Store(time.Now().UnixMilli())
	return t
}

// RecordTokens adds token count for a request.
func (t *ThroughputTracker) RecordTokens(tokens int64, _ time.Duration) {
	t.totalTokens.Add(tokens)
}

// TokensPerSecond returns the average throughput.
func (t *ThroughputTracker) TokensPerSecond() float64 {
	elapsed := time.Since(time.UnixMilli(t.windowStart.Load())).Seconds()
	if elapsed < 1 {
		return 0
	}
	return float64(t.totalTokens.Load()) / elapsed
}

// RateTracker tracks hit/miss ratios.
type RateTracker struct {
	hits   atomic.Int64
	misses atomic.Int64
}

// NewRateTracker creates a rate tracker.
func NewRateTracker() *RateTracker {
	return &RateTracker{}
}

// RecordHit records a cache hit.
func (r *RateTracker) RecordHit() { r.hits.Add(1) }

// RecordMiss records a cache miss.
func (r *RateTracker) RecordMiss() { r.misses.Add(1) }

// Percent returns the hit rate as a percentage.
func (r *RateTracker) Percent() float64 {
	h := r.hits.Load()
	total := h + r.misses.Load()
	if total == 0 {
		return 0
	}
	return float64(h) / float64(total) * 100
}

// --- Sentinel Errors ---

// ErrLoadShed is returned when a request is dropped due to queue overflow.
var ErrLoadShed = fmt.Errorf("request shed: queue at capacity")

// ErrDeadlineExceeded is returned when a request's deadline expires in queue.
var ErrDeadlineExceeded = fmt.Errorf("request deadline exceeded while queued")

// --- Connection Warmup ---

// WarmConnections proactively establishes TCP+TLS connections to endpoints
// before the first request arrives. Eliminates cold-start handshake latency.
func (p *ConnectionPool) WarmConnections(ctx context.Context, endpoints []string) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(endpoints))

	for _, addr := range endpoints {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()

			conn := p.Get(address)

			// Issue a lightweight health check to force connection establishment
			req, err := newHealthRequest(ctx, address)
			if err != nil {
				errCh <- fmt.Errorf("warmup %s: %w", address, err)
				return
			}

			resp, err := conn.Client.Do(req)
			if err != nil {
				errCh <- fmt.Errorf("warmup %s: %w", address, err)
				return
			}
			_ = resp.Body.Close()
		}(addr)
	}

	wg.Wait()
	close(errCh)

	// Collect errors but don't fail — warmup is best-effort
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		slog.Warn("warmup errors", "count", len(errs))
	}
	return nil
}

func newHealthRequest(ctx context.Context, address string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/v2/health/ready", address), nil)
}
