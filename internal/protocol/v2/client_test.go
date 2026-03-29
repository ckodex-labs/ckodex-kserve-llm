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

func TestEncodeMultimodalInput_RawData(t *testing.T) {
	input := &MultimodalInput{
		Modality: ModalityImage,
		Data:     []byte{0xFF, 0xD8, 0xFF}, // JPEG magic bytes
		MIMEType: "image/jpeg",
	}

	out, err := EncodeMultimodalInput(input)
	require.NoError(t, err)
	assert.Equal(t, "image_input", out.Name)
	assert.Equal(t, DatatypeBYTES, out.Datatype)
	assert.Equal(t, "image/jpeg", out.Parameters["content_type"])
	assert.Equal(t, "image", out.Parameters["modality"])
}

func TestEncodeMultimodalInput_PreEncodedBase64(t *testing.T) {
	input := &MultimodalInput{
		Modality:   ModalityText,
		Base64Data: "aGVsbG8=", // "hello"
		MIMEType:   "text/plain",
	}

	out, err := EncodeMultimodalInput(input)
	require.NoError(t, err)
	assert.Equal(t, "text_input", out.Name)
	// Pre-encoded data is used as-is
	require.Len(t, out.Data.([]interface{}), 1)
	assert.Equal(t, "aGVsbG8=", out.Data.([]interface{})[0])
}

func TestEncodeMultimodalInput_EmptyData_Error(t *testing.T) {
	input := &MultimodalInput{Modality: ModalityImage}
	_, err := EncodeMultimodalInput(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data")
}

func TestEncodeMultimodalInput_DefaultShape(t *testing.T) {
	data := []byte("audio data")
	input := &MultimodalInput{
		Modality: ModalityAudio,
		Data:     data,
		MIMEType: "audio/wav",
		// No Shape — defaults to [1, len(data)]
	}

	out, err := EncodeMultimodalInput(input)
	require.NoError(t, err)
	assert.Equal(t, []int64{1, int64(len(data))}, out.Shape)
}

func TestEncodeMultimodalInput_CustomShape(t *testing.T) {
	input := &MultimodalInput{
		Modality: ModalityVideo,
		Data:     []byte("frame data"),
		MIMEType: "video/mp4",
		Shape:    []int64{1, 3, 224, 224},
	}

	out, err := EncodeMultimodalInput(input)
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 3, 224, 224}, out.Shape)
}

// ---- DecodeMultimodalOutput ------------------------------------------------

func TestDecodeMultimodalOutput_WithModality(t *testing.T) {
	encoded := "aGVsbG8=" // base64("hello")
	output := &InferOutput{
		Name:     "text_output",
		Shape:    []int64{1},
		Datatype: DatatypeBYTES,
		Data:     []interface{}{encoded},
		Parameters: map[string]interface{}{
			"modality":     "text",
			"content_type": "text/plain",
		},
	}

	result, err := DecodeMultimodalOutput(output)
	require.NoError(t, err)
	assert.Equal(t, ModalityText, result.Modality)
	assert.Equal(t, "text/plain", result.MIMEType)
	assert.Equal(t, []byte("hello"), result.Data)
}

func TestDecodeMultimodalOutput_DefaultModality(t *testing.T) {
	output := &InferOutput{
		Data:       []interface{}{""},
		Parameters: map[string]interface{}{},
	}

	result, err := DecodeMultimodalOutput(output)
	require.NoError(t, err)
	assert.Equal(t, ModalityText, result.Modality)
}

func TestDecodeMultimodalOutput_DefaultMIMEType(t *testing.T) {
	output := &InferOutput{
		Data:       []interface{}{""},
		Parameters: map[string]interface{}{},
	}

	result, err := DecodeMultimodalOutput(output)
	require.NoError(t, err)
	assert.Equal(t, "application/octet-stream", result.MIMEType)
}

func TestDecodeMultimodalOutput_InvalidBase64_Error(t *testing.T) {
	output := &InferOutput{
		Data:       []interface{}{"!!!not-valid-base64!!!"},
		Parameters: map[string]interface{}{},
	}

	_, err := DecodeMultimodalOutput(output)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode multimodal output")
}

func TestDecodeMultimodalOutput_EmptyData(t *testing.T) {
	output := &InferOutput{
		Data:       []interface{}{},
		Parameters: map[string]interface{}{},
	}

	result, err := DecodeMultimodalOutput(output)
	require.NoError(t, err)
	assert.Empty(t, result.Data)
}

// ---- EmbeddingRequest.ToV2InferRequest -------------------------------------

func TestToV2InferRequest_StringInput(t *testing.T) {
	req := &EmbeddingRequest{
		Model: "text-embed",
		Input: "hello world",
	}

	v2 := req.ToV2InferRequest()
	require.Len(t, v2.Inputs, 1)
	assert.Equal(t, "text", v2.Inputs[0].Name)
	assert.Equal(t, DatatypeBYTES, v2.Inputs[0].Datatype)
	assert.Equal(t, []int64{1}, v2.Inputs[0].Shape)
	assert.Equal(t, "hello world", v2.Inputs[0].Data.([]interface{})[0])
}

func TestToV2InferRequest_SliceInput(t *testing.T) {
	req := &EmbeddingRequest{
		Model: "text-embed",
		Input: []interface{}{"first", "second", "third"},
	}

	v2 := req.ToV2InferRequest()
	require.Len(t, v2.Inputs, 1)
	assert.Equal(t, []int64{3}, v2.Inputs[0].Shape)
	assert.Equal(t, "first", v2.Inputs[0].Data.([]interface{})[0])
}

func TestToV2InferRequest_ModelInParameters(t *testing.T) {
	req := &EmbeddingRequest{Model: "my-model", Input: "x"}
	v2 := req.ToV2InferRequest()
	assert.Equal(t, "my-model", v2.Parameters["model"])
}

func TestToV2InferRequest_UnknownInputType_EmptyData(t *testing.T) {
	req := &EmbeddingRequest{Model: "m", Input: 42} // int is not handled
	v2 := req.ToV2InferRequest()
	require.Len(t, v2.Inputs, 1)
	assert.Empty(t, v2.Inputs[0].Data)
}
