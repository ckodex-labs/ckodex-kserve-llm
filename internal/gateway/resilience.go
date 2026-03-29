/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package gateway

import (
	"context"
	"fmt"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ResiliencePolicy configures resilience patterns for inference routing.
type ResiliencePolicy struct {
	CircuitBreaker *CircuitBreakerConfig `json:"circuitBreaker,omitempty"`
	Retry          *RetryConfig          `json:"retry,omitempty"`
	Bulkhead       *BulkheadConfig       `json:"bulkhead,omitempty"`
	Hedging        *HedgingConfig        `json:"hedging,omitempty"`
	TimeoutCascade *TimeoutCascadeConfig `json:"timeoutCascade,omitempty"`
	FallbackModel  string                `json:"fallbackModel,omitempty"`
}

// CircuitBreakerConfig defines circuit breaker parameters.
type CircuitBreakerConfig struct {
	FailureThreshold int32         `json:"failureThreshold"` // 5
	SuccessThreshold int32         `json:"successThreshold"` // 3
	Timeout          time.Duration `json:"timeout"`          // 30s
}

// RetryConfig defines retry with exponential backoff + jitter.
type RetryConfig struct {
	MaxRetries  int32         `json:"maxRetries"`  // 3
	BaseBackoff time.Duration `json:"baseBackoff"` // 100ms
	MaxBackoff  time.Duration `json:"maxBackoff"`  // 10s
}

// BulkheadConfig defines per-model concurrency limits.
type BulkheadConfig struct {
	MaxConcurrent int32 `json:"maxConcurrent"` // 100
}

// HedgingConfig defines request hedging for latency-sensitive inference.
type HedgingConfig struct {
	Enabled    bool          `json:"enabled"`
	HedgeDelay time.Duration `json:"hedgeDelay"` // time before sending hedge request
}

// TimeoutCascadeConfig defines client→gateway→inference timeouts.
type TimeoutCascadeConfig struct {
	Client    time.Duration `json:"client"`    // 30s
	Gateway   time.Duration `json:"gateway"`   // 25s
	Inference time.Duration `json:"inference"` // 20s
}

// DefaultResiliencePolicy returns production-grade defaults.
func DefaultResiliencePolicy() ResiliencePolicy {
	return ResiliencePolicy{
		CircuitBreaker: &CircuitBreakerConfig{
			FailureThreshold: 5,
			SuccessThreshold: 3,
			Timeout:          30 * time.Second,
		},
		Retry: &RetryConfig{
			MaxRetries:  3,
			BaseBackoff: 100 * time.Millisecond,
			MaxBackoff:  10 * time.Second,
		},
		Bulkhead: &BulkheadConfig{MaxConcurrent: 100},
		Hedging:  &HedgingConfig{Enabled: false},
		TimeoutCascade: &TimeoutCascadeConfig{
			Client:    30 * time.Second,
			Gateway:   25 * time.Second,
			Inference: 20 * time.Second,
		},
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu              sync.RWMutex
	config          CircuitBreakerConfig
	state           CircuitState
	failures        int32
	successes       int32
	lastFailureTime time.Time
}

// CircuitState represents the circuit breaker state.
type CircuitState int

const (
	StateClosed   CircuitState = iota // normal — requests pass through
	StateOpen                         // tripped — requests fail fast
	StateHalfOpen                     // testing — limited requests pass
)

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{config: config, state: StateClosed}
}

// Allow checks if a request is allowed through the circuit breaker.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.config.Timeout {
			return true // transition to half-open
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.successes++
	if cb.state == StateHalfOpen && cb.successes >= cb.config.SuccessThreshold {
		cb.state = StateClosed
		cb.failures = 0
		cb.successes = 0
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure(ctx context.Context) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()
	if cb.failures >= cb.config.FailureThreshold {
		cb.state = StateOpen
		log.FromContext(ctx).Info("circuit breaker opened",
			"failures", cb.failures,
			"threshold", cb.config.FailureThreshold,
		)
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState { cb.mu.RLock(); defer cb.mu.RUnlock(); return cb.state }

// Bulkhead implements per-model concurrency limiting.
type Bulkhead struct {
	sem chan struct{}
}

// NewBulkhead creates a bulkhead with the given concurrency limit.
func NewBulkhead(maxConcurrent int32) *Bulkhead {
	return &Bulkhead{sem: make(chan struct{}, maxConcurrent)}
}

// Acquire acquires a slot. Returns false if full.
func (b *Bulkhead) Acquire(ctx context.Context) bool {
	select {
	case b.sem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	default:
		return false
	}
}

// Release releases a slot.
func (b *Bulkhead) Release() { <-b.sem }

// ActiveCount returns current active requests.
func (b *Bulkhead) ActiveCount() int { return len(b.sem) }

// ComputeRetryDelay calculates exponential backoff with jitter.
func ComputeRetryDelay(attempt int, config RetryConfig) time.Duration {
	delay := config.BaseBackoff * time.Duration(1<<uint(attempt))
	if delay > config.MaxBackoff {
		delay = config.MaxBackoff
	}
	// Add 25% jitter
	jitter := time.Duration(float64(delay) * 0.25)
	return delay + jitter
}

// FallbackRouter routes to a fallback model on primary failure.
type FallbackRouter struct {
	PrimaryModel  string
	FallbackModel string
}

// ShouldFallback determines if traffic should be routed to fallback.
func (f *FallbackRouter) ShouldFallback(primaryHealthy bool) bool {
	return !primaryHealthy && f.FallbackModel != ""
}

// TargetModel returns the model to route to.
func (f *FallbackRouter) TargetModel(primaryHealthy bool) string {
	if f.ShouldFallback(primaryHealthy) {
		return f.FallbackModel
	}
	return f.PrimaryModel
}

// HealthDrainConfig configures gradual traffic shift.
type HealthDrainConfig struct {
	Enabled          bool
	DrainRatePercent int // % of traffic to shift per interval
	Interval         time.Duration
}

// DefaultHealthDrainConfig returns default drain config.
func DefaultHealthDrainConfig() HealthDrainConfig {
	return HealthDrainConfig{
		Enabled:          true,
		DrainRatePercent: 10,
		Interval:         30 * time.Second,
	}
}

func init() {
	_ = fmt.Sprintf // keep import
}
