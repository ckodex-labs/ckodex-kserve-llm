/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOIS_URN(t *testing.T) {
	t.Run("GeneratesCorrectURN", func(t *testing.T) {
		urn := URN("exec", "test-id")
		assert.Equal(t, "urn:ois:exec:ckodex:test-id", urn)
	})

	t.Run("IdempotentWithExistingURN", func(t *testing.T) {
		existing := "urn:ois:model:ckodex:phi-3"
		urn := URN("exec", existing)
		assert.Equal(t, existing, urn)
	})
}

func TestOIS_AuditLogger_Compliance(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(spanRecorder))
	tracer := tp.Tracer("test-tracer")

	logger := NewAuditLoggerWithOptions(nil, nil, false)

	ctx, span := tracer.Start(context.Background(), "inference-request")
	logger.LogModelAccess(ctx, "tenant-1", "llama3", "user-1", 100)
	span.End()

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)

	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, "audit", events[0].Name)

	attrs := make(map[string]string)
	for _, attr := range events[0].Attributes {
		attrs[string(attr.Key)] = attr.Value.AsString()
	}

	// Verify OIS Core Profile Requirements (Section 26.1)
	assert.True(t, strings.HasPrefix(attrs[AttrExecID], "urn:ois:exec:ckodex:"), "ExecID MUST be a URN")
	assert.Equal(t, ExecKindInference, attrs[AttrExecKind], "ExecKind MUST be inference for model access")
	assert.Equal(t, AuditSuccess, AuditOutcome(attrs["audit.outcome"]))
	assert.Equal(t, ReproExplanatory, attrs[AttrExecReproClass])
}

func TestOIS_ReceiptEmission(t *testing.T) {
	logger := NewAuditLoggerWithOptions(nil, nil, false)
	ctx := context.Background()

	execID := URN("exec", "12345")
	logger.LogReceipt(ctx, execID, "ok", "Inference complete", map[string]string{"foo": "bar"})

	// Check if LogReceipt correctly sets semantic kind
	// (Internally it calls emit, we can verify via a custom logger or by checking the file)
}

func TestOIS_RedactionCompliance(t *testing.T) {
	redactor := NewRedactor(true)

	input := "My SSN is 123-45-6789"
	output := redactor.RedactString(input)

	assert.Equal(t, "My SSN is __REDACTED__", output, "MUST use canonical __REDACTED__ placeholder")
}

func TestOIS_SemanticMetrics(t *testing.T) {
	// Since NewInstrumentation doesn't return attributes, we check tenantModelAttrs
	attrs := tenantModelAttrs("tenant-1", "model-1")
	attrMap := make(map[string]string)
	for _, kv := range attrs {
		attrMap[string(kv.Key)] = kv.Value.AsString()
	}

	assert.Equal(t, "tenant-1", attrMap[AttrActorID])
	assert.Equal(t, "urn:ois:actor:ckodex:tenant-1", attrMap[AttrActorURN])
	assert.Equal(t, "service", attrMap[AttrActorType])
	assert.Equal(t, "model-1", attrMap[AttrModelBaseID])
	assert.Equal(t, "vllm", attrMap[AttrEngineRuntime])
}

func TestOIS_InferenceProfile_Coverage(t *testing.T) {
	t.Run("ModelAssembly_Serialization", func(t *testing.T) {
		assembly := ModelAssembly{
			Base:         ModelIdentity{ID: "llama3", URN: URN("model", "llama3"), Version: "1.0"},
			Quantization: &QuantProfile{ID: "4bit", Method: "awq", Bits: 4},
			Adapters: []ModelIdentity{
				{ID: "lora-finance", URN: URN("adapter", "lora-finance")},
			},
		}

		assert.Equal(t, "urn:ois:model:ckodex:llama3", assembly.Base.URN)
		assert.Equal(t, 4, assembly.Quantization.Bits)
		assert.Len(t, assembly.Adapters, 1)
	})

	t.Run("RichInferenceSignal_Emission", func(t *testing.T) {
		spanRecorder := tracetest.NewSpanRecorder()
		tp := trace.NewTracerProvider(trace.WithSpanProcessor(spanRecorder))
		tracer := tp.Tracer("test-tracer")

		logger := NewAuditLoggerWithOptions(nil, nil, false)
		ctx, span := tracer.Start(context.Background(), "inference")

		assembly := ModelAssembly{Base: ModelIdentity{ID: "phi-3"}}
		perf := PerformanceMetrics{LatencyMS: 150, FirstTokenMS: 40}

		logger.LogRichInferenceSignal(ctx, "exec-123", "tenant-1", assembly, perf, AuditSuccess, nil)
		span.End()

		spans := spanRecorder.Ended()
		require.Len(t, spans, 1)
		events := spans[0].Events()
		require.Len(t, events, 1)

		attrs := make(map[string]string)
		for _, attr := range events[0].Attributes {
			attrs[string(attr.Key)] = attr.Value.AsString()
		}

		assert.Equal(t, "150", attrs[AttrPerfLatencyMS])
		assert.Equal(t, "40", attrs[AttrPerfFirstTokenMS])
		assert.Equal(t, "phi-3", attrs[AttrModelBaseID])
	})
}
