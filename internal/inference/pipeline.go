/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package inference provides the request-level inference pipeline optimized
// for minimum time-to-first-token (TTFT) and time-to-inference (TTI).
package inference

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionPool manages persistent HTTP/2 connections to inference endpoints.
// Eliminates TCP+TLS handshake overhead on the hot path.
type ConnectionPool struct {
	mu        sync.RWMutex
	endpoints map[string]*EndpointConn
	transport *http.Transport
	config    PoolConfig
}

// PoolConfig configures the connection pool.
type PoolConfig struct {
	// MaxIdleConnsPerHost controls persistent connection reuse.
	MaxIdleConnsPerHost int

	// MaxConnsPerHost caps total connections to a single endpoint.
	MaxConnsPerHost int

	// IdleConnTimeout is how long idle connections stay in the pool.
	IdleConnTimeout time.Duration

	// DialTimeout is the TCP connection timeout.
	DialTimeout time.Duration

	// TLSHandshakeTimeout is the TLS negotiation timeout.
	TLSHandshakeTimeout time.Duration

	// EnableHTTP2 forces HTTP/2 for multiplexed requests.
	EnableHTTP2 bool

	// DisableKeepAlives disables connection reuse (for testing only).
	DisableKeepAlives bool
}

// DefaultPoolConfig returns production defaults.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     250,
		IdleConnTimeout:     90 * time.Second,
		DialTimeout:         2 * time.Second,
		TLSHandshakeTimeout: 3 * time.Second,
		EnableHTTP2:         true,
	}
}

// EndpointConn tracks a single endpoint's connection state.
type EndpointConn struct {
	// Address is the endpoint address (ip:port).
	Address string

	// Client is the per-endpoint HTTP client with connection pooling.
	Client *http.Client

	// ActiveRequests tracks in-flight requests for load awareness.
	ActiveRequests atomic.Int64

	// AvgLatencyMs is the exponentially weighted moving average latency.
	AvgLatencyMs atomic.Int64

	// TotalRequests is the lifetime request count.
	TotalRequests atomic.Int64

	// ErrorCount is the recent error count (reset on success).
	ErrorCount atomic.Int64

	// LastUsed tracks the last request time for idle eviction.
	LastUsed atomic.Int64
}

// NewConnectionPool creates a pool with optimized transport settings.
func NewConnectionPool(config PoolConfig) *ConnectionPool {
	transport := &http.Transport{
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		DisableKeepAlives:   config.DisableKeepAlives,
		ForceAttemptHTTP2:   config.EnableHTTP2,
		DialContext: (&net.Dialer{
			Timeout:   config.DialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: config.TLSHandshakeTimeout,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		// Pre-allocate write buffer to avoid allocation on hot path
		WriteBufferSize: 64 * 1024,
		ReadBufferSize:  64 * 1024,
	}

	return &ConnectionPool{
		endpoints: make(map[string]*EndpointConn),
		transport: transport,
		config:    config,
	}
}

// Get returns or creates a connection to the endpoint.
func (p *ConnectionPool) Get(address string) *EndpointConn {
	p.mu.RLock()
	conn, ok := p.endpoints[address]
	p.mu.RUnlock()
	if ok {
		return conn
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after write lock
	if conn, ok := p.endpoints[address]; ok {
		return conn
	}

	conn = &EndpointConn{
		Address: address,
		Client: &http.Client{
			Transport: p.transport,
			Timeout:   0, // Managed by LatencyBudget per-request
		},
	}
	p.endpoints[address] = conn
	return conn
}

// FastestEndpoint returns the endpoint with the lowest weighted latency.
// Combines avg latency, active request count, and error rate.
func (p *ConnectionPool) FastestEndpoint(candidates []string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	bestAddr := ""
	bestScore := int64(1<<62 - 1) // max int63

	for _, addr := range candidates {
		conn, ok := p.endpoints[addr]
		if !ok {
			return addr // New endpoint, try it
		}

		// Score = avgLatency + (activeRequests * 10) + (errors * 100)
		score := conn.AvgLatencyMs.Load() +
			conn.ActiveRequests.Load()*10 +
			conn.ErrorCount.Load()*100

		if score < bestScore {
			bestScore = score
			bestAddr = addr
		}
	}

	if bestAddr == "" && len(candidates) > 0 {
		return candidates[0]
	}
	return bestAddr
}

// RecordLatency updates the EWMA latency for an endpoint.
func (p *ConnectionPool) RecordLatency(address string, latency time.Duration) {
	conn := p.Get(address)
	ms := latency.Milliseconds()

	// EWMA with alpha=0.3 (recent samples weighted more)
	current := conn.AvgLatencyMs.Load()
	if current == 0 {
		conn.AvgLatencyMs.Store(ms)
	} else {
		updated := (current*7 + ms*3) / 10
		conn.AvgLatencyMs.Store(updated)
	}

	conn.TotalRequests.Add(1)
	conn.ErrorCount.Store(0) // Reset on success
	conn.LastUsed.Store(time.Now().UnixMilli())
}

// RecordError increments the error count for circuit-breaking.
func (p *ConnectionPool) RecordError(address string) {
	conn := p.Get(address)
	conn.ErrorCount.Add(1)
}

// EvictIdle removes endpoints that haven't been used recently.
func (p *ConnectionPool) EvictIdle(maxIdle time.Duration) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	cutoff := time.Now().Add(-maxIdle).UnixMilli()
	evicted := 0

	for addr, conn := range p.endpoints {
		if conn.LastUsed.Load() < cutoff && conn.ActiveRequests.Load() == 0 {
			delete(p.endpoints, addr)
			evicted++
		}
	}
	return evicted
}

// --- Latency Budget ---

// LatencyBudget propagates a request-level deadline across inference phases.
// Each phase (route → prefill → decode → respond) gets a proportional budget.
type LatencyBudget struct {
	// TotalBudget is the end-to-end latency limit.
	TotalBudget time.Duration

	// RoutingBudget is the max time for endpoint selection.
	RoutingBudget time.Duration

	// PrefillBudget is the max time for the prefill phase.
	PrefillBudget time.Duration

	// DecodeBudget is the max time for decode (remaining after route+prefill).
	DecodeBudget time.Duration

	// Deadline is the absolute wall-clock deadline.
	Deadline time.Time
}

// DefaultLatencyBudget returns a 30s total budget split across phases.
func DefaultLatencyBudget() LatencyBudget {
	total := 30 * time.Second
	return LatencyBudget{
		TotalBudget:   total,
		RoutingBudget: 50 * time.Millisecond,
		PrefillBudget: 10 * time.Second,
		DecodeBudget:  total - 50*time.Millisecond - 10*time.Second,
		Deadline:      time.Now().Add(total),
	}
}

// WithContext returns a context with the budget's deadline.
func (b *LatencyBudget) WithContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, b.Deadline)
}

// Remaining returns the time left in the budget.
func (b *LatencyBudget) Remaining() time.Duration {
	return time.Until(b.Deadline)
}

// PhaseContext returns a context scoped to a specific phase budget.
func (b *LatencyBudget) PhaseContext(ctx context.Context, phase string) (context.Context, context.CancelFunc) {
	var budget time.Duration
	switch phase {
	case "route":
		budget = b.RoutingBudget
	case "prefill":
		budget = b.PrefillBudget
	case "decode":
		budget = b.DecodeBudget
	default:
		budget = b.Remaining()
	}

	deadline := time.Now().Add(budget)
	if deadline.After(b.Deadline) {
		deadline = b.Deadline
	}
	return context.WithDeadline(ctx, deadline)
}

// Exceeded returns true if the budget is exhausted.
func (b *LatencyBudget) Exceeded() bool {
	return time.Now().After(b.Deadline)
}

// --- Warm Start Preloader ---

// Preloader handles background model loading to eliminate cold-start latency.
type Preloader struct {
	mu     sync.RWMutex
	models map[string]*PreloadState
}

// PreloadState tracks a single model's loading state.
type PreloadState struct {
	ModelName string
	Phase     PreloadPhase
	StartTime time.Time
	ReadyTime time.Time
	Error     error
	ReadyCh   chan struct{} // Closed when model is ready
}

// PreloadPhase represents model loading phases.
type PreloadPhase string

const (
	PreloadPending     PreloadPhase = "Pending"
	PreloadDownloading PreloadPhase = "Downloading"
	PreloadLoading     PreloadPhase = "Loading"
	PreloadWarming     PreloadPhase = "Warming"
	PreloadReady       PreloadPhase = "Ready"
	PreloadFailed      PreloadPhase = "Failed"
)

// NewPreloader creates a preloader.
func NewPreloader() *Preloader {
	return &Preloader{
		models: make(map[string]*PreloadState),
	}
}

// Start begins preloading a model in the background.
func (p *Preloader) Start(modelName string) *PreloadState {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, ok := p.models[modelName]; ok {
		return state
	}

	state := &PreloadState{
		ModelName: modelName,
		Phase:     PreloadPending,
		StartTime: time.Now(),
		ReadyCh:   make(chan struct{}),
	}
	p.models[modelName] = state
	return state
}

// MarkReady signals that a model is loaded and warmed up.
func (p *Preloader) MarkReady(modelName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, ok := p.models[modelName]; ok {
		state.Phase = PreloadReady
		state.ReadyTime = time.Now()
		close(state.ReadyCh)
	}
}

// MarkFailed records a model load failure.
func (p *Preloader) MarkFailed(modelName string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, ok := p.models[modelName]; ok {
		state.Phase = PreloadFailed
		state.Error = err
		close(state.ReadyCh)
	}
}

// WaitReady blocks until the model is ready or the context expires.
func (p *Preloader) WaitReady(ctx context.Context, modelName string) error {
	p.mu.RLock()
	state, ok := p.models[modelName]
	p.mu.RUnlock()

	if !ok {
		return fmt.Errorf("model %s not in preload queue", modelName)
	}

	select {
	case <-state.ReadyCh:
		// Channel close happens-before this receive, so fields are safe to read.
		if state.Error != nil {
			return fmt.Errorf("model %s preload failed: %w", modelName, state.Error)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("model %s preload timed out: %w", modelName, ctx.Err())
	}
}

// LoadDuration returns how long the model took to load.
func (p *Preloader) LoadDuration(modelName string) time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if state, ok := p.models[modelName]; ok && state.Phase == PreloadReady {
		return state.ReadyTime.Sub(state.StartTime)
	}
	return 0
}

// --- Streaming Response ---

// StreamWriter writes SSE-formatted chunks for streaming inference.
type StreamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	closed  bool
}

// NewStreamWriter creates a streaming response writer.
// Sets headers for SSE (Server-Sent Events) and disables buffering.
func NewStreamWriter(w http.ResponseWriter) (*StreamWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response writer does not support flushing")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &StreamWriter{w: w, flusher: flusher}, nil
}

// WriteChunk sends a single SSE data event and flushes immediately.
func (s *StreamWriter) WriteChunk(data []byte) error {
	if s.closed {
		return fmt.Errorf("stream closed")
	}
	_, err := fmt.Fprintf(s.w, "data: %s\n\n", data)
	if err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteDone sends the [DONE] sentinel and closes the stream.
func (s *StreamWriter) WriteDone() error {
	if s.closed {
		return nil
	}
	s.closed = true
	_, err := fmt.Fprint(s.w, "data: [DONE]\n\n")
	if err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// --- Fast-Path Router ---

// FastPathRouter selects endpoints using latency-weighted scoring.
// Bypasses the full EPP pipeline when a session-bound endpoint is healthy.
type FastPathRouter struct {
	pool      *ConnectionPool
	preloader *Preloader
}

// NewFastPathRouter creates a fast-path router with the given pool.
func NewFastPathRouter(pool *ConnectionPool, preloader *Preloader) *FastPathRouter {
	return &FastPathRouter{pool: pool, preloader: preloader}
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
func (r *FastPathRouter) Route(ctx context.Context, sessionEndpoint string, candidates []string) RouteResult {
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

// --- Request Coalescer ---

// Coalescer groups identical requests within a short window for batch processing.
// Reduces GPU utilization overhead when multiple users send the same prompt.
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
