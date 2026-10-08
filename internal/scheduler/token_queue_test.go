/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenQueue_BasicAdmitAndDequeue(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	q := NewTokenQueue(cfg)

	req := &QueuedRequest{
		ID:              "req-1",
		TenantID:        "tenant-a",
		Priority:        50,
		PromptTokens:    100,
		MaxOutputTokens: 50,
	}

	result := q.Enqueue(req)
	assert.True(t, result.Admitted)
	assert.Equal(t, DispositionAdmit, result.Disposition)
	assert.Equal(t, 1, q.Len())
	assert.Equal(t, int64(150), q.TotalTokens())

	dequeued := q.Dequeue()
	require.NotNil(t, dequeued)
	assert.Equal(t, "req-1", dequeued.ID)
	assert.Equal(t, 0, q.Len())
	assert.Equal(t, int64(0), q.TotalTokens())

	snap := q.Snapshot()
	assert.Equal(t, int64(150), snap.Tenants["tenant-a"].ActiveTokens)

	q.Finish("tenant-a", 150, 120)
	snap = q.Snapshot()
	assert.Equal(t, int64(0), snap.Tenants["tenant-a"].ActiveTokens)
	assert.Equal(t, int64(120), snap.Tenants["tenant-a"].ServedTokens)
}

func TestTokenQueue_PriorityOrder_BasePriority(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	q := NewTokenQueue(cfg)

	rLow := &QueuedRequest{ID: "low", TenantID: "t1", Priority: 20, PromptTokens: 100, MaxOutputTokens: 50}
	rHigh := &QueuedRequest{ID: "high", TenantID: "t1", Priority: 90, PromptTokens: 100, MaxOutputTokens: 50}
	rMed := &QueuedRequest{ID: "med", TenantID: "t1", Priority: 50, PromptTokens: 100, MaxOutputTokens: 50}

	q.Enqueue(rLow)
	q.Enqueue(rHigh)
	q.Enqueue(rMed)

	assert.Equal(t, "high", q.Dequeue().ID)
	assert.Equal(t, "med", q.Dequeue().ID)
	assert.Equal(t, "low", q.Dequeue().ID)
}

func TestTokenQueue_PriorityOrder_DeadlineUrgency(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	q := NewTokenQueue(cfg)
	now := time.Now()

	rUrgent := &QueuedRequest{
		ID:               "urgent",
		TenantID:         "t1",
		Priority:         50,
		PromptTokens:     100,
		MaxOutputTokens:  50,
		Deadline:         now.Add(200 * time.Millisecond),
		EstimatedLatency: 100 * time.Millisecond,
	}
	rRelaxed := &QueuedRequest{
		ID:               "relaxed",
		TenantID:         "t1",
		Priority:         50,
		PromptTokens:     100,
		MaxOutputTokens:  50,
		Deadline:         now.Add(10 * time.Second),
		EstimatedLatency: 100 * time.Millisecond,
	}

	q.Enqueue(rRelaxed)
	q.Enqueue(rUrgent)

	assert.Equal(t, "urgent", q.Dequeue().ID)
	assert.Equal(t, "relaxed", q.Dequeue().ID)
}

func TestTokenQueue_PriorityOrder_CacheLocality(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	q := NewTokenQueue(cfg)

	rNoCache := &QueuedRequest{
		ID:              "no-cache",
		TenantID:        "t1",
		Priority:        50,
		PromptTokens:    500,
		MaxOutputTokens: 100,
		CacheHitTokens:  0,
	}
	rCached := &QueuedRequest{
		ID:              "cached",
		TenantID:        "t1",
		Priority:        50,
		PromptTokens:    500,
		MaxOutputTokens: 100,
		CacheHitTokens:  400,
	}

	q.Enqueue(rNoCache)
	q.Enqueue(rCached)

	first := q.Dequeue()
	assert.Equal(t, "cached", first.ID, "request with KV cache locality bonus should dequeue first")
	second := q.Dequeue()
	assert.Equal(t, "no-cache", second.ID)
}

func TestTokenQueue_PriorityOrder_PredictedCost(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	q := NewTokenQueue(cfg)

	rLarge := &QueuedRequest{ID: "large", TenantID: "t1", Priority: 50, PromptTokens: 4000, MaxOutputTokens: 1000}
	rSmall := &QueuedRequest{ID: "small", TenantID: "t1", Priority: 50, PromptTokens: 50, MaxOutputTokens: 20}

	q.Enqueue(rLarge)
	q.Enqueue(rSmall)

	assert.Equal(t, "small", q.Dequeue().ID, "smaller predicted cost should be favored under identical priority")
	assert.Equal(t, "large", q.Dequeue().ID)
}

func TestTokenQueue_TenantFairness_OverBudgetPenalty(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	cfg.Tenants["heavy"] = TenantConfig{
		TenantID:          "heavy",
		TokenBudget:       500,
		MaxQueuedRequests: 10,
		MaxQueuedTokens:   5000,
		Weight:            1.0,
	}
	cfg.Tenants["light"] = TenantConfig{
		TenantID:          "light",
		TokenBudget:       500,
		MaxQueuedRequests: 10,
		MaxQueuedTokens:   5000,
		Weight:            1.0,
	}
	q := NewTokenQueue(cfg)

	// Heavy tenant enqueues requests consuming 1200 tokens (over 500 budget)
	q.Enqueue(&QueuedRequest{ID: "h-1", TenantID: "heavy", Priority: 50, PromptTokens: 600, MaxOutputTokens: 0})
	q.Enqueue(&QueuedRequest{ID: "h-2", TenantID: "heavy", Priority: 50, PromptTokens: 600, MaxOutputTokens: 0})

	// Light tenant arrives with same priority but within budget
	q.Enqueue(&QueuedRequest{ID: "l-1", TenantID: "light", Priority: 50, PromptTokens: 200, MaxOutputTokens: 0})

	// Dequeue order: Light tenant should get scheduled ahead of over-budget heavy tenant requests
	popped := q.Dequeue()
	assert.Equal(t, "l-1", popped.ID, "under-budget tenant must be prioritized over penalized heavy tenant")
}

func TestTokenQueue_Backpressure_SaturationThreshold(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	cfg.MaxQueueTokens = 1000
	cfg.SaturationThreshold = 0.80 // 800 tokens is saturation line
	cfg.HighPriorityFloor = 80
	cfg.Tenants["t1"] = TenantConfig{TenantID: "t1", TokenBudget: 300, MaxQueuedTokens: 1000, MaxQueuedRequests: 20}

	q := NewTokenQueue(cfg)

	// Fill queue to 850 tokens (over 80% saturation)
	res1 := q.Enqueue(&QueuedRequest{ID: "r1", TenantID: "t1", Priority: 50, PromptTokens: 850, MaxOutputTokens: 0})
	require.True(t, res1.Admitted)

	// Normal priority request from over-budget tenant is backpressured
	res2 := q.Enqueue(&QueuedRequest{ID: "r2", TenantID: "t1", Priority: 50, PromptTokens: 50, MaxOutputTokens: 0})
	assert.False(t, res2.Admitted)
	assert.Equal(t, DispositionBackpressure, res2.Disposition)
	assert.Equal(t, "queue-saturated-tenant-over-budget", res2.Reason)
	assert.Greater(t, res2.RetryAfter, time.Duration(0))

	// High priority request is admitted despite saturation
	resHigh := q.Enqueue(&QueuedRequest{ID: "high-prio", TenantID: "t1", Priority: 85, PromptTokens: 50, MaxOutputTokens: 0})
	assert.True(t, resHigh.Admitted)
	assert.Equal(t, DispositionAdmit, resHigh.Disposition)
}

func TestTokenQueue_Backpressure_TenantCapacityExhausted(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	cfg.Tenants["t-isolated"] = TenantConfig{
		TenantID:          "t-isolated",
		MaxQueuedRequests: 2,
		MaxQueuedTokens:   500,
	}
	q := NewTokenQueue(cfg)

	res1 := q.Enqueue(&QueuedRequest{ID: "iso-1", TenantID: "t-isolated", Priority: 50, PromptTokens: 100})
	assert.True(t, res1.Admitted)
	res2 := q.Enqueue(&QueuedRequest{ID: "iso-2", TenantID: "t-isolated", Priority: 50, PromptTokens: 100})
	assert.True(t, res2.Admitted)

	// Third request breaches tenant max queued requests
	res3 := q.Enqueue(&QueuedRequest{ID: "iso-3", TenantID: "t-isolated", Priority: 50, PromptTokens: 100})
	assert.False(t, res3.Admitted)
	assert.Equal(t, DispositionReject, res3.Disposition)
	assert.Equal(t, "tenant-queue-depth-exhausted", res3.Reason)

	// Another tenant is unaffected
	resOther := q.Enqueue(&QueuedRequest{ID: "other-1", TenantID: "other", Priority: 50, PromptTokens: 100})
	assert.True(t, resOther.Admitted)
}

func TestTokenQueue_HardCapacityRejection(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	cfg.MaxQueueDepth = 2
	cfg.MaxQueueTokens = 500
	q := NewTokenQueue(cfg)

	res1 := q.Enqueue(&QueuedRequest{ID: "q1", TenantID: "t1", Priority: 50, PromptTokens: 200})
	assert.True(t, res1.Admitted)
	res2 := q.Enqueue(&QueuedRequest{ID: "q2", TenantID: "t1", Priority: 50, PromptTokens: 200})
	assert.True(t, res2.Admitted)

	// Exceeds MaxQueueDepth
	res3 := q.Enqueue(&QueuedRequest{ID: "q3", TenantID: "t1", Priority: 99, PromptTokens: 50})
	assert.False(t, res3.Admitted)
	assert.Equal(t, DispositionReject, res3.Disposition)
	assert.Equal(t, "queue-depth-capacity-exhausted", res3.Reason)
}

func TestTokenQueue_DeadlineExpired_RejectionAndEviction(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	q := NewTokenQueue(cfg)
	past := time.Now().Add(-1 * time.Second)

	// Enqueue already expired request
	res := q.Enqueue(&QueuedRequest{ID: "past", TenantID: "t1", Priority: 50, PromptTokens: 100, Deadline: past})
	assert.False(t, res.Admitted)
	assert.Equal(t, "request-deadline-expired", res.Reason)

	// Enqueue request expiring soon
	shortDeadline := time.Now().Add(50 * time.Millisecond)
	resShort := q.Enqueue(&QueuedRequest{
		ID:           "expires-in-queue",
		TenantID:     "t1",
		Priority:     100,
		PromptTokens: 100,
		Deadline:     shortDeadline,
	})
	assert.True(t, resShort.Admitted)

	time.Sleep(70 * time.Millisecond)

	// Evict expired explicitly
	evicted := q.EvictExpired(time.Now())
	assert.Equal(t, 1, evicted)
	assert.Equal(t, 0, q.Len())
	assert.Equal(t, int64(0), q.TotalTokens())
}

func TestTokenQueue_RemoveAndPeek(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	q := NewTokenQueue(cfg)

	r1 := &QueuedRequest{ID: "r1", TenantID: "t1", Priority: 40, PromptTokens: 100}
	r2 := &QueuedRequest{ID: "r2", TenantID: "t1", Priority: 80, PromptTokens: 100}
	q.Enqueue(r1)
	q.Enqueue(r2)

	assert.Equal(t, "r2", q.Peek().ID)
	assert.True(t, q.Remove("r2"))
	assert.False(t, q.Remove("r2"))
	assert.Equal(t, "r1", q.Peek().ID)
}

func TestTokenQueue_ConcurrentAccess(t *testing.T) {
	cfg := DefaultTokenQueueConfig()
	cfg.MaxQueueDepth = 5000
	cfg.MaxQueueTokens = 10_000_000
	q := NewTokenQueue(cfg)

	var wg sync.WaitGroup
	workers := 10
	opsPerWorker := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				reqID := string(rune('A'+workerID)) + "-" + string(rune('0'+j))
				res := q.Enqueue(&QueuedRequest{
					ID:              reqID,
					TenantID:        "t-concurrent",
					Priority:        (j % 100),
					PromptTokens:    50,
					MaxOutputTokens: 20,
				})
				if res.Admitted {
					popped := q.Dequeue()
					if popped != nil {
						q.Finish(popped.TenantID, popped.EffectiveCost(), 70)
					}
				}
			}
		}(i)
	}

	wg.Wait()
	snap := q.Snapshot()
	assert.Greater(t, snap.AdmittedCount, int64(0))
}
