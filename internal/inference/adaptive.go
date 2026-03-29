/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// AdaptiveTimeout dynamically adjusts request timeouts based on observed
// latency percentiles and current system load. Prevents timeout cascades
// while keeping tail latency bounded.
type AdaptiveTimeout struct {
	mu         sync.RWMutex
	samples    []int64 // ring buffer of latency samples (ms)
	writeIdx   int
	sampleSize int

	// Configurable bounds
	minTimeout time.Duration
	maxTimeout time.Duration

	// Current computed values
	p50 atomic.Int64
	p95 atomic.Int64
	p99 atomic.Int64
}

// NewAdaptiveTimeout creates a timeout controller with the given sample window.
func NewAdaptiveTimeout(sampleSize int, min, max time.Duration) *AdaptiveTimeout {
	return &AdaptiveTimeout{
		samples:    make([]int64, sampleSize),
		sampleSize: sampleSize,
		minTimeout: min,
		maxTimeout: max,
	}
}

// Record adds a latency observation.
func (a *AdaptiveTimeout) Record(latency time.Duration) {
	ms := latency.Milliseconds()
	a.mu.Lock()
	a.samples[a.writeIdx] = ms
	a.writeIdx = (a.writeIdx + 1) % a.sampleSize
	a.mu.Unlock()

	// Recompute percentiles periodically (every 100 samples)
	if a.writeIdx%100 == 0 {
		a.recomputePercentiles()
	}
}

// Timeout returns the current adaptive timeout.
// Uses P99 + 20% headroom, clamped between min and max.
func (a *AdaptiveTimeout) Timeout() time.Duration {
	p99 := a.p99.Load()
	if p99 == 0 {
		return a.maxTimeout // No data yet, use max
	}

	// P99 + 20% headroom
	timeout := time.Duration(float64(p99)*1.2) * time.Millisecond

	if timeout < a.minTimeout {
		return a.minTimeout
	}
	if timeout > a.maxTimeout {
		return a.maxTimeout
	}
	return timeout
}

// P50 returns the median latency.
func (a *AdaptiveTimeout) P50() time.Duration {
	return time.Duration(a.p50.Load()) * time.Millisecond
}

// P95 returns the 95th percentile latency.
func (a *AdaptiveTimeout) P95() time.Duration {
	return time.Duration(a.p95.Load()) * time.Millisecond
}

// P99 returns the 99th percentile latency.
func (a *AdaptiveTimeout) P99() time.Duration {
	return time.Duration(a.p99.Load()) * time.Millisecond
}

func (a *AdaptiveTimeout) recomputePercentiles() {
	a.mu.RLock()
	sorted := make([]int64, 0, a.sampleSize)
	for _, s := range a.samples {
		if s > 0 {
			sorted = append(sorted, s)
		}
	}
	a.mu.RUnlock()

	if len(sorted) == 0 {
		return
	}

	// Insertion sort (small N, no alloc)
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	n := len(sorted)
	a.p50.Store(sorted[n/2])
	a.p95.Store(sorted[int(float64(n)*0.95)])
	a.p99.Store(sorted[int(math.Min(float64(n)*0.99, float64(n-1)))])
}

// --- Priority Queue ---

// Priority defines request priority levels.
type Priority int

const (
	// PriorityCritical is for health checks and internal control plane requests.
	PriorityCritical Priority = 0

	// PriorityInteractive is for user-facing chat/completion requests.
	PriorityInteractive Priority = 1

	// PriorityBatch is for batch/offline processing.
	PriorityBatch Priority = 2

	// PriorityBackground is for prefill warmup and speculative precompute.
	PriorityBackground Priority = 3
)

// PriorityRequest wraps a request with scheduling metadata.
type PriorityRequest struct {
	Priority    Priority
	EnqueueTime time.Time
	ModelName   string
	SessionID   string
	MaxTokens   int32
	Deadline    time.Time

	// Result is written by the executor and closed when done.
	Result chan PriorityResult
}

// PriorityResult is the response from priority-scheduled execution.
type PriorityResult struct {
	Endpoint string
	Latency  time.Duration
	Error    error
}

// PriorityQueue implements a multi-level feedback queue for inference requests.
// Interactive requests preempt batch; batch requests yield under load.
type PriorityQueue struct {
	mu       sync.Mutex
	queues   [4][]PriorityRequest // One queue per priority level
	depth    atomic.Int64
	maxDepth int
}

// NewPriorityQueue creates a queue with the given max depth.
func NewPriorityQueue(maxDepth int) *PriorityQueue {
	return &PriorityQueue{maxDepth: maxDepth}
}

// Enqueue adds a request. Returns false if the queue is full.
func (q *PriorityQueue) Enqueue(req PriorityRequest) bool {
	if int(q.depth.Load()) >= q.maxDepth {
		// Shed lowest-priority requests when full
		q.mu.Lock()
		if len(q.queues[PriorityBackground]) > 0 {
			dropped := q.queues[PriorityBackground][0]
			q.queues[PriorityBackground] = q.queues[PriorityBackground][1:]
			q.depth.Add(-1)
			q.mu.Unlock()
			dropped.Result <- PriorityResult{Error: ErrLoadShed}
			close(dropped.Result)
		} else {
			q.mu.Unlock()
			return false
		}
	}

	q.mu.Lock()
	q.queues[req.Priority] = append(q.queues[req.Priority], req)
	q.mu.Unlock()
	q.depth.Add(1)
	return true
}

// Dequeue returns the highest-priority pending request.
// Returns ok=false if all queues are empty.
func (q *PriorityQueue) Dequeue() (PriorityRequest, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for p := PriorityCritical; p <= PriorityBackground; p++ {
		if len(q.queues[p]) > 0 {
			req := q.queues[p][0]
			q.queues[p] = q.queues[p][1:]
			q.depth.Add(-1)

			// Skip expired requests
			if !req.Deadline.IsZero() && time.Now().After(req.Deadline) {
				req.Result <- PriorityResult{Error: ErrDeadlineExceeded}
				close(req.Result)
				continue
			}
			return req, true
		}
	}
	return PriorityRequest{}, false
}

// Depth returns the total queue depth.
func (q *PriorityQueue) Depth() int64 {
	return q.depth.Load()
}

// --- Graceful Degradation ---

// DegradationLevel represents system load states.
type DegradationLevel int

const (
	// DegradationNone means full quality, all features enabled.
	DegradationNone DegradationLevel = 0

	// DegradationLight reduces max tokens and disables speculative decoding.
	DegradationLight DegradationLevel = 1

	// DegradationModerate sheds batch traffic and limits concurrent requests.
	DegradationModerate DegradationLevel = 2

	// DegradationSevere limits to interactive-only with hard token cap.
	DegradationSevere DegradationLevel = 3
)

// DegradationController monitors load and adjusts quality limits.
type DegradationController struct {
	mu    sync.RWMutex
	level DegradationLevel
	rules []DegradationRule
}

// DegradationRule maps a load threshold to a response.
type DegradationRule struct {
	// QueueDepthThreshold triggers this rule when queue depth exceeds it.
	QueueDepthThreshold int64

	// P99LatencyThreshold triggers when P99 exceeds this duration.
	P99LatencyThreshold time.Duration

	// Level is the degradation level to set.
	Level DegradationLevel

	// MaxTokensOverride caps output tokens at this level.
	MaxTokensOverride int32

	// RejectBatch rejects batch-priority requests at this level.
	RejectBatch bool

	// MaxConcurrent limits concurrent inference requests.
	MaxConcurrent int32
}

// NewDegradationController creates a controller with default thresholds.
func NewDegradationController() *DegradationController {
	return &DegradationController{
		rules: []DegradationRule{
			{QueueDepthThreshold: 100, P99LatencyThreshold: 5 * time.Second, Level: DegradationLight, MaxTokensOverride: 2048, MaxConcurrent: 200},
			{QueueDepthThreshold: 500, P99LatencyThreshold: 15 * time.Second, Level: DegradationModerate, MaxTokensOverride: 1024, RejectBatch: true, MaxConcurrent: 100},
			{QueueDepthThreshold: 1000, P99LatencyThreshold: 30 * time.Second, Level: DegradationSevere, MaxTokensOverride: 512, RejectBatch: true, MaxConcurrent: 50},
		},
	}
}

// Evaluate checks current metrics and sets the degradation level.
func (d *DegradationController) Evaluate(queueDepth int64, p99 time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	newLevel := DegradationNone
	for _, rule := range d.rules {
		if queueDepth >= rule.QueueDepthThreshold || p99 >= rule.P99LatencyThreshold {
			newLevel = rule.Level
		}
	}
	d.level = newLevel
}

// Level returns the current degradation level.
func (d *DegradationController) Level() DegradationLevel {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.level
}

// ActiveRule returns the rule for the current degradation level.
func (d *DegradationController) ActiveRule() *DegradationRule {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for i := range d.rules {
		if d.rules[i].Level == d.level {
			return &d.rules[i]
		}
	}
	return nil
}

// ClampTokens applies the max tokens override for the current level.
func (d *DegradationController) ClampTokens(requested int32) int32 {
	rule := d.ActiveRule()
	if rule == nil {
		return requested
	}
	if requested > rule.MaxTokensOverride {
		return rule.MaxTokensOverride
	}
	return requested
}

// ShouldRejectBatch returns true if batch traffic should be rejected.
func (d *DegradationController) ShouldRejectBatch() bool {
	rule := d.ActiveRule()
	if rule == nil {
		return false
	}
	return rule.RejectBatch
}
