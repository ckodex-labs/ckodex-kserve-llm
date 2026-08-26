/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v2

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

func TestNewHealthChecker_DefaultTimeout(t *testing.T) {
	hc := NewHealthChecker("http://localhost:8080")
	require.NotNil(t, hc)
	assert.Equal(t, 5*time.Second, hc.client.httpClient.Timeout)
}

func TestCheckServerLive_OK(t *testing.T) {
	srv := serveJSON(t, ServerLiveResponse{Live: true})
	defer srv.Close()

	hc := NewHealthChecker(srv.URL, WithHTTPClient(srv.Client()))
	require.NoError(t, hc.CheckServerLive(context.Background()))
}

func TestCheckServerLive_NotLive_Error(t *testing.T) {
	srv := serveJSON(t, ServerLiveResponse{Live: false})
	defer srv.Close()

	hc := NewHealthChecker(srv.URL, WithHTTPClient(srv.Client()))
	err := hc.CheckServerLive(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not live")
}

func TestCheckServerLive_RequestError(t *testing.T) {
	hc := NewHealthChecker("http://127.0.0.1:19990")
	err := hc.CheckServerLive(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "liveness check failed")
}

func TestCheckServerReady_OK(t *testing.T) {
	srv := serveJSON(t, ServerReadyResponse{Ready: true})
	defer srv.Close()

	hc := NewHealthChecker(srv.URL, WithHTTPClient(srv.Client()))
	require.NoError(t, hc.CheckServerReady(context.Background()))
}

func TestCheckServerReady_NotReady_Error(t *testing.T) {
	srv := serveJSON(t, ServerReadyResponse{Ready: false})
	defer srv.Close()

	hc := NewHealthChecker(srv.URL, WithHTTPClient(srv.Client()))
	err := hc.CheckServerReady(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")
}

func TestCheckServerReady_RequestError(t *testing.T) {
	hc := NewHealthChecker("http://127.0.0.1:19991")
	err := hc.CheckServerReady(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "readiness check failed")
}

func TestCheckModelReady_OK(t *testing.T) {
	srv := serveJSON(t, ModelReadyResponse{Ready: true})
	defer srv.Close()

	hc := NewHealthChecker(srv.URL, WithHTTPClient(srv.Client()))
	require.NoError(t, hc.CheckModelReady(context.Background(), "llama3"))
}

func TestCheckModelReady_NotReady_Error(t *testing.T) {
	srv := serveJSON(t, ModelReadyResponse{Ready: false})
	defer srv.Close()

	hc := NewHealthChecker(srv.URL, WithHTTPClient(srv.Client()))
	err := hc.CheckModelReady(context.Background(), "llama3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")
}

func TestCheckModelReady_RequestError(t *testing.T) {
	hc := NewHealthChecker("http://127.0.0.1:19992")
	err := hc.CheckModelReady(context.Background(), "model")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "readiness check failed")
}

func TestIsHealthy_BothOK_True(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/v2/health/live":
			_ = json.NewEncoder(w).Encode(ServerLiveResponse{Live: true})
		case "/v2/health/ready":
			_ = json.NewEncoder(w).Encode(ServerReadyResponse{Ready: true})
		}
	}))
	defer srv.Close()

	hc := NewHealthChecker(srv.URL, WithHTTPClient(srv.Client()))
	assert.True(t, hc.IsHealthy(context.Background()))
}

func TestIsHealthy_LiveFails_False(t *testing.T) {
	srv := serveJSON(t, ServerLiveResponse{Live: false})
	defer srv.Close()

	hc := NewHealthChecker(srv.URL, WithHTTPClient(srv.Client()))
	assert.False(t, hc.IsHealthy(context.Background()))
}

func TestIsHealthy_ReadyFails_False(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/v2/health/live":
			_ = json.NewEncoder(w).Encode(ServerLiveResponse{Live: true})
		case "/v2/health/ready":
			_ = json.NewEncoder(w).Encode(ServerReadyResponse{Ready: false})
		}
	}))
	defer srv.Close()

	hc := NewHealthChecker(srv.URL, WithHTTPClient(srv.Client()))
	assert.False(t, hc.IsHealthy(context.Background()))
}

// ---- EncodeMultimodalInput -------------------------------------------------
