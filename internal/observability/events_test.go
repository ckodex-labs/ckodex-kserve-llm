/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// newTestSpan returns a recording span backed by an in-memory exporter.
func newTestSpan(t *testing.T, name string) (sdktrace.ReadWriteSpan, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	tracer := tp.Tracer("test")
	_, span := tracer.Start(t.Context(), name)
	//nolint:forcetypeassert
	return span.(sdktrace.ReadWriteSpan), rec
}

// ---- AddReconcileStartEvent --------------------------------------------------

func TestAddReconcileStartEvent_SetsAttributes(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddReconcileStartEvent(span, "mymodel", "prod", "v1")
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventReconcileStart, events[0].Name)
}

func TestAddReconcileCompleteEvent_SetsAttributes(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddReconcileCompleteEvent(span, "mymodel", true)
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventReconcileComplete, events[0].Name)
}

func TestAddInferenceFirstTokenEvent_SetsAttributes(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddInferenceFirstTokenEvent(span, 120, "acme", "llama3")
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventInferenceFirstToken, events[0].Name)
}

func TestAddInferenceCompleteEvent_SetsAttributes(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddInferenceCompleteEvent(span, 100, 200, 350)
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventInferenceComplete, events[0].Name)
}

func TestAddScaleEvent_ScaleOut(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddScaleEvent(span, observability.EventScaleOut, 1, 3, "load")
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventScaleOut, events[0].Name)
}

func TestAddLoRASwapEvent_Start(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddLoRASwapEvent(span, false, "adapter-a", "adapter-b")
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventLoRASwapStart, events[0].Name)
}

func TestAddLoRASwapEvent_Done(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddLoRASwapEvent(span, true, "adapter-a", "adapter-b")
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventLoRASwapDone, events[0].Name)
}

func TestAddKVCacheEvent_Hit(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddKVCacheEvent(span, true, "sess-42")
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventKVCacheHit, events[0].Name)
}

func TestAddKVCacheEvent_Miss(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddKVCacheEvent(span, false, "sess-42")
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventKVCacheMiss, events[0].Name)
}

func TestAddPolicyViolationEvent_SetsAttributes(t *testing.T) {
	span, rec := newTestSpan(t, "root")
	observability.AddPolicyViolationEvent(span, "opa-policy", "registry-not-allowed", "tenant-001")
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, observability.EventPolicyViolation, events[0].Name)
}
