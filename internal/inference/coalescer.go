/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"sync"
	"time"
)

// Coalescer groups identical requests within a short window for batch processing.
// It is a standalone primitive until a request executor is supplied by the
// inference pipeline; callers must flush it and publish real results.
type Coalescer struct {
	mu       sync.Mutex
	pending  map[string]*coalescedRequest
	window   time.Duration
	maxBatch int
}

type coalescedRequest struct {
	key       string
	waiters   []chan CoalescedResult
	createdAt time.Time
}

// CoalescedResult is the response shared across coalesced requests.
type CoalescedResult struct {
	Data  []byte
	Error error
}

// NewCoalescer creates a request coalescer.
func NewCoalescer(window time.Duration, maxBatch int) *Coalescer {
	return &Coalescer{
		pending:  make(map[string]*coalescedRequest),
		window:   window,
		maxBatch: maxBatch,
	}
}

// Submit adds a request to the coalescing window.
// Returns a channel that receives the result when the batch completes.
func (c *Coalescer) Submit(key string) <-chan CoalescedResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan CoalescedResult, 1)

	if req, ok := c.pending[key]; ok && len(req.waiters) < c.maxBatch {
		req.waiters = append(req.waiters, ch)
		return ch
	}

	req := &coalescedRequest{
		key:       key,
		waiters:   []chan CoalescedResult{ch},
		createdAt: time.Now(),
	}
	c.pending[key] = req
	return ch
}

// Flush returns all pending requests that have exceeded the coalescing window.
func (c *Coalescer) Flush() map[string][]chan CoalescedResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	ready := make(map[string][]chan CoalescedResult)
	cutoff := time.Now().Add(-c.window)

	for key, req := range c.pending {
		if req.createdAt.Before(cutoff) || len(req.waiters) >= c.maxBatch {
			ready[key] = req.waiters
			delete(c.pending, key)
		}
	}
	return ready
}
