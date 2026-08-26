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

func TestEmit_ReportsUnavailableDirectOTLPExport(t *testing.T) {
	var output bytes.Buffer
	audit := &AuditLogger{
		logger:       slog.New(slog.NewTextHandler(&output, nil)),
		otelEndpoint: "https://otel.example.test",
	}
	audit.emit(context.Background(), AuditEvent{Action: AuditCreate})

	require.Contains(t, output.String(), "direct OTLP audit export is unavailable")
	require.Contains(t, output.String(), "https://otel.example.test")
}
