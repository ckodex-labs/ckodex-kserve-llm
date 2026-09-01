/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package inference provides request-level inference optimizations.
package inference

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/accessplane"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// RequestPipeline orchestrates the full inference request lifecycle
// with latency budget enforcement at each phase.
type RequestPipeline struct {
	pool        *ConnectionPool
	router      *FastPathRouter
	preloader   *Preloader
	cache       *SemanticCache
	prefetcher  *AnticipatoryPrefetcher
	pipeliner   *ChunkedPipeliner
	loraManager *LoRAPinManager
	obs         *observability.Pipeline
	policy      *accessplane.Evaluator
}

// NewRequestPipeline creates a pipeline with request policy explicitly
// disabled. Callers with an access policy use NewRequestPipelineWithPolicy.
func NewRequestPipeline() *RequestPipeline {
	return newRequestPipeline(nil)
}

// NewRequestPipelineWithPolicy creates a pipeline that evaluates every request
// before cache lookup, routing, or endpoint execution.
func NewRequestPipelineWithPolicy(policy *accessplane.Evaluator) (*RequestPipeline, error) {
	if policy == nil {
		return nil, errors.New("request pipeline policy is required")
	}
	return newRequestPipeline(policy), nil
}

func newRequestPipeline(policy *accessplane.Evaluator) *RequestPipeline {
	pool := NewConnectionPool(DefaultPoolConfig())
	preloader := NewPreloader()
	router := NewFastPathRouter(pool)

	return &RequestPipeline{
		pool:      pool,
		router:    router,
		preloader: preloader,
		// In-memory backend — callers that want Redis use NewRequestPipelineWithCache.
		cache:       mustInMemoryCache(),
		prefetcher:  NewAnticipatoryPrefetcher(pool, router),
		pipeliner:   NewChunkedPipeliner(pool),
		loraManager: NewLoRAPinManager(16384), // 16GB pinned RAM for LoRAs
		obs:         observability.NewPipeline(),
		policy:      policy,
	}
}

// InferenceRequest represents a single inference call.
type InferenceRequest struct {
	// TenantID is the access-policy tenant boundary.
	TenantID string

	// Route is the access-policy route requested by the caller.
	Route string

	// AccessObservation is the caller's immutable runtime-load snapshot.
	AccessObservation accessplane.RuntimeObservation

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
	// PolicyDecision is present when the pipeline evaluated access policy.
	PolicyDecision *accessplane.Decision

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

// AccessPolicyError reports a non-admit policy decision. Backpressure is an
// instruction to the caller; the pipeline does not create or mutate a queue.
type AccessPolicyError struct {
	Decision accessplane.Decision
}

func (e *AccessPolicyError) Error() string {
	return fmt.Sprintf("request policy %s: %s", e.Decision.Disposition, e.Decision.Reason)
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

	effectiveReq := *req
	resp := &InferenceResponse{
		Model: effectiveReq.Model,
	}
	if p.policy != nil {
		decision, err := p.policy.EvaluateContext(budgetCtx, accessplane.Intent{
			TenantID: req.TenantID,
			Route:    req.Route,
		}, req.AccessObservation)
		if err != nil {
			observability.RecordError(span, err)
			return nil, fmt.Errorf("evaluate request policy: %w", err)
		}
		if decision.Disposition != accessplane.DispositionAdmit {
			err := &AccessPolicyError{Decision: decision}
			observability.RecordError(span, err)
			return nil, err
		}
		effectiveReq.Model = decision.Model
		resp.Model = decision.Model
		resp.PolicyDecision = &decision
	}

	// Phase 0: Semantic Cache check (Zero GPU cycles)
	if _, hit := p.cache.GetExact(ctx, effectiveReq.Prompt); hit {
		resp.Endpoint = "semantic-cache"
		resp.CacheHit = true
		resp.TotalLatency = time.Since(start)
		// Return immediately, bypassing routing and execution completely
		// TODO(ckodex): return the cached response payload when this pipeline owns
		// the response transport; currently this result exposes routing metadata.
		return resp, nil
	}

	// Phase 1: Route (budget: 50ms)
	routeCtx, routeCancel := budget.PhaseContext(budgetCtx, "route")
	routeCtx, routeSpan := p.obs.StartSessionRoute(routeCtx, effectiveReq.SessionID)
	routeResult := p.resolveRoute(routeCtx, &effectiveReq, start)
	routeCancel()
	routeSpan.End()

	if routeResult.Endpoint == "" {
		return nil, fmt.Errorf("no healthy endpoint for model %s", effectiveReq.Model)
	}

	resp.Endpoint = routeResult.Endpoint
	resp.CacheHit = routeResult.CacheHit
	resp.Phases.RoutingMs = float64(routeResult.RoutingLatency.Microseconds()) / 1000.0

	// Phase 2: Check model readiness
	if err := p.preloader.WaitReady(budgetCtx, effectiveReq.Model); err != nil {
		// Model not in preloader; assume ready (already loaded by operator)
		slog.Debug("preloader readiness check returned error; assuming model ready", "model", effectiveReq.Model, "error", err)
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

	if err := p.runInference(budgetCtx, conn, &effectiveReq, routeResult.Endpoint); err != nil {
		observability.RecordError(span, err)
		return nil, fmt.Errorf("V2 inference failed: %w", err)
	}

	resp.TotalLatency = time.Since(start)

	// Record success metrics on span
	observability.RecordInferenceMetrics(span, 0, 0, resp.TotalLatency, routeResult.CacheHit)
	observability.RecordSuccess(span)

	return resp, nil
}
