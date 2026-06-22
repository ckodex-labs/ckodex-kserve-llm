/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"time"
)

// FastPathRouter selects endpoints using latency-weighted scoring.
// Bypasses the full EPP pipeline when a session-bound endpoint is healthy.
type FastPathRouter struct {
	pool *ConnectionPool
}

// NewFastPathRouter creates a fast-path router with the given pool.
func NewFastPathRouter(pool *ConnectionPool) *FastPathRouter {
	return &FastPathRouter{pool: pool}
}

// RouteResult contains the routing decision.
type RouteResult struct {
	// Endpoint is the selected endpoint address.
	Endpoint string

	// CacheHit indicates the endpoint has a warm KV cache for this session.
	CacheHit bool

	// EstimatedLatencyMs is the predicted latency based on EWMA.
	EstimatedLatencyMs int64

	// RoutingLatency is how long the routing decision took.
	RoutingLatency time.Duration
}

// Route selects the best endpoint for a request.
// Priority: session-bound (cache hit) > fastest healthy > any available.
func (r *FastPathRouter) Route(_ context.Context, sessionEndpoint string, candidates []string) RouteResult {
	start := time.Now()
	result := RouteResult{}

	// Fast path: session-bound endpoint with cache
	if sessionEndpoint != "" {
		conn := r.pool.Get(sessionEndpoint)
		if conn.ErrorCount.Load() < 3 { // Circuit breaker threshold
			result.Endpoint = sessionEndpoint
			result.CacheHit = true
			result.EstimatedLatencyMs = conn.AvgLatencyMs.Load()
			result.RoutingLatency = time.Since(start)
			return result
		}
		// Session endpoint unhealthy, fall through to scored selection
	}

	// Scored selection: pick fastest healthy endpoint
	result.Endpoint = r.pool.FastestEndpoint(candidates)
	if result.Endpoint != "" {
		conn := r.pool.Get(result.Endpoint)
		result.EstimatedLatencyMs = conn.AvgLatencyMs.Load()
	}

	result.RoutingLatency = time.Since(start)
	return result
}
