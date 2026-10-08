/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"container/heap"
	"math"
	"sync"
	"time"
)

type requestHeap []*QueuedRequest

func (h requestHeap) Len() int           { return len(h) }
func (h requestHeap) Less(i, j int) bool { return h[i].calculatedScore > h[j].calculatedScore }
func (h requestHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}
func (h *requestHeap) Push(x any) {
	n := len(*h)
	item := x.(*QueuedRequest)
	item.heapIndex = n
	*h = append(*h, item)
}
func (h *requestHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.heapIndex = -1
	*h = old[0 : n-1]
	return item
}

// TokenQueue coordinates token-aware admission, tenant fairness, and backpressure.
type TokenQueue struct {
	mu          sync.Mutex
	cfg         TokenQueueConfig
	heap        requestHeap
	lookup      map[string]*QueuedRequest
	tenants     map[string]*TenantState
	totalTokens int64

	admittedCount       int64
	backpressuredCount  int64
	rejectedCount       int64
	expiredEvictedCount int64
}

// NewTokenQueue initializes a thread-safe token queue with the provided configuration.
func NewTokenQueue(cfg TokenQueueConfig) *TokenQueue {
	if cfg.MaxQueueDepth <= 0 {
		cfg.MaxQueueDepth = 1000
	}
	if cfg.MaxQueueTokens <= 0 {
		cfg.MaxQueueTokens = 1_000_000
	}
	if cfg.SaturationThreshold <= 0 || cfg.SaturationThreshold > 1.0 {
		cfg.SaturationThreshold = 0.80
	}
	if cfg.DefaultEstimatedLatency <= 0 {
		cfg.DefaultEstimatedLatency = 500 * time.Millisecond
	}
	return &TokenQueue{
		cfg:     cfg,
		heap:    make(requestHeap, 0),
		lookup:  make(map[string]*QueuedRequest),
		tenants: make(map[string]*TenantState),
	}
}

// calculatePriority computes the composite priority score for a queued request.
// Score = BasePriority + Urgency + CacheLocality + SJF_Cost - FairnessPenalty.
func (q *TokenQueue) calculatePriority(req *QueuedRequest, now time.Time) float64 {
	baseScore := float64(req.Priority) * q.cfg.PriorityWeight
	urgencyScore := q.calculateUrgency(req, now)
	localityScore := req.LocalityScore() * q.cfg.LocalityWeight
	cost := float64(req.EffectiveCost())
	costScore := q.cfg.CostWeight / (1.0 + (cost / 1000.0))
	fairnessPenalty := q.calculateFairnessPenalty(req.TenantID)

	return baseScore + urgencyScore + localityScore + costScore - fairnessPenalty
}

func (q *TokenQueue) calculateUrgency(req *QueuedRequest, now time.Time) float64 {
	if req.Deadline.IsZero() {
		return 0.0
	}
	latency := req.EstimatedLatency
	if latency <= 0 {
		latency = q.cfg.DefaultEstimatedLatency
	}
	slack := req.Deadline.Sub(now) - latency
	if slack <= 0 {
		return q.cfg.UrgencyWeight * 10.0
	}
	return q.cfg.UrgencyWeight / (1.0 + slack.Seconds())
}

func (q *TokenQueue) calculateFairnessPenalty(tenantID string) float64 {
	state, exists := q.tenants[tenantID]
	if !exists {
		return 0.0
	}
	cfg := q.getTenantConfig(tenantID)
	if cfg.TokenBudget <= 0 {
		return 0.0
	}
	active := float64(state.QueuedTokens + state.ActiveTokens)
	budget := float64(cfg.TokenBudget) * math.Max(cfg.Weight, 0.1)
	if active <= budget {
		return 0.0
	}
	ratio := (active - budget) / budget
	return ratio * q.cfg.FairnessWeight * 10.0
}

func (q *TokenQueue) getTenantConfig(tenantID string) TenantConfig {
	if cfg, ok := q.cfg.Tenants[tenantID]; ok {
		return cfg
	}
	cfg := q.cfg.DefaultTenant
	cfg.TenantID = tenantID
	return cfg
}

func (q *TokenQueue) getOrCreateTenantState(tenantID string) *TenantState {
	state, ok := q.tenants[tenantID]
	if !ok {
		state = &TenantState{TenantID: tenantID}
		q.tenants[tenantID] = state
	}
	return state
}

// Enqueue evaluates admission, calculates priority, and places viable requests into the queue.
func (q *TokenQueue) Enqueue(req *QueuedRequest) EnqueueResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	cost := req.EffectiveCost()
	tenantCfg := q.getTenantConfig(req.TenantID)
	tenantState := q.getOrCreateTenantState(req.TenantID)

	disposition, reason, retryAfter := q.evaluateAdmission(req, tenantCfg, tenantState, cost, now)
	if disposition != DispositionAdmit {
		if disposition == DispositionBackpressure {
			q.backpressuredCount++
		} else {
			q.rejectedCount++
		}
		return EnqueueResult{
			Disposition: disposition,
			Reason:      reason,
			Admitted:    false,
			QueueDepth:  len(q.heap),
			QueueTokens: q.totalTokens,
			RetryAfter:  retryAfter,
		}
	}

	score := q.calculatePriority(req, now)
	req.calculatedScore = score
	if req.Arrival.IsZero() {
		req.Arrival = now
	}

	heap.Push(&q.heap, req)
	q.lookup[req.ID] = req
	q.totalTokens += cost
	tenantState.QueuedRequests++
	tenantState.QueuedTokens += cost
	q.admittedCount++

	return EnqueueResult{
		Disposition:   DispositionAdmit,
		Reason:        "admitted",
		Admitted:      true,
		PriorityScore: score,
		QueueDepth:    len(q.heap),
		QueueTokens:   q.totalTokens,
	}
}

func (q *TokenQueue) evaluateAdmission(
	req *QueuedRequest,
	tCfg TenantConfig,
	tState *TenantState,
	cost int64,
	now time.Time,
) (QueueDisposition, string, time.Duration) {
	if !req.Deadline.IsZero() && now.After(req.Deadline) {
		return DispositionReject, "request-deadline-expired", 0
	}
	if len(q.heap)+1 > q.cfg.MaxQueueDepth {
		return DispositionReject, "queue-depth-capacity-exhausted", 0
	}
	if q.totalTokens+cost > q.cfg.MaxQueueTokens {
		return DispositionReject, "queue-token-capacity-exhausted", 0
	}
	if tCfg.MaxQueuedRequests > 0 && tState.QueuedRequests+1 > tCfg.MaxQueuedRequests {
		return DispositionReject, "tenant-queue-depth-exhausted", 0
	}
	if tCfg.MaxQueuedTokens > 0 && tState.QueuedTokens+cost > tCfg.MaxQueuedTokens {
		return DispositionReject, "tenant-token-capacity-exhausted", 0
	}

	saturationRatio := float64(q.totalTokens) / float64(q.cfg.MaxQueueTokens)
	depthRatio := float64(len(q.heap)) / float64(q.cfg.MaxQueueDepth)
	isSaturated := saturationRatio >= q.cfg.SaturationThreshold || depthRatio >= q.cfg.SaturationThreshold

	if isSaturated {
		if req.Priority >= q.cfg.HighPriorityFloor {
			return DispositionAdmit, "admitted-high-priority", 0
		}
		if tCfg.TokenBudget > 0 && (tState.QueuedTokens+tState.ActiveTokens) >= tCfg.TokenBudget {
			retry := time.Duration(100+((cost%500)+100)) * time.Millisecond
			return DispositionBackpressure, "queue-saturated-tenant-over-budget", retry
		}
		if saturationRatio >= 0.95 {
			return DispositionBackpressure, "queue-saturated-backpressure", 250 * time.Millisecond
		}
	}

	return DispositionAdmit, "admitted", 0
}

// Dequeue extracts the highest priority viable request from the queue.
func (q *TokenQueue) Dequeue() *QueuedRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	for len(q.heap) > 0 {
		req := heap.Pop(&q.heap).(*QueuedRequest)
		delete(q.lookup, req.ID)
		cost := req.EffectiveCost()
		q.totalTokens -= cost

		tenantState := q.getOrCreateTenantState(req.TenantID)
		tenantState.QueuedRequests--
		tenantState.QueuedTokens -= cost

		if !req.Deadline.IsZero() && now.After(req.Deadline) {
			q.expiredEvictedCount++
			continue
		}

		tenantState.ActiveTokens += cost
		return req
	}
	return nil
}

// Peek returns the highest priority request without removing it.
func (q *TokenQueue) Peek() *QueuedRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.heap) == 0 {
		return nil
	}
	return q.heap[0]
}

// Remove cancels and extracts a specific request by ID.
func (q *TokenQueue) Remove(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	req, ok := q.lookup[id]
	if !ok {
		return false
	}
	heap.Remove(&q.heap, req.heapIndex)
	delete(q.lookup, id)

	cost := req.EffectiveCost()
	q.totalTokens -= cost
	tenantState := q.getOrCreateTenantState(req.TenantID)
	tenantState.QueuedRequests--
	tenantState.QueuedTokens -= cost
	return true
}

// Finish records request completion, releasing in-flight tokens.
func (q *TokenQueue) Finish(tenantID string, allocatedCost int64, actualTokens int64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	tenantState := q.getOrCreateTenantState(tenantID)
	tenantState.ActiveTokens -= allocatedCost
	if tenantState.ActiveTokens < 0 {
		tenantState.ActiveTokens = 0
	}
	tenantState.ServedTokens += actualTokens
}

// EvictExpired purges any requests whose deadlines have elapsed.
func (q *TokenQueue) EvictExpired(now time.Time) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	evicted := 0
	var valid []*QueuedRequest
	for len(q.heap) > 0 {
		req := heap.Pop(&q.heap).(*QueuedRequest)
		delete(q.lookup, req.ID)
		cost := req.EffectiveCost()
		q.totalTokens -= cost

		tenantState := q.getOrCreateTenantState(req.TenantID)
		tenantState.QueuedRequests--
		tenantState.QueuedTokens -= cost

		if !req.Deadline.IsZero() && now.After(req.Deadline) {
			q.expiredEvictedCount++
			evicted++
		} else {
			valid = append(valid, req)
		}
	}

	for _, req := range valid {
		cost := req.EffectiveCost()
		req.calculatedScore = q.calculatePriority(req, now)
		heap.Push(&q.heap, req)
		q.lookup[req.ID] = req
		q.totalTokens += cost
		tenantState := q.getOrCreateTenantState(req.TenantID)
		tenantState.QueuedRequests++
		tenantState.QueuedTokens += cost
	}

	return evicted
}

// Snapshot returns the current observable vector state of the queue.
func (q *TokenQueue) Snapshot() QueueMetrics {
	q.mu.Lock()
	defer q.mu.Unlock()

	maxTokens := q.cfg.MaxQueueTokens
	if maxTokens <= 0 {
		maxTokens = 1
	}
	ratio := float64(q.totalTokens) / float64(maxTokens)

	tenantsCopy := make(map[string]TenantState, len(q.tenants))
	for k, v := range q.tenants {
		tenantsCopy[k] = *v
	}

	return QueueMetrics{
		TotalRequestsQueued: int64(len(q.heap)),
		TotalTokensQueued:   q.totalTokens,
		MaxQueueDepth:       q.cfg.MaxQueueDepth,
		MaxQueueTokens:      q.cfg.MaxQueueTokens,
		SaturationRatio:     ratio,
		AdmittedCount:       q.admittedCount,
		BackpressuredCount:  q.backpressuredCount,
		RejectedCount:       q.rejectedCount,
		ExpiredEvictedCount: q.expiredEvictedCount,
		Tenants:             tenantsCopy,
	}
}

// Len returns the current count of items in the queue.
func (q *TokenQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.heap)
}

// TotalTokens returns the aggregate token count currently queued.
func (q *TokenQueue) TotalTokens() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.totalTokens
}
