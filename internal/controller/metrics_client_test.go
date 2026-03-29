/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckodex-labs/kserve-llm-operator/internal/controller"
)

func skipIfNoTCP(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("TCP binding unavailable in this environment: %v", err)
	}
	_ = ln.Close()
}

// promResponse builds a minimal Prometheus /api/v1/query response with a single scalar.
func promResponse(value string) map[string]any {
	return map[string]any{
		"status": "success",
		"data": map[string]any{
			"resultType": "vector",
			"result": []map[string]any{
				{
					"metric": map[string]string{},
					"value":  []any{float64(1700000000), value},
				},
			},
		},
	}
}

// promEmptyResponse builds a Prometheus response with no matching time-series.
func promEmptyResponse() map[string]any {
	return map[string]any{
		"status": "success",
		"data": map[string]any{
			"resultType": "vector",
			"result":     []any{},
		},
	}
}

func servePrometheus(t *testing.T, body any, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(body)
	}))
}

// ---- PrometheusMetricsQuerier ------------------------------------------------

func TestPrometheusMetricsQuerier_QuerySuccessRate_Returns100(t *testing.T) {
	skipIfNoTCP(t)
	srv := servePrometheus(t, promResponse("98.7"), http.StatusOK)
	defer srv.Close()

	q := controller.NewPrometheusMetricsQuerier(srv.URL)
	rate, err := q.QuerySuccessRate(context.Background(), "llama3", "default")
	require.NoError(t, err)
	assert.InDelta(t, 98.7, rate, 0.01)
}

func TestPrometheusMetricsQuerier_QuerySuccessRate_EmptyResult_ReturnsZero(t *testing.T) {
	skipIfNoTCP(t)
	srv := servePrometheus(t, promEmptyResponse(), http.StatusOK)
	defer srv.Close()

	q := controller.NewPrometheusMetricsQuerier(srv.URL)
	rate, err := q.QuerySuccessRate(context.Background(), "new-model", "default")
	require.NoError(t, err)
	assert.Equal(t, float64(0), rate)
}

func TestPrometheusMetricsQuerier_QuerySuccessRate_HTTPError(t *testing.T) {
	skipIfNoTCP(t)
	srv := servePrometheus(t, map[string]any{"error": "server error"}, http.StatusInternalServerError)
	defer srv.Close()

	q := controller.NewPrometheusMetricsQuerier(srv.URL)
	_, err := q.QuerySuccessRate(context.Background(), "llama3", "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestPrometheusMetricsQuerier_QueryP99LatencyMS_ReturnsValue(t *testing.T) {
	skipIfNoTCP(t)
	// The PromQL includes "* 1000" so Prometheus returns the value already in ms.
	// The mock server returns 245 ms directly.
	srv := servePrometheus(t, promResponse("245"), http.StatusOK)
	defer srv.Close()

	q := controller.NewPrometheusMetricsQuerier(srv.URL)
	p99, err := q.QueryP99LatencyMS(context.Background(), "llama3", "default")
	require.NoError(t, err)
	assert.Equal(t, int64(245), p99)
}

func TestPrometheusMetricsQuerier_QueryP99LatencyMS_EmptyResult_ReturnsZero(t *testing.T) {
	skipIfNoTCP(t)
	srv := servePrometheus(t, promEmptyResponse(), http.StatusOK)
	defer srv.Close()

	q := controller.NewPrometheusMetricsQuerier(srv.URL)
	p99, err := q.QueryP99LatencyMS(context.Background(), "new-model", "default")
	require.NoError(t, err)
	assert.Equal(t, int64(0), p99)
}

func TestPrometheusMetricsQuerier_QuerySuccessRate_ContextCancelled(t *testing.T) {
	skipIfNoTCP(t)
	// Server that never responds — cancelled context should surface immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // block until client disconnects
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled immediately

	q := controller.NewPrometheusMetricsQuerier(srv.URL)
	_, err := q.QuerySuccessRate(ctx, "llama3", "default")
	require.Error(t, err)
}

func TestPrometheusMetricsQuerier_InvalidBaseURL(t *testing.T) {
	q := controller.NewPrometheusMetricsQuerier("://invalid-url")
	_, err := q.QuerySuccessRate(context.Background(), "llama3", "default")
	require.Error(t, err)
}

func TestPrometheusMetricsQuerier_PrometheusErrorStatus(t *testing.T) {
	skipIfNoTCP(t)
	body := map[string]any{
		"status":    "error",
		"errorType": "bad_data",
		"error":     "invalid query",
	}
	srv := servePrometheus(t, body, http.StatusOK)
	defer srv.Close()

	q := controller.NewPrometheusMetricsQuerier(srv.URL)
	_, err := q.QuerySuccessRate(context.Background(), "llama3", "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid query")
}
