/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"sync/atomic"
	"time"
)

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
