/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client implements the V2 Open Inference Protocol HTTP API.
// Supports both JSON and binary tensor payloads.
type Client struct {
	baseURL    string
	httpClient *http.Client
	encoder    *BinaryEncoder
}

// NewClient creates a new V2 protocol HTTP client.
func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		encoder: NewBinaryEncoder(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ClientOption configures the V2 client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// ----- Health APIs -----

// ServerLive checks if the inference server is live.
// GET /v2/health/live
func (c *Client) ServerLive(ctx context.Context) (bool, error) {
	var resp ServerLiveResponse
	if err := c.get(ctx, "/v2/health/live", &resp); err != nil {
		return false, err
	}
	return resp.Live, nil
}

// ServerReady checks if the inference server is ready.
// GET /v2/health/ready
func (c *Client) ServerReady(ctx context.Context) (bool, error) {
	var resp ServerReadyResponse
	if err := c.get(ctx, "/v2/health/ready", &resp); err != nil {
		return false, err
	}
	return resp.Ready, nil
}

// ModelReady checks if a specific model is ready.
// GET /v2/models/{model_name}/ready
func (c *Client) ModelReady(ctx context.Context, modelName string) (bool, error) {
	var resp ModelReadyResponse
	if err := c.get(ctx, fmt.Sprintf("/v2/models/%s/ready", modelName), &resp); err != nil {
		return false, err
	}
	return resp.Ready, nil
}

// ----- Metadata APIs -----

// GetServerMetadata retrieves server metadata.
// GET /v2
func (c *Client) GetServerMetadata(ctx context.Context) (*ServerMetadata, error) {
	var resp ServerMetadata
	if err := c.get(ctx, "/v2", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetModelMetadata retrieves model metadata.
// GET /v2/models/{model_name}[/versions/{version}]
func (c *Client) GetModelMetadata(ctx context.Context, modelName string, version string) (*ModelMetadata, error) {
	path := fmt.Sprintf("/v2/models/%s", modelName)
	if version != "" {
		path = fmt.Sprintf("%s/versions/%s", path, version)
	}
	var resp ModelMetadata
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ----- Inference API -----

// Infer performs model inference using JSON payloads.
// POST /v2/models/{model_name}[/versions/{version}]/infer
func (c *Client) Infer(ctx context.Context, modelName string, version string, req *InferRequest) (*InferResponse, error) {
	path := fmt.Sprintf("/v2/models/%s", modelName)
	if version != "" {
		path = fmt.Sprintf("%s/versions/%s", path, version)
	}
	path += "/infer"

	var resp InferResponse
	if err := c.post(ctx, path, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// InferWithBinary performs model inference with binary tensor data.
// The request inputs that have binary_data_size set will have their
// binary data appended after the JSON body, per the Binary Tensor
// Data Extension specification.
func (c *Client) InferWithBinary(ctx context.Context, modelName string, version string, req *InferRequest, binaryData []byte) (*InferResponse, []byte, error) {
	path := fmt.Sprintf("/v2/models/%s", modelName)
	if version != "" {
		path = fmt.Sprintf("%s/versions/%s", path, version)
	}
	path += "/infer"

	// Marshal JSON portion
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal inference request: %w", err)
	}

	// Construct body: JSON + binary data
	var body []byte
	body = append(body, jsonBody...)
	body = append(body, binaryData...)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set(HeaderInferenceHeaderContentLength, strconv.Itoa(len(jsonBody)))

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		var v2Err V2Error
		if jsonErr := json.Unmarshal(respBody, &v2Err); jsonErr == nil && v2Err.Error != "" {
			return nil, nil, fmt.Errorf("inference error (HTTP %d): %s", httpResp.StatusCode, v2Err.Error)
		}
		return nil, nil, fmt.Errorf("inference error (HTTP %d): %s", httpResp.StatusCode, string(respBody))
	}

	// Parse response — check for binary data
	jsonLen := len(respBody)
	if hdr := httpResp.Header.Get(HeaderInferenceHeaderContentLength); hdr != "" {
		n, parseErr := strconv.Atoi(hdr)
		if parseErr == nil {
			jsonLen = n
		}
	}

	var inferResp InferResponse
	if err := json.Unmarshal(respBody[:jsonLen], &inferResp); err != nil {
		return nil, nil, fmt.Errorf("unmarshal inference response: %w", err)
	}

	var respBinaryData []byte
	if jsonLen < len(respBody) {
		respBinaryData = respBody[jsonLen:]
	}

	return &inferResp, respBinaryData, nil
}

// ----- HTTP helpers -----

func (c *Client) get(ctx context.Context, path string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var v2Err V2Error
		if jsonErr := json.Unmarshal(body, &v2Err); jsonErr == nil && v2Err.Error != "" {
			return fmt.Errorf("V2 error (HTTP %d): %s", resp.StatusCode, v2Err.Error)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return json.Unmarshal(body, result)
}

func (c *Client) post(ctx context.Context, path string, body any, result any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var v2Err V2Error
		if jsonErr := json.Unmarshal(respBody, &v2Err); jsonErr == nil && v2Err.Error != "" {
			return fmt.Errorf("V2 error (HTTP %d): %s", resp.StatusCode, v2Err.Error)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return json.Unmarshal(respBody, result)
}
