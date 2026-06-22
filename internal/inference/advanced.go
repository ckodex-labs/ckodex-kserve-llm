/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"fmt"
	"hash/fnv"
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

// NewPrefixCacheWarmer creates a prefix warmer.
func NewPrefixCacheWarmer() *PrefixCacheWarmer {
	return &PrefixCacheWarmer{
		prefixes: make(map[string]*PrefixEntry),
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

// --- Sentinel Errors ---

// ErrLoadShed is returned when a request is dropped due to queue overflow.
var ErrLoadShed = fmt.Errorf("request shed: queue at capacity")

// ErrDeadlineExceeded is returned when a request's deadline expires in queue.
var ErrDeadlineExceeded = fmt.Errorf("request deadline exceeded while queued")
