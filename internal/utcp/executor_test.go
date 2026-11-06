/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package utcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteTool_HTTP_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "secret-token", r.Header.Get("Authorization"))

		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "doc-123", body["document_id"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "parsed",
			"pages":  42,
		})
	}))
	defer ts.Close()

	tool := &Tool{
		Name:        "ocr_parser",
		Description: "Parses document via native HTTP endpoint",
		Inputs: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"document_id": map[string]any{"type": "string"},
			},
			"required": []string{"document_id"},
		},
		ToolCallTemplate: &ToolCallTemplate{
			CallTemplateType: TemplateTypeHTTP,
			URL:              ts.URL,
			HTTPMethod:       "POST",
			Headers: map[string]string{
				"Authorization": "secret-token",
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	inputs := map[string]any{"document_id": "doc-123"}
	receipt, err := ExecuteTool(ctx, tool, inputs)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	assert.Equal(t, "ocr_parser", receipt.ToolName)
	assert.Equal(t, TemplateTypeHTTP, receipt.TemplateType)
	assert.Equal(t, http.StatusOK, receipt.StatusCode)
	assert.True(t, receipt.Duration > 0)

	outMap, ok := receipt.Output.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "parsed", outMap["status"])
	assert.Equal(t, float64(42), outMap["pages"])
}

func TestExecuteTool_CLI_Success(t *testing.T) {
	tool := &Tool{
		Name:        "echo_tool",
		Description: "Echoes message via CLI",
		Inputs:      map[string]any{"type": "object"},
		ToolCallTemplate: &ToolCallTemplate{
			CallTemplateType: TemplateTypeCLI,
			Command:          "echo",
			Args:             []string{"hello-utcp"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	receipt, err := ExecuteTool(ctx, tool, map[string]any{"test": true})
	require.NoError(t, err)
	require.NotNil(t, receipt)

	assert.Equal(t, "echo_tool", receipt.ToolName)
	assert.Equal(t, TemplateTypeCLI, receipt.TemplateType)
	assert.Contains(t, receipt.Output, "hello-utcp")
}

func TestExecuteTool_Failures(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_tool", func(t *testing.T) {
		receipt, err := ExecuteTool(ctx, nil, nil)
		assert.Error(t, err)
		assert.Nil(t, receipt)
	})

	t.Run("missing_template", func(t *testing.T) {
		receipt, err := ExecuteTool(ctx, &Tool{Name: "bad"}, nil)
		assert.Error(t, err)
		assert.Nil(t, receipt)
	})

	t.Run("unsupported_template", func(t *testing.T) {
		tool := &Tool{
			Name: "ws_tool",
			ToolCallTemplate: &ToolCallTemplate{
				CallTemplateType: TemplateTypeWebSocket,
			},
		}
		receipt, err := ExecuteTool(ctx, tool, nil)
		assert.Error(t, err)
		assert.Nil(t, receipt)
	})
}
