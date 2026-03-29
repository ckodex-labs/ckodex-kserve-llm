/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SpanNames for end-to-end pipeline tracing.
const (
	SpanReconcile         = "ckodex.reconcile"
	SpanGateway           = "ckodex.gateway.reconcile"
	SpanScheduler         = "ckodex.scheduler.route"
	SpanSessionRoute      = "ckodex.session.route"
	SpanActorActivate     = "ckodex.actor.activate"
	SpanCoactorCoordinate = "ckodex.coactor.coordinate"
	SpanInferRequest      = "ckodex.infer.request"
	SpanInferPrefill      = "ckodex.infer.prefill"
	SpanInferDecode       = "ckodex.infer.decode"
	SpanModelDownload     = "ckodex.model.download"
	SpanModelOnboard      = "ckodex.model.onboard"
	SpanAuthVerify        = "ckodex.auth.verify"
	SpanKVCacheTransfer   = "ckodex.kvcache.transfer"
)

// Pipeline instruments the full agent inference pipeline with OTel spans.
type Pipeline struct {
	tracer trace.Tracer
}

// NewPipeline creates a new pipeline instrumentor.
func NewPipeline() *Pipeline {
	return &Pipeline{
		tracer: otel.Tracer(InstrumentationName),
	}
}

// StartReconcile creates a root span for a reconcile loop.
func (p *Pipeline) StartReconcile(ctx context.Context, model, namespace string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, SpanReconcile,
		trace.WithAttributes(
			attribute.String("model", model),
			attribute.String("namespace", namespace),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
}

// StartInference creates a span for an inference request.
func (p *Pipeline) StartInference(ctx context.Context, model, sessionID string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, SpanInferRequest,
		trace.WithAttributes(
			attribute.String("model", model),
			attribute.String("session.id", sessionID),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)
}

// StartPrefill creates a child span for the prefill phase.
func (p *Pipeline) StartPrefill(ctx context.Context, inputTokens int64) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, SpanInferPrefill,
		trace.WithAttributes(
			attribute.Int64("input_tokens", inputTokens),
		),
	)
}

// StartDecode creates a child span for the decode phase.
func (p *Pipeline) StartDecode(ctx context.Context, maxTokens int64) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, SpanInferDecode,
		trace.WithAttributes(
			attribute.Int64("max_tokens", maxTokens),
		),
	)
}

// StartSessionRoute creates a span for session-aware routing.
func (p *Pipeline) StartSessionRoute(ctx context.Context, sessionID string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, SpanSessionRoute,
		trace.WithAttributes(
			attribute.String("session.id", sessionID),
		),
	)
}

// StartActorActivation creates a span for actor lifecycle.
func (p *Pipeline) StartActorActivation(ctx context.Context, actorType, actorID string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, SpanActorActivate,
		trace.WithAttributes(
			attribute.String("actor.type", actorType),
			attribute.String("actor.id", actorID),
		),
	)
}

// StartCoactorCoordination creates a span for coactor group coordination.
func (p *Pipeline) StartCoactorCoordination(ctx context.Context, pattern, groupID string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, SpanCoactorCoordinate,
		trace.WithAttributes(
			attribute.String("pattern", pattern),
			attribute.String("group.id", groupID),
		),
	)
}

// StartAuth creates a span for auth verification.
func (p *Pipeline) StartAuth(ctx context.Context, issuer string) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, SpanAuthVerify,
		trace.WithAttributes(
			attribute.String("issuer", issuer),
		),
	)
}

// RecordError records an error on the span and sets error status.
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// RecordSuccess sets the span status to OK.
func RecordSuccess(span trace.Span) {
	span.SetStatus(codes.Ok, "")
}

// RecordInferenceMetrics adds inference-specific attributes to a span.
func RecordInferenceMetrics(span trace.Span, inputTokens, outputTokens int64, latency time.Duration, cacheHit bool) {
	span.SetAttributes(
		attribute.Int64("tokens.input", inputTokens),
		attribute.Int64("tokens.output", outputTokens),
		attribute.Float64("latency_ms", float64(latency.Milliseconds())),
		attribute.Bool("cache_hit", cacheHit),
	)
	if latency > 0 && outputTokens > 0 {
		tokensPerSec := float64(outputTokens) / latency.Seconds()
		span.SetAttributes(attribute.Float64("tokens_per_second", tokensPerSec))
	}
}
