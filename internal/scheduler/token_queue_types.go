/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"errors"
	"math"
	"time"
)

var (
	// ErrQueueFull indicates the token queue has reached maximum capacity.
	ErrQueueFull = errors.New("token queue capacity exhausted")
	// ErrTenantExhausted indicates a tenant has exceeded its queue capacity or budget.
	ErrTenantExhausted = errors.New("tenant queue capacity or token budget exhausted")
	// ErrRequestExpired indicates the request deadline has already elapsed.
	ErrRequestExpired = errors.New("request deadline already elapsed")
	// ErrRequestNotFound indicates the request was not present in the queue.
	ErrRequestNotFound = errors.New("request not found in queue")
)

// QueueDisposition represents the admission disposition of a request.
type QueueDisposition string

const (
	// DispositionAdmit indicates the request is admitted into the queue.
	DispositionAdmit QueueDisposition = "admit"
	// DispositionBackpressure indicates the caller should back off and retry.
	DispositionBackpressure QueueDisposition = "backpressure"
	// DispositionReject indicates the request is rejected immediately.
	DispositionReject QueueDisposition = "reject"
)

// QueuedRequest represents a token-aware inference request waiting in the queue.
type QueuedRequest struct {
	ID               string
	TenantID         string
	Priority         int           // Base priority (0 to 100)
	PromptTokens     int64         // Input token count
	MaxOutputTokens  int64         // Maximum generation token count
	PredictedTokens  int64         // Estimated total cost (PromptTokens + MaxOutputTokens if 0)
	CacheHitTokens   int64         // Prompt tokens already cached in KV cache
	CacheLocality    float64       // Locality factor [0.0, 1.0] (derived from CacheHitTokens if 0)
	Arrival          time.Time     // Arrival timestamp
	Deadline         time.Time     // Hard deadline (zero time if unset)
	EstimatedLatency time.Duration // Estimated execution duration

	calculatedScore float64
	heapIndex       int
}

// EffectiveCost returns the effective token cost accounting for KV cache reuse.
func (r *QueuedRequest) EffectiveCost() int64 {
	if r.PredictedTokens > 0 {
		return r.PredictedTokens
	}
	uncachedPrompt := r.PromptTokens - r.CacheHitTokens
	if uncachedPrompt < 0 {
		uncachedPrompt = 0
	}
	total := uncachedPrompt + r.MaxOutputTokens
	if total <= 0 {
		return 1
	}
	return total
}

// LocalityScore returns the cache locality fraction in [0.0, 1.0].
func (r *QueuedRequest) LocalityScore() float64 {
	if r.CacheLocality > 0 {
		return math.Min(r.CacheLocality, 1.0)
	}
	if r.PromptTokens <= 0 {
		return 0.0
	}
	ratio := float64(r.CacheHitTokens) / float64(r.PromptTokens)
	return math.Min(ratio, 1.0)
}

// TenantConfig defines quotas and fairness controls for a specific tenant.
type TenantConfig struct {
	TenantID          string
	TokenBudget       int64   // Fair-share token budget threshold
	MaxQueuedRequests int     // Max concurrent queued requests for this tenant
	MaxQueuedTokens   int64   // Max total queued tokens for this tenant
	Weight            float64 // Relative fairness weight (default 1.0)
}

// TenantState tracks live resource consumption for a tenant.
type TenantState struct {
	TenantID       string `json:"tenantID"`
	QueuedRequests int    `json:"queuedRequests"`
	QueuedTokens   int64  `json:"queuedTokens"`
	ActiveTokens   int64  `json:"activeTokens"`
	ServedTokens   int64  `json:"servedTokens"`
}

// TokenQueueConfig defines limits, thresholds, and priority weights for the queue.
type TokenQueueConfig struct {
	MaxQueueDepth           int
	MaxQueueTokens          int64
	SaturationThreshold     float64       // Ratio [0.0, 1.0] triggering backpressure (e.g. 0.8)
	HighPriorityFloor       int           // Priority level exempt from saturation backpressure
	DefaultEstimatedLatency time.Duration // Fallback execution latency estimate
	PriorityWeight          float64
	UrgencyWeight           float64
	LocalityWeight          float64
	FairnessWeight          float64
	CostWeight              float64
	Tenants                 map[string]TenantConfig
	DefaultTenant           TenantConfig
}

// DefaultTokenQueueConfig returns production defaults for the token queue.
func DefaultTokenQueueConfig() TokenQueueConfig {
	return TokenQueueConfig{
		MaxQueueDepth:           1000,
		MaxQueueTokens:          1_000_000,
		SaturationThreshold:     0.80,
		HighPriorityFloor:       80,
		DefaultEstimatedLatency: 500 * time.Millisecond,
		PriorityWeight:          1.0,
		UrgencyWeight:           10.0,
		LocalityWeight:          20.0,
		FairnessWeight:          15.0,
		CostWeight:              5.0,
		Tenants:                 make(map[string]TenantConfig),
		DefaultTenant: TenantConfig{
			TokenBudget:       100_000,
			MaxQueuedRequests: 200,
			MaxQueuedTokens:   200_000,
			Weight:            1.0,
		},
	}
}

// EnqueueResult describes the admission decision and queue state.
type EnqueueResult struct {
	Disposition   QueueDisposition
	Reason        string
	Admitted      bool
	PriorityScore float64
	QueueDepth    int
	QueueTokens   int64
	RetryAfter    time.Duration
}

// QueueMetrics is the vector state snapshot of queue performance and utilization.
type QueueMetrics struct {
	TotalRequestsQueued int64                  `json:"totalRequestsQueued"`
	TotalTokensQueued   int64                  `json:"totalTokensQueued"`
	MaxQueueDepth       int                    `json:"maxQueueDepth"`
	MaxQueueTokens      int64                  `json:"maxQueueTokens"`
	SaturationRatio     float64                `json:"saturationRatio"`
	AdmittedCount       int64                  `json:"admittedCount"`
	BackpressuredCount  int64                  `json:"backpressuredCount"`
	RejectedCount       int64                  `json:"rejectedCount"`
	ExpiredEvictedCount int64                  `json:"expiredEvictedCount"`
	Tenants             map[string]TenantState `json:"tenants"`
}
