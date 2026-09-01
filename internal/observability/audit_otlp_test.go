/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuditLogger_ExportsOTLPLogRecord(t *testing.T) {
	var receivedPath string
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedPath = request.URL.Path
		received, _ = io.ReadAll(request.Body)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("CKODEX_AUDIT_LOG_PATH", filepath.Join(t.TempDir(), "audit.jsonl"))
	audit, err := NewAuditLoggerWithOptionsAndEndpoint(nil, nil, false, server.URL)
	require.NoError(t, err)

	audit.LogCreate(context.Background(), "LLMInferenceService/default/model", "controller", nil)
	flushContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, audit.Flush(flushContext))
	require.NoError(t, audit.Shutdown(flushContext))
	require.Equal(t, "/v1/logs", receivedPath)
	require.NotEmpty(t, received)
}

func TestAuditLogger_ReportsOTLPExportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	t.Setenv("CKODEX_AUDIT_LOG_PATH", filepath.Join(t.TempDir(), "audit.jsonl"))
	audit, err := NewAuditLoggerWithOptionsAndEndpoint(nil, nil, false, server.URL)
	require.NoError(t, err)
	audit.LogCreate(context.Background(), "LLMInferenceService/default/model", "controller", nil)

	flushContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.Error(t, audit.Flush(flushContext))
	require.Error(t, audit.Shutdown(flushContext))
}
