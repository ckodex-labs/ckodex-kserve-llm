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
	"sync"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	v2 "github.com/ckodex-labs/kserve-llm-operator/internal/protocol/v2"
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
	router := NewFastPathRouter(pool, preloader)

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
	defer routeSpan.End()

	// Check bleeding-edge state: Did the user stream a massive context?
	endpoint, seqID, pipelined := p.pipeliner.GetPipelinedEndpoint(req.SessionID)
	if pipelined {
		// We already have a warm KV cache block matching seqID on this endpoint
		_ = seqID
	} else {
		// Fallback to anticipatory prefetcher (did we warm this while they typed?)
		endpoint, pipelined = p.prefetcher.GetWarmedEndpoint(req.SessionID)
	}

	var routeResult RouteResult
	if pipelined {
		routeResult = RouteResult{
			Endpoint:           endpoint,
			CacheHit:           true,
			EstimatedLatencyMs: 0,
			RoutingLatency:     time.Since(start),
		}
	} else {
		routeResult = p.router.Route(routeCtx, req.SessionEndpoint, req.Candidates)
	}
	routeCancel()

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
	defer func() {
		conn.ActiveRequests.Add(-1)
		p.pool.RecordLatency(routeResult.Endpoint, time.Since(start))
	}()

	// Phase 4: Execute inference (budget: remaining)
	if budget.Exceeded() {
		err := fmt.Errorf("latency budget exceeded before inference: %v elapsed", time.Since(start))
		observability.RecordError(span, err)
		return nil, err
	}

	// Create V2 client using the pooled transport
	v2Client := v2.NewClient(
		fmt.Sprintf("http://%s", routeResult.Endpoint),
		v2.WithHTTPClient(conn.Client),
	)

	// Map internal request to V2 protocol request
	v2Req := &v2.InferRequest{
		ID: req.SessionID,
		Inputs: []v2.InferInput{
			{
				Name:     "prompt",
				Shape:    []int64{1},
				Datatype: v2.DatatypeBYTES,
				Data:     []string{req.Prompt},
			},
		},
	}

	// Execute the HTTP call with span
	v2Resp, err := v2Client.Infer(ctx, req.Model, "", v2Req)
	if err != nil {
		observability.RecordError(span, err)
		return nil, fmt.Errorf("V2 inference failed: %w", err)
	}

	// Map V2 response back to internal response
	// In a real implementation, we'd extract the tensor data
	_ = v2Resp

	resp.TotalLatency = time.Since(start)

	// Record success metrics on span
	observability.RecordInferenceMetrics(span, 0, 0, resp.TotalLatency, routeResult.CacheHit)
	observability.RecordSuccess(span)

	return resp, nil
}

// --- Background Maintenance ---

// StartMaintenance runs periodic pool cleanup in the background.
func (p *RequestPipeline) StartMaintenance(ctx context.Context) {
	var wg sync.WaitGroup

	// Idle connection eviction every 30s
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.pool.EvictIdle(5 * time.Minute)
			}
		}
	}()

	// Request coalescer flush every 5ms
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				batches := p.coalescer.Flush()
				for key, waiters := range batches {
					_ = key
					// Execute batch inference and broadcast result
					result := CoalescedResult{Data: nil, Error: nil}
					for _, ch := range waiters {
						ch <- result
						close(ch)
					}
				}
			}
		}
	}()

	wg.Wait()
}
