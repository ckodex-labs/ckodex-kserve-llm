/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package inference provides request-level inference optimizations.
package inference

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// RequestPipeline orchestrates the full inference request lifecycle
// with latency budget enforcement at each phase.
type RequestPipeline struct {
	pool        *ConnectionPool
	router      *FastPathRouter
	preloader   *Preloader
	coalescer   *Coalescer
	cache       *SemanticCache
	prefetcher  *AnticipatoryPrefetcher
	pipeliner   *ChunkedPipeliner
	loraManager *LoRAPinManager
	obs         *observability.Pipeline
}

// NewRequestPipeline creates a production-configured pipeline.
func NewRequestPipeline() *RequestPipeline {
	pool := NewConnectionPool(DefaultPoolConfig())
	preloader := NewPreloader()
	router := NewFastPathRouter(pool)

	return &RequestPipeline{
		pool:      pool,
		router:    router,
		preloader: preloader,
		coalescer: NewCoalescer(5*time.Millisecond, 32),
		// In-memory backend — callers that want Redis use NewRequestPipelineWithCache.
		cache:       mustInMemoryCache(),
		prefetcher:  NewAnticipatoryPrefetcher(pool, router),
		pipeliner:   NewChunkedPipeliner(pool),
		loraManager: NewLoRAPinManager(16384), // 16GB pinned RAM for LoRAs
		obs:         observability.NewPipeline(),
	}
}

// InferenceRequest represents a single inference call.
type InferenceRequest struct {
	// Model is the model name.
	Model string

	// SessionID is the session for KV-cache affinity.
	SessionID string

	// SessionEndpoint is the previously bound endpoint.
	SessionEndpoint string

	// Candidates are the available inference endpoints.
	Candidates []string

	// Prompt is the input text.
	Prompt string

	// MaxTokens limits output length.
	MaxTokens int32

	// Stream enables SSE token streaming.
	Stream bool
}

// InferenceResponse contains the inference result.
type InferenceResponse struct {
	// Model is the model name.
	Model string

	// Endpoint is the endpoint that served the request.
	Endpoint string

	// CacheHit indicates KV-cache reuse.
	CacheHit bool

	// Phases records per-phase timing.
	Phases PhaseTimings

	// TotalLatency is the end-to-end request time.
	TotalLatency time.Duration
}

// PhaseTimings records latency per inference phase.
type PhaseTimings struct {
	RoutingMs  float64
	QueueMs    float64
	PrefillMs  float64
	DecodeMs   float64
	TransferMs float64
}

// Execute runs the full inference pipeline with latency budget enforcement.
func (p *RequestPipeline) Execute(ctx context.Context, req *InferenceRequest) (*InferenceResponse, error) {
	start := time.Now()

	// Create root span for the inference request
	ctx, span := p.obs.StartInference(ctx, req.Model, req.SessionID)
	defer span.End()

	budget := DefaultLatencyBudget()
	budgetCtx, cancel := budget.WithContext(ctx)
	defer cancel()

	resp := &InferenceResponse{
		Model: req.Model,
	}

	// Phase 0: Semantic Cache check (Zero GPU cycles)
	if _, hit := p.cache.GetExact(ctx, req.Prompt); hit {
		resp.Endpoint = "semantic-cache"
		resp.CacheHit = true
		resp.TotalLatency = time.Since(start)
		// Return immediately, bypassing routing and execution completely
		// The cachedResponse string would be returned here in real operation.
		return resp, nil
	}

	// Phase 1: Route (budget: 50ms)
	routeCtx, routeCancel := budget.PhaseContext(budgetCtx, "route")
	routeCtx, routeSpan := p.obs.StartSessionRoute(routeCtx, req.SessionID)
	routeResult := p.resolveRoute(routeCtx, req, start)
	routeCancel()
	routeSpan.End()

	if routeResult.Endpoint == "" {
		return nil, fmt.Errorf("no healthy endpoint for model %s", req.Model)
	}

	resp.Endpoint = routeResult.Endpoint
	resp.CacheHit = routeResult.CacheHit
	resp.Phases.RoutingMs = float64(routeResult.RoutingLatency.Microseconds()) / 1000.0

	// Phase 2: Check model readiness
	if err := p.preloader.WaitReady(budgetCtx, req.Model); err != nil {
		// Model not in preloader; assume ready (already loaded by operator)
		slog.Debug("preloader readiness check returned error; assuming model ready", "model", req.Model, "error", err)
	}

	// Phase 3: Acquire connection and track active requests
	conn := p.pool.Get(routeResult.Endpoint)
	conn.ActiveRequests.Add(1)
	defer p.releaseRequest(routeResult.Endpoint, start, conn)

	// Phase 4: Execute inference (budget: remaining)
	if budget.Exceeded() {
		err := fmt.Errorf("latency budget exceeded before inference: %v elapsed", time.Since(start))
		observability.RecordError(span, err)
		return nil, err
	}

	if err := p.runInference(ctx, conn, req, routeResult.Endpoint); err != nil {
		observability.RecordError(span, err)
		return nil, fmt.Errorf("V2 inference failed: %w", err)
	}

	resp.TotalLatency = time.Since(start)

	// Record success metrics on span
	observability.RecordInferenceMetrics(span, 0, 0, resp.TotalLatency, routeResult.CacheHit)
	observability.RecordSuccess(span)

	return resp, nil
}
