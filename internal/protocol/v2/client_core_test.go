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
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- test server helpers ---------------------------------------------------

func serveJSON(t *testing.T, v interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(v)
	}))
}

func serveError(statusCode int, message string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(message))
	}))
}

func serveV2Error(statusCode int, errMsg string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(V2Error{Error: errMsg})
	}))
}

// ---- NewClient / options ---------------------------------------------------

func TestNewClient_DefaultTimeout(t *testing.T) {
	c := NewClient("http://localhost:8080")
	require.NotNil(t, c)
	assert.Equal(t, 30*time.Second, c.httpClient.Timeout)
}

func TestNewClient_WithTimeout(t *testing.T) {
	c := NewClient("http://localhost:8080", WithTimeout(5*time.Second))
	assert.Equal(t, 5*time.Second, c.httpClient.Timeout)
}

func TestNewClient_WithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 1 * time.Second}
	c := NewClient("http://localhost:8080", WithHTTPClient(custom))
	assert.Equal(t, custom, c.httpClient)
}

func TestNewClient_BaseURLStored(t *testing.T) {
	c := NewClient("http://inference:8080")
	assert.Equal(t, "http://inference:8080", c.baseURL)
}

// ---- ServerLive ------------------------------------------------------------

func TestServerLive_True(t *testing.T) {
	srv := serveJSON(t, ServerLiveResponse{Live: true})
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	live, err := c.ServerLive(context.Background())
	require.NoError(t, err)
	assert.True(t, live)
}

func TestServerLive_False(t *testing.T) {
	srv := serveJSON(t, ServerLiveResponse{Live: false})
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	live, err := c.ServerLive(context.Background())
	require.NoError(t, err)
	assert.False(t, live)
}

func TestServerLive_ServerError(t *testing.T) {
	srv := serveError(http.StatusInternalServerError, "internal error")
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, err := c.ServerLive(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestServerLive_V2ErrorBody(t *testing.T) {
	srv := serveV2Error(http.StatusServiceUnavailable, "server overloaded")
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, err := c.ServerLive(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server overloaded")
}

// ---- ServerReady -----------------------------------------------------------

func TestServerReady_True(t *testing.T) {
	srv := serveJSON(t, ServerReadyResponse{Ready: true})
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	ready, err := c.ServerReady(context.Background())
	require.NoError(t, err)
	assert.True(t, ready)
}

func TestServerReady_ServerError(t *testing.T) {
	srv := serveError(http.StatusServiceUnavailable, "")
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, err := c.ServerReady(context.Background())
	require.Error(t, err)
}

// ---- ModelReady ------------------------------------------------------------

func TestModelReady_True(t *testing.T) {
	srv := serveJSON(t, ModelReadyResponse{Ready: true})
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	ready, err := c.ModelReady(context.Background(), "llama3")
	require.NoError(t, err)
	assert.True(t, ready)
}

func TestModelReady_ServerError(t *testing.T) {
	srv := serveError(http.StatusNotFound, "model not found")
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, err := c.ModelReady(context.Background(), "nonexistent")
	require.Error(t, err)
}

// ---- GetServerMetadata -----------------------------------------------------

func TestGetServerMetadata_OK(t *testing.T) {
	meta := ServerMetadata{Name: "vllm", Version: "0.4.0"}
	srv := serveJSON(t, meta)
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	got, err := c.GetServerMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "vllm", got.Name)
	assert.Equal(t, "0.4.0", got.Version)
}

func TestGetServerMetadata_Error(t *testing.T) {
	srv := serveError(http.StatusInternalServerError, "")
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, err := c.GetServerMetadata(context.Background())
	require.Error(t, err)
}

// ---- GetModelMetadata ------------------------------------------------------

func TestGetModelMetadata_NoVersion(t *testing.T) {
	meta := ModelMetadata{Name: "llama3"}
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(meta)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	got, err := c.GetModelMetadata(context.Background(), "llama3", "")
	require.NoError(t, err)
	assert.Equal(t, "llama3", got.Name)
	assert.Equal(t, "/v2/models/llama3", capturedPath)
}

func TestGetModelMetadata_WithVersion(t *testing.T) {
	meta := ModelMetadata{Name: "llama3"}
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(meta)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, err := c.GetModelMetadata(context.Background(), "llama3", "v1")
	require.NoError(t, err)
	assert.Equal(t, "/v2/models/llama3/versions/v1", capturedPath)
}

// ---- Infer -----------------------------------------------------------------

func TestInfer_NoVersion_Success(t *testing.T) {
	response := InferResponse{ModelName: "llama3", ID: "req-1"}
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	req := &InferRequest{}
	got, err := c.Infer(context.Background(), "llama3", "", req)
	require.NoError(t, err)
	assert.Equal(t, "llama3", got.ModelName)
	assert.Equal(t, "/v2/models/llama3/infer", capturedPath)
}

func TestInfer_WithVersion(t *testing.T) {
	response := InferResponse{ModelName: "llama3"}
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, err := c.Infer(context.Background(), "llama3", "v2", &InferRequest{})
	require.NoError(t, err)
	assert.Equal(t, "/v2/models/llama3/versions/v2/infer", capturedPath)
}

func TestInfer_ServerError(t *testing.T) {
	srv := serveV2Error(http.StatusBadRequest, "invalid input shape")
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, err := c.Infer(context.Background(), "llama3", "", &InferRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input shape")
}

// ---- InferWithBinary -------------------------------------------------------

func TestInferWithBinary_Success(t *testing.T) {
	response := InferResponse{ModelName: "llama3", ID: "bin-req-1"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/octet-stream", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	req := &InferRequest{}
	binaryData := []byte{0x01, 0x02, 0x03}
	got, respBinary, err := c.InferWithBinary(context.Background(), "llama3", "", req, binaryData)
	require.NoError(t, err)
	assert.Equal(t, "llama3", got.ModelName)
	assert.Nil(t, respBinary) // no binary trailer in response
}

func TestInferWithBinary_WithResponseBinaryData(t *testing.T) {
	response := InferResponse{ModelName: "llama3"}
	jsonBytes, _ := json.Marshal(response)
	binaryTrailer := []byte{0xDE, 0xAD}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HeaderInferenceHeaderContentLength, strconv.Itoa(len(jsonBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jsonBytes)
		_, _ = w.Write(binaryTrailer)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, respBinary, err := c.InferWithBinary(context.Background(), "llama3", "", &InferRequest{}, nil)
	require.NoError(t, err)
	assert.Equal(t, binaryTrailer, respBinary)
}

func TestInferWithBinary_ServerError(t *testing.T) {
	srv := serveV2Error(http.StatusUnprocessableEntity, "tensor shape mismatch")
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, _, err := c.InferWithBinary(context.Background(), "llama3", "", &InferRequest{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tensor shape mismatch")
}

func TestInferWithBinary_NonV2ErrorBody(t *testing.T) {
	srv := serveError(http.StatusBadGateway, "upstream timeout")
	defer srv.Close()

	c := NewClient(srv.URL, WithHTTPClient(srv.Client()))
	_, _, err := c.InferWithBinary(context.Background(), "llama3", "", &InferRequest{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

// ---- HealthChecker ---------------------------------------------------------
