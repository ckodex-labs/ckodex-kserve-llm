/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type failingCreateClient struct {
	client.Client
}

type capturingCreateClient struct {
	client.Client
	event *corev1.Event
}

func (c *capturingCreateClient) Create(_ context.Context, object client.Object, _ ...client.CreateOption) error {
	event, ok := object.(*corev1.Event)
	if ok {
		c.event = event.DeepCopy()
	}
	return nil
}

func (failingCreateClient) Create(context.Context, client.Object, ...client.CreateOption) error {
	return errors.New("create failed")
}

func TestEmitK8sEvent_ReportsCreateFailure(t *testing.T) {
	var output bytes.Buffer
	audit := &AuditLogger{
		Client: failingCreateClient{},
		logger: slog.New(slog.NewTextHandler(&output, nil)),
	}

	audit.emitK8sEvent(context.Background(), AuditEvent{
		Action:   AuditCreate,
		Resource: "LLMInferenceService/default/model",
	})

	require.Contains(t, output.String(), "failed to create Kubernetes audit event")
	require.Contains(t, output.String(), "create failed")
}

func TestEmitK8sEvent_UsesAuditedResourceIdentity(t *testing.T) {
	capture := &capturingCreateClient{}
	audit := &AuditLogger{Client: capture, logger: slog.Default()}
	audit.emitK8sEvent(context.Background(), AuditEvent{
		Action:   AuditCreate,
		Resource: "LLMInferenceService/tenant-a/model",
	})

	require.NotNil(t, capture.event)
	require.Equal(t, "tenant-a", capture.event.Namespace)
	require.Equal(t, "LLMInferenceService", capture.event.InvolvedObject.Kind)
	require.Equal(t, "model", capture.event.InvolvedObject.Name)
}

func TestEmitToStructuredLog_IncludesDetails(t *testing.T) {
	var output bytes.Buffer
	audit := &AuditLogger{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	audit.emitToStructuredLog(context.Background(), AuditEvent{
		Action:  AuditModelAccess,
		Details: map[string]string{"tenant_id": "tenant-a"},
	})

	require.Contains(t, output.String(), "tenant_id")
	require.Contains(t, output.String(), "tenant-a")
}

func TestNewAuditLogger_RejectsInvalidOTLPEndpoint(t *testing.T) {
	_, err := NewAuditLoggerWithOptionsAndEndpoint(nil, nil, false, "otel.example.test")
	require.ErrorContains(t, err, "absolute http(s) URL")
}

func TestLogReceipt_RuntimeToSpec_EmitsOnlyUnverifiedContentCommitment(t *testing.T) {
	var output bytes.Buffer
	audit := &AuditLogger{
		logger:         slog.New(slog.NewJSONHandler(&output, nil)),
		evidenceHealth: &EvidenceHealthMonitor{},
	}
	audit.LogReceipt(context.Background(), "exec-1", "ok", "secret model output", map[string]string{"prompt": "secret prompt"})

	require.NotContains(t, output.String(), "secret model output")
	require.NotContains(t, output.String(), "secret prompt")
	require.Contains(t, output.String(), "evidence.commitment")
	require.Contains(t, output.String(), "unverified")
	require.Contains(t, output.String(), "cryptographic receipt unavailable")
}

func TestLogVerifiedReceiptSequence_SpecToRuntime_WiresFailureToReadiness(t *testing.T) {
	key := healthTestKey(7)
	receipt := healthReceipt(t, key, "received", 1, "")
	audit := &AuditLogger{
		logger:         slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		evidenceHealth: &EvidenceHealthMonitor{},
	}
	verifier := healthVerifier(t, key)
	require.NoError(t, audit.LogVerifiedReceiptSequence(context.Background(), []string{"received"}, []provenance.EvidenceReceipt{receipt}, verifier))
	require.NoError(t, audit.EvidenceHealthCheck(nil))

	receipt.SubjectDigest = healthDigest("tampered")
	require.Error(t, audit.LogVerifiedReceiptSequence(context.Background(), []string{"received"}, []provenance.EvidenceReceipt{receipt}, verifier))
	require.Error(t, audit.EvidenceHealthCheck(nil))
}
