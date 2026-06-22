/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v2 "github.com/ckodex-labs/kserve-llm-operator/internal/protocol/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestPipeline_UsesWarmedEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/models/gpt2/infer" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		resp := v2.InferResponse{
			ModelName: "gpt2",
			Outputs: []v2.InferOutput{
				{
					Name:     "output",
					Datatype: v2.DatatypeBYTES,
					Shape:    []int64{1},
					Data:     []string{"warm route"},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	pipeline := NewRequestPipeline()
	endpoint := strings.TrimPrefix(server.URL, "http://")
	sessionID := "warm-session"
	pipeline.prefetcher.warmPaths.Store(sessionID, endpoint)

	resp, err := pipeline.Execute(context.Background(), &InferenceRequest{
		Model:      "gpt2",
		SessionID:  sessionID,
		Prompt:     "warm route",
		Candidates: []string{"unreachable:9000"},
	})
	require.NoError(t, err)

	assert.Equal(t, endpoint, resp.Endpoint)
	assert.True(t, resp.CacheHit)
	assert.Greater(t, resp.TotalLatency, 0*time.Nanosecond)
}
