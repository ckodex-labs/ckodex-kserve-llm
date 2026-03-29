/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ---- DefaultResiliencePolicy -----------------------------------------------

func TestDefaultResiliencePolicy_CircuitBreaker(t *testing.T) {
	p := DefaultResiliencePolicy()
	require.NotNil(t, p.CircuitBreaker)
	assert.Equal(t, int32(5), p.CircuitBreaker.FailureThreshold)
	assert.Equal(t, int32(3), p.CircuitBreaker.SuccessThreshold)
	assert.Equal(t, 30*time.Second, p.CircuitBreaker.Timeout)
}

func TestDefaultResiliencePolicy_Retry(t *testing.T) {
	p := DefaultResiliencePolicy()
	require.NotNil(t, p.Retry)
	assert.Equal(t, int32(3), p.Retry.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, p.Retry.BaseBackoff)
	assert.Equal(t, 10*time.Second, p.Retry.MaxBackoff)
}

func TestDefaultResiliencePolicy_Bulkhead(t *testing.T) {
	p := DefaultResiliencePolicy()
	require.NotNil(t, p.Bulkhead)
	assert.Equal(t, int32(100), p.Bulkhead.MaxConcurrent)
}

func TestDefaultResiliencePolicy_HedgingDisabledByDefault(t *testing.T) {
	p := DefaultResiliencePolicy()
	require.NotNil(t, p.Hedging)
	assert.False(t, p.Hedging.Enabled, "hedging must default to disabled — opt-in only")
}

func TestDefaultResiliencePolicy_TimeoutCascade(t *testing.T) {
	p := DefaultResiliencePolicy()
	require.NotNil(t, p.TimeoutCascade)
	// Client > Gateway > Inference provides natural cascade headroom.
	assert.Equal(t, 30*time.Second, p.TimeoutCascade.Client)
	assert.Equal(t, 25*time.Second, p.TimeoutCascade.Gateway)
	assert.Equal(t, 20*time.Second, p.TimeoutCascade.Inference)
	assert.Greater(t, p.TimeoutCascade.Client, p.TimeoutCascade.Gateway)
	assert.Greater(t, p.TimeoutCascade.Gateway, p.TimeoutCascade.Inference)
}

// ---- CircuitBreaker — initial state ----------------------------------------

func TestNewCircuitBreaker_StartsInClosedState(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 5, SuccessThreshold: 3, Timeout: 30 * time.Second})
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_Allow_Closed(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 5, SuccessThreshold: 3, Timeout: 30 * time.Second})
	assert.True(t, cb.Allow(), "closed circuit must allow requests")
}

// ---- CircuitBreaker — trip to open -----------------------------------------

func TestCircuitBreaker_TripOpen_OnFailureThreshold(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, SuccessThreshold: 2, Timeout: 30 * time.Second})
	ctx := context.Background()

	cb.RecordFailure(ctx)
	assert.Equal(t, StateClosed, cb.State(), "still closed after 1 failure")

	cb.RecordFailure(ctx)
	assert.Equal(t, StateClosed, cb.State(), "still closed after 2 failures")

	cb.RecordFailure(ctx)
	assert.Equal(t, StateOpen, cb.State(), "must open after reaching threshold")
}

func TestCircuitBreaker_Open_BlocksRequests(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, SuccessThreshold: 1, Timeout: 30 * time.Second})
	ctx := context.Background()

	cb.RecordFailure(ctx)
	require.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow(), "open circuit must block requests")
}

// ---- CircuitBreaker — half-open on timeout ---------------------------------

func TestCircuitBreaker_Open_AllowsAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, SuccessThreshold: 1, Timeout: 1 * time.Millisecond})
	ctx := context.Background()

	cb.RecordFailure(ctx)
	require.Equal(t, StateOpen, cb.State())

	time.Sleep(5 * time.Millisecond) // well past 1ms timeout
	assert.True(t, cb.Allow(), "circuit must allow probe after timeout elapses (half-open probe)")
}

// ---- CircuitBreaker — recovery (half-open → closed) ------------------------

func TestCircuitBreaker_RecoverToClosedOnSuccessThreshold(t *testing.T) {
	// Allow() signals half-open readiness but leaves state at StateOpen;
	// RecordSuccess closes only when state is explicitly StateHalfOpen.
	// Directly set StateHalfOpen (white-box) to test the close transition.
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, SuccessThreshold: 2, Timeout: 30 * time.Second})
	cb.mu.Lock()
	cb.state = StateHalfOpen
	cb.mu.Unlock()

	cb.RecordSuccess()
	assert.Equal(t, StateHalfOpen, cb.State(), "one success below threshold — still half-open")

	cb.RecordSuccess()
	assert.Equal(t, StateClosed, cb.State(), "must close after success threshold")
}

func TestCircuitBreaker_RecordSuccess_InClosed_NoStateChange(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 5, SuccessThreshold: 3, Timeout: 30 * time.Second})
	cb.RecordSuccess()
	assert.Equal(t, StateClosed, cb.State(), "success in closed state is a no-op for state")
}

// ---- CircuitBreaker — concurrent safety ------------------------------------

func TestCircuitBreaker_ConcurrentAccess_NoDataRace(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 50, SuccessThreshold: 10, Timeout: 1 * time.Second})
	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); cb.Allow() }()
		go func() { defer wg.Done(); cb.RecordSuccess() }()
		go func() { defer wg.Done(); cb.RecordFailure(ctx) }()
	}
	wg.Wait()
	// Just asserting state is accessible without panic or deadlock.
	_ = cb.State()
}

// ---- Bulkhead — basic acquire/release --------------------------------------

func TestNewBulkhead_AllowsUpToMaxConcurrent(t *testing.T) {
	b := NewBulkhead(3)
	ctx := context.Background()

	assert.True(t, b.Acquire(ctx))
	assert.True(t, b.Acquire(ctx))
	assert.True(t, b.Acquire(ctx))
	assert.Equal(t, 3, b.ActiveCount())
}

func TestBulkhead_Acquire_RejectsWhenFull(t *testing.T) {
	b := NewBulkhead(2)
	ctx := context.Background()

	require.True(t, b.Acquire(ctx))
	require.True(t, b.Acquire(ctx))

	// Channel is full — default case triggers immediately.
	assert.False(t, b.Acquire(ctx), "must reject when at capacity")
}

func TestBulkhead_Release_FreesSlot(t *testing.T) {
	b := NewBulkhead(1)
	ctx := context.Background()

	require.True(t, b.Acquire(ctx))
	assert.Equal(t, 1, b.ActiveCount())

	b.Release()
	assert.Equal(t, 0, b.ActiveCount())

	// Slot freed — next acquire should succeed.
	assert.True(t, b.Acquire(ctx))
}

func TestBulkhead_ActiveCount_StartsAtZero(t *testing.T) {
	b := NewBulkhead(10)
	assert.Equal(t, 0, b.ActiveCount())
}

func TestBulkhead_Acquire_CancelledContext_Fails(t *testing.T) {
	b := NewBulkhead(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	// With a full bulkhead, cancelled ctx triggers the ctx.Done() branch.
	require.True(t, b.Acquire(context.Background())) // fill it
	assert.False(t, b.Acquire(ctx), "cancelled context must not acquire a slot")
}

// ---- ComputeRetryDelay -------------------------------------------------------

func TestComputeRetryDelay_Attempt0_BaseBackoff(t *testing.T) {
	cfg := RetryConfig{BaseBackoff: 100 * time.Millisecond, MaxBackoff: 10 * time.Second}
	// attempt=0: delay = 100ms * 1 = 100ms, jitter = 25ms → 125ms
	d := ComputeRetryDelay(0, cfg)
	assert.Equal(t, 125*time.Millisecond, d)
}

func TestComputeRetryDelay_Attempt1_DoubledBackoff(t *testing.T) {
	cfg := RetryConfig{BaseBackoff: 100 * time.Millisecond, MaxBackoff: 10 * time.Second}
	// attempt=1: delay = 100ms * 2 = 200ms, jitter = 50ms → 250ms
	d := ComputeRetryDelay(1, cfg)
	assert.Equal(t, 250*time.Millisecond, d)
}

func TestComputeRetryDelay_Attempt2_QuadrupleBackoff(t *testing.T) {
	cfg := RetryConfig{BaseBackoff: 100 * time.Millisecond, MaxBackoff: 10 * time.Second}
	// attempt=2: delay = 100ms * 4 = 400ms, jitter = 100ms → 500ms
	d := ComputeRetryDelay(2, cfg)
	assert.Equal(t, 500*time.Millisecond, d)
}

func TestComputeRetryDelay_CapsAtMaxBackoff(t *testing.T) {
	cfg := RetryConfig{BaseBackoff: 100 * time.Millisecond, MaxBackoff: 200 * time.Millisecond}
	// attempt=10: 100ms * 1024 = 102400ms → clamped to 200ms, jitter = 50ms → 250ms
	d := ComputeRetryDelay(10, cfg)
	expected := 200*time.Millisecond + time.Duration(float64(200*time.Millisecond)*0.25)
	assert.Equal(t, expected, d)
}

func TestComputeRetryDelay_AlwaysExceedsBase(t *testing.T) {
	cfg := RetryConfig{BaseBackoff: 50 * time.Millisecond, MaxBackoff: 5 * time.Second}
	for attempt := 0; attempt < 5; attempt++ {
		d := ComputeRetryDelay(attempt, cfg)
		assert.Greater(t, d, cfg.BaseBackoff, "jitter ensures delay > base backoff")
	}
}

// ---- FallbackRouter ---------------------------------------------------------

func TestFallbackRouter_ShouldFallback_WhenPrimaryUnhealthy(t *testing.T) {
	r := &FallbackRouter{PrimaryModel: "primary", FallbackModel: "fallback"}
	assert.True(t, r.ShouldFallback(false))
}

func TestFallbackRouter_ShouldNotFallback_WhenPrimaryHealthy(t *testing.T) {
	r := &FallbackRouter{PrimaryModel: "primary", FallbackModel: "fallback"}
	assert.False(t, r.ShouldFallback(true))
}

func TestFallbackRouter_ShouldNotFallback_WhenNoFallbackConfigured(t *testing.T) {
	r := &FallbackRouter{PrimaryModel: "primary", FallbackModel: ""}
	assert.False(t, r.ShouldFallback(false), "must not fallback when FallbackModel is empty")
}

func TestFallbackRouter_TargetModel_Primary_WhenHealthy(t *testing.T) {
	r := &FallbackRouter{PrimaryModel: "primary", FallbackModel: "fallback"}
	assert.Equal(t, "primary", r.TargetModel(true))
}

func TestFallbackRouter_TargetModel_Fallback_WhenUnhealthy(t *testing.T) {
	r := &FallbackRouter{PrimaryModel: "primary", FallbackModel: "fallback"}
	assert.Equal(t, "fallback", r.TargetModel(false))
}

func TestFallbackRouter_TargetModel_Primary_WhenUnhealthyButNoFallback(t *testing.T) {
	r := &FallbackRouter{PrimaryModel: "primary", FallbackModel: ""}
	assert.Equal(t, "primary", r.TargetModel(false))
}

// ---- DefaultHealthDrainConfig -----------------------------------------------

func TestDefaultHealthDrainConfig_Enabled(t *testing.T) {
	cfg := DefaultHealthDrainConfig()
	assert.True(t, cfg.Enabled)
}

func TestDefaultHealthDrainConfig_DrainRate(t *testing.T) {
	cfg := DefaultHealthDrainConfig()
	assert.Equal(t, 10, cfg.DrainRatePercent, "conservative 10%/interval prevents traffic cliff")
}

func TestDefaultHealthDrainConfig_Interval(t *testing.T) {
	cfg := DefaultHealthDrainConfig()
	assert.Equal(t, 30*time.Second, cfg.Interval)
}

// ---- EnvoyAIGatewayReconciler — DefaultAIGatewayConfig ---------------------

func TestDefaultAIGatewayConfig_DisabledByDefault(t *testing.T) {
	cfg := DefaultAIGatewayConfig()
	assert.False(t, cfg.Enabled, "AI gateway must default to disabled — explicit opt-in")
}

func TestDefaultAIGatewayConfig_TokenBudget(t *testing.T) {
	cfg := DefaultAIGatewayConfig()
	assert.Equal(t, int64(100000), cfg.DefaultTokenBudget)
}

func TestDefaultAIGatewayConfig_UserHeader(t *testing.T) {
	cfg := DefaultAIGatewayConfig()
	assert.Equal(t, "x-user-id", cfg.UserHeaderName)
}

// ---- EnvoyAIGatewayReconciler — ReconcileRateLimiting ----------------------

func TestReconcileRateLimiting_DisabledConfig_NoOp(t *testing.T) {
	r := &EnvoyAIGatewayReconciler{Config: DefaultAIGatewayConfig()} // Enabled=false
	llmSvc := makeGatewayLLMSvc("model-a", "default")
	err := r.ReconcileRateLimiting(context.Background(), llmSvc)
	assert.NoError(t, err, "disabled config must return nil without any side effects")
}

func TestReconcileRateLimiting_EnabledConfig_NoError(t *testing.T) {
	r := &EnvoyAIGatewayReconciler{
		Config: AIGatewayConfig{
			Enabled:            true,
			DefaultTokenBudget: 50000,
			UserHeaderName:     "x-tenant",
		},
	}
	llmSvc := makeGatewayLLMSvc("model-b", "prod")
	err := r.ReconcileRateLimiting(context.Background(), llmSvc)
	// ReconcileRateLimiting is a no-op stub (CRD types pending) — must not error.
	assert.NoError(t, err)
}

// ---- helpers ----------------------------------------------------------------

func makeGatewayLLMSvc(name, namespace string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{Name: name, URI: "hf://test/" + name},
		},
	}
}
