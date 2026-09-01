/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/accessplane"
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

func TestRequestPipeline_ForwardPolicyDecisionSelectsExecutionModel(t *testing.T) {
	var calls atomic.Int32
	server := newPolicyTestServer(t, &calls, "policy-model")
	evaluator := newRequestPolicyEvaluator(t)
	pipeline, err := NewRequestPipelineWithPolicy(evaluator)
	require.NoError(t, err)

	req := policyTestRequest(server.URL, accessplane.RuntimeObservation{
		Models: map[string]accessplane.ModelObservation{
			"policy-model": {Ready: true},
		},
	})
	resp, err := pipeline.Execute(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, "policy-model", resp.Model)
	require.NotNil(t, resp.PolicyDecision)
	assert.Equal(t, accessplane.DispositionAdmit, resp.PolicyDecision.Disposition)
	assert.Equal(t, "target-eligible", resp.PolicyDecision.Reason)
}

func TestRequestPipeline_ReverseObservationChangesDecisionBeforeExecution(t *testing.T) {
	var calls atomic.Int32
	server := newPolicyTestServer(t, &calls, "policy-model")
	evaluator := newRequestPolicyEvaluator(t)
	pipeline, err := NewRequestPipelineWithPolicy(evaluator)
	require.NoError(t, err)

	admitted := policyTestRequest(server.URL, accessplane.RuntimeObservation{
		Models: map[string]accessplane.ModelObservation{
			"policy-model": {Ready: true},
		},
	})
	_, err = pipeline.Execute(context.Background(), admitted)
	require.NoError(t, err)

	backpressured := policyTestRequest(server.URL, accessplane.RuntimeObservation{
		Tenant: accessplane.TenantObservation{InFlight: 1},
		Models: map[string]accessplane.ModelObservation{
			"policy-model": {Ready: true},
		},
	})
	_, err = pipeline.Execute(context.Background(), backpressured)

	var policyErr *AccessPolicyError
	require.ErrorAs(t, err, &policyErr)
	assert.Equal(t, accessplane.DispositionBackpressure, policyErr.Decision.Disposition)
	assert.Equal(t, "tenant-concurrency-limit", policyErr.Decision.Reason)
	assert.Equal(t, 1, policyErr.Decision.QueueCapacityRemaining)
	assert.Equal(t, int32(1), calls.Load(), "backpressure must stop before endpoint execution")
}

func TestRequestPipeline_RejectsBeforeExecution(t *testing.T) {
	var calls atomic.Int32
	server := newPolicyTestServer(t, &calls, "policy-model")
	evaluator := newRequestPolicyEvaluator(t)
	pipeline, err := NewRequestPipelineWithPolicy(evaluator)
	require.NoError(t, err)

	req := policyTestRequest(server.URL, accessplane.RuntimeObservation{})
	req.TenantID = "unconfigured-tenant"
	_, err = pipeline.Execute(context.Background(), req)

	var policyErr *AccessPolicyError
	require.ErrorAs(t, err, &policyErr)
	assert.Equal(t, accessplane.DispositionReject, policyErr.Decision.Disposition)
	assert.Equal(t, "tenant-not-admitted", policyErr.Decision.Reason)
	assert.Equal(t, int32(0), calls.Load())
}

func TestRequestPipeline_PolicyHonorsDeadlineBeforeExecution(t *testing.T) {
	var calls atomic.Int32
	server := newPolicyTestServer(t, &calls, "policy-model")
	evaluator := newRequestPolicyEvaluator(t)
	pipeline, err := NewRequestPipelineWithPolicy(evaluator)
	require.NoError(t, err)
	req := policyTestRequest(server.URL, accessplane.RuntimeObservation{
		Models: map[string]accessplane.ModelObservation{
			"policy-model": {Ready: true},
		},
	})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err = pipeline.Execute(ctx, req)

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
	assert.Equal(t, int32(0), calls.Load())
}

func TestRequestPipeline_PolicyDoesNotMutateRequestOrObservation(t *testing.T) {
	var calls atomic.Int32
	server := newPolicyTestServer(t, &calls, "policy-model")
	evaluator := newRequestPolicyEvaluator(t)
	pipeline, err := NewRequestPipelineWithPolicy(evaluator)
	require.NoError(t, err)
	req := policyTestRequest(server.URL, accessplane.RuntimeObservation{
		Models: map[string]accessplane.ModelObservation{
			"policy-model": {Ready: true},
		},
	})
	original := *req
	original.Candidates = append([]string(nil), req.Candidates...)
	original.AccessObservation.Models = cloneModelObservations(req.AccessObservation.Models)

	_, err = pipeline.Execute(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, original, *req)
}

func TestRequestPipeline_DisabledPolicyPreservesExistingBehavior(t *testing.T) {
	var calls atomic.Int32
	server := newPolicyTestServer(t, &calls, "caller-model")
	pipeline := NewRequestPipeline()
	req := policyTestRequest(server.URL, accessplane.RuntimeObservation{})
	req.TenantID = ""
	req.Route = ""
	req.Model = "caller-model"

	resp, err := pipeline.Execute(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "caller-model", resp.Model)
	assert.Nil(t, resp.PolicyDecision)
	assert.Equal(t, int32(1), calls.Load())
}

func TestNewRequestPipelineWithPolicy_RejectsNilPolicy(t *testing.T) {
	pipeline, err := NewRequestPipelineWithPolicy(nil)

	assert.Nil(t, pipeline)
	assert.EqualError(t, err, "request pipeline policy is required")
}

func newRequestPolicyEvaluator(t *testing.T) *accessplane.Evaluator {
	t.Helper()
	evaluator, err := accessplane.NewEvaluator([]accessplane.TenantPolicy{
		{
			TenantID:      "tenant-a",
			MaxInFlight:   1,
			MaxQueueDepth: 1,
			Routes: map[string]accessplane.RoutePolicy{
				"chat": {
					Targets: []accessplane.TargetPolicy{
						{Model: "policy-model", MaxInFlight: 1},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return evaluator
}

func policyTestRequest(serverURL string, observed accessplane.RuntimeObservation) *InferenceRequest {
	return &InferenceRequest{
		TenantID:          "tenant-a",
		Route:             "chat",
		AccessObservation: observed,
		Model:             "caller-model",
		SessionID:         "policy-session",
		Prompt:            "policy request",
		Candidates:        []string{strings.TrimPrefix(serverURL, "http://")},
	}
}

func newPolicyTestServer(t *testing.T, calls *atomic.Int32, expectedModel string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "/v2/models/"+expectedModel+"/infer", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(v2.InferResponse{ModelName: expectedModel}))
	}))
	t.Cleanup(server.Close)
	return server
}

func cloneModelObservations(
	models map[string]accessplane.ModelObservation,
) map[string]accessplane.ModelObservation {
	cloned := make(map[string]accessplane.ModelObservation, len(models))
	for model, observed := range models {
		cloned[model] = observed
	}
	return cloned
}
