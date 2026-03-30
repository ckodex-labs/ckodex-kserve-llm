/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	v2 "github.com/ckodex-labs/kserve-llm-operator/internal/protocol/v2"
	"github.com/stretchr/testify/assert"
)

func TestInferenceFullPipeline(t *testing.T) {
	// 1. Setup Mock V2 Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/models/gpt2/infer" {
			resp := v2.InferResponse{
				ModelName: "gpt2",
				Outputs: []v2.InferOutput{
					{
						Name:     "output",
						Datatype: v2.DatatypeBYTES,
						Shape:    []int64{1},
						Data:     []string{"The future of AI infrastructure is evidence-native and blazing fast."},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// 2. Initialize Pipeline
	pipeline := NewRequestPipeline()

	// manually inject the mock server into candidates for routing
	endpoint := server.URL[len("http://"):]

	req := &InferenceRequest{
		Model:      "gpt2",
		Prompt:     "The future of AI infrastructure is",
		Candidates: []string{endpoint},
		SessionID:  "test-session",
	}

	// 3. Execute Inference (Cold Start)
	ctx := context.Background()
	resp1, err := pipeline.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Cold Inference failed: %v", err)
	}

	// 4. Manually seed Semantic Cache for Warm Start demonstration
	pipeline.cache.StoreExact(ctx, req.Prompt, "The future of AI infrastructure is evidence-native and blazing fast.")

	// 5. Execute Inference (Warm Start - Semantic Cache Hit)
	resp2, err := pipeline.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Warm Inference failed: %v", err)
	}

	// 6. Validate result and print benchmarks
	assert.NotNil(t, resp1)
	assert.Equal(t, endpoint, resp1.Endpoint)
	assert.True(t, resp2.CacheHit, "Warm start should be a cache hit")
	assert.True(t, resp2.TotalLatency < resp1.TotalLatency, "Semantic Cache must be faster than Router")

	fmt.Printf("\n--- Inference Benchmarks ---\n")
	fmt.Printf("Phase: Cold Start (Hot-Path Router)\n")
	fmt.Printf("  Endpoint: %s\n", resp1.Endpoint)
	fmt.Printf("  Latency:  %v\n", resp1.TotalLatency)
	fmt.Printf("  CacheHit: %v\n", resp1.CacheHit)

	fmt.Printf("\nPhase: Warm Start (Zero-GPU Semantic Cache)\n")
	fmt.Printf("  Latency:  %v\n", resp2.TotalLatency)
	fmt.Printf("  CacheHit: %v\n", resp2.CacheHit)
	fmt.Printf("----------------------------\n")
}
