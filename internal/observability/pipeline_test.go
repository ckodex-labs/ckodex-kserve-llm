/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// newPipelineProvider sets up an SDK tracer provider for pipeline tests.
func newPipelineProvider(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	prevTP := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
	})
	return rec
}

// ---- NewPipeline -------------------------------------------------------------

func TestNewPipeline_NotNil(t *testing.T) {
	p := observability.NewPipeline()
	assert.NotNil(t, p)
}

// ---- StartReconcile ----------------------------------------------------------

func TestPipeline_StartReconcile_SpanCreated(t *testing.T) {
	rec := newPipelineProvider(t)
	p := observability.NewPipeline()

	ctx, span := p.StartReconcile(context.Background(), "llama3", "production")
	require.NotNil(t, span)
	assert.NotNil(t, ctx)
	span.End()

	require.Len(t, rec.Ended(), 1)
	assert.Equal(t, observability.SpanReconcile, rec.Ended()[0].Name())
}

// ---- StartInference ----------------------------------------------------------

func TestPipeline_StartInference_SpanCreated(t *testing.T) {
	rec := newPipelineProvider(t)
	p := observability.NewPipeline()

	ctx, span := p.StartInference(context.Background(), "llama3", "sess-001")
	require.NotNil(t, span)
	assert.NotNil(t, ctx)
	span.End()

	require.Len(t, rec.Ended(), 1)
	assert.Equal(t, observability.SpanInferRequest, rec.Ended()[0].Name())
}

// ---- StartPrefill ------------------------------------------------------------

func TestPipeline_StartPrefill_SpanCreated(t *testing.T) {
	rec := newPipelineProvider(t)
	p := observability.NewPipeline()

	ctx, span := p.StartPrefill(context.Background(), 512)
	require.NotNil(t, span)
	assert.NotNil(t, ctx)
	span.End()

	require.Len(t, rec.Ended(), 1)
	assert.Equal(t, observability.SpanInferPrefill, rec.Ended()[0].Name())
}

// ---- StartDecode -------------------------------------------------------------

func TestPipeline_StartDecode_SpanCreated(t *testing.T) {
	rec := newPipelineProvider(t)
	p := observability.NewPipeline()

	ctx, span := p.StartDecode(context.Background(), 256)
	require.NotNil(t, span)
	assert.NotNil(t, ctx)
	span.End()

	require.Len(t, rec.Ended(), 1)
	assert.Equal(t, observability.SpanInferDecode, rec.Ended()[0].Name())
}

// ---- StartSessionRoute -------------------------------------------------------

func TestPipeline_StartSessionRoute_SpanCreated(t *testing.T) {
	rec := newPipelineProvider(t)
	p := observability.NewPipeline()

	ctx, span := p.StartSessionRoute(context.Background(), "sess-abc")
	require.NotNil(t, span)
	assert.NotNil(t, ctx)
	span.End()

	require.Len(t, rec.Ended(), 1)
	assert.Equal(t, observability.SpanSessionRoute, rec.Ended()[0].Name())
}

// ---- StartActorActivation ----------------------------------------------------

func TestPipeline_StartActorActivation_SpanCreated(t *testing.T) {
	rec := newPipelineProvider(t)
	p := observability.NewPipeline()

	ctx, span := p.StartActorActivation(context.Background(), "worker", "actor-1")
	require.NotNil(t, span)
	assert.NotNil(t, ctx)
	span.End()

	require.Len(t, rec.Ended(), 1)
	assert.Equal(t, observability.SpanActorActivate, rec.Ended()[0].Name())
}

// ---- StartCoactorCoordination ------------------------------------------------

func TestPipeline_StartCoactorCoordination_SpanCreated(t *testing.T) {
	rec := newPipelineProvider(t)
	p := observability.NewPipeline()

	ctx, span := p.StartCoactorCoordination(context.Background(), "scatter-gather", "group-7")
	require.NotNil(t, span)
	assert.NotNil(t, ctx)
	span.End()

	require.Len(t, rec.Ended(), 1)
	assert.Equal(t, observability.SpanCoactorCoordinate, rec.Ended()[0].Name())
}

// ---- StartAuth ---------------------------------------------------------------

func TestPipeline_StartAuth_SpanCreated(t *testing.T) {
	rec := newPipelineProvider(t)
	p := observability.NewPipeline()

	ctx, span := p.StartAuth(context.Background(), "https://auth.example.com")
	require.NotNil(t, span)
	assert.NotNil(t, ctx)
	span.End()

	require.Len(t, rec.Ended(), 1)
	assert.Equal(t, observability.SpanAuthVerify, rec.Ended()[0].Name())
}

// ---- RecordError -------------------------------------------------------------

func TestRecordError_NonNil_SetsError(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "root")
	observability.RecordError(span, errors.New("something broke"))
	span.End()
	_ = tp.Shutdown(context.Background())

	require.Len(t, rec.Ended(), 1)
	// Span should have at least one event (the recorded error).
	assert.NotEmpty(t, rec.Ended()[0].Events())
}

func TestRecordError_Nil_NoPanic(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "root")

	assert.NotPanics(t, func() {
		observability.RecordError(span, nil)
	})
	span.End()
	_ = tp.Shutdown(context.Background())
}

// ---- RecordSuccess -----------------------------------------------------------

func TestRecordSuccess_NoPanic(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "root")

	assert.NotPanics(t, func() {
		observability.RecordSuccess(span)
	})
	span.End()
	_ = tp.Shutdown(context.Background())
}

// ---- RecordInferenceMetrics -------------------------------------------------

func TestRecordInferenceMetrics_CacheHit_NoPanic(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "root")

	assert.NotPanics(t, func() {
		observability.RecordInferenceMetrics(span, 128, 64, 150*time.Millisecond, true)
	})
	span.End()
	_ = tp.Shutdown(context.Background())
}

func TestRecordInferenceMetrics_ZeroLatency_NoPanic(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "root")

	assert.NotPanics(t, func() {
		observability.RecordInferenceMetrics(span, 100, 0, 0, false)
	})
	span.End()
	_ = tp.Shutdown(context.Background())
}

func TestRecordInferenceMetrics_WithLatencyAndTokens_TokensPerSecondSet(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "root")

	// latency=1s, 100 output tokens → 100 tokens/s
	observability.RecordInferenceMetrics(span, 50, 100, time.Second, false)
	span.End()
	_ = tp.Shutdown(context.Background())

	require.Len(t, rec.Ended(), 1)
	attrs := rec.Ended()[0].Attributes()
	var found bool
	for _, a := range attrs {
		if string(a.Key) == "tokens_per_second" {
			found = true
			assert.InDelta(t, 100.0, a.Value.AsFloat64(), 0.01)
		}
	}
	assert.True(t, found, "tokens_per_second attribute must be present")
}
