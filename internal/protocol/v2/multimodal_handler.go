/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v2

import (
	"encoding/base64"
	"fmt"
)

// MultimodalInput represents a multimodal input for V2 inference.
type MultimodalInput struct {
	// Modality identifies the input type.
	Modality Modality `json:"modality"`
	// Data is the raw input data.
	Data []byte `json:"-"`
	// Base64Data is the base64-encoded data for JSON transport.
	Base64Data string `json:"data,omitempty"`
	// MIMEType is the content type (e.g., "image/png", "audio/wav").
	MIMEType string `json:"mimeType"`
	// Shape defines the tensor dimensions.
	Shape []int64 `json:"shape,omitempty"`
}

// Modality is the type of multimodal input.
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
)

// EncodeMultimodalInput converts raw data to an InferInput for V2 protocol.
func EncodeMultimodalInput(input *MultimodalInput) (*InferInput, error) {
	if len(input.Data) == 0 && input.Base64Data == "" {
		return nil, fmt.Errorf("multimodal input has no data")
	}

	// Encode data as base64 for JSON transport
	encoded := input.Base64Data
	if encoded == "" {
		encoded = base64.StdEncoding.EncodeToString(input.Data)
	}

	name := fmt.Sprintf("%s_input", input.Modality)
	shape := input.Shape
	if len(shape) == 0 {
		shape = []int64{1, int64(len(input.Data))} // [batch, bytes]
	}

	return &InferInput{
		Name:     name,
		Shape:    shape,
		Datatype: DatatypeBYTES,
		Parameters: map[string]interface{}{
			"content_type": input.MIMEType,
			"modality":     string(input.Modality),
		},
		Data: []interface{}{encoded},
	}, nil
}

// DecodeMultimodalOutput extracts multimodal data from V2 response.
func DecodeMultimodalOutput(output *InferOutput) (*MultimodalInput, error) {
	modality := ModalityText
	if m, ok := output.Parameters["modality"].(string); ok {
		modality = Modality(m)
	}

	mimeType := "application/octet-stream"
	if ct, ok := output.Parameters["content_type"].(string); ok {
		mimeType = ct
	}

	// Extract base64 data
	var base64Data string
	if dataSlice, ok := output.Data.([]interface{}); ok && len(dataSlice) > 0 {
		if s, ok := dataSlice[0].(string); ok {
			base64Data = s
		}
	}

	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("decode multimodal output: %w", err)
	}

	return &MultimodalInput{
		Modality:   modality,
		Data:       data,
		Base64Data: base64Data,
		MIMEType:   mimeType,
		Shape:      output.Shape,
	}, nil
}

// EmbeddingRequest represents an OpenAI-compatible /v1/embeddings request.
type EmbeddingRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"` // string or []string
}

// EmbeddingResponse represents an OpenAI-compatible /v1/embeddings response.
type EmbeddingResponse struct {
	Object string          `json:"object"` // "list"
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  EmbeddingUsage  `json:"usage"`
}

// EmbeddingData holds a single embedding vector.
type EmbeddingData struct {
	Object    string    `json:"object"` // "embedding"
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// EmbeddingUsage tracks token usage.
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ToV2InferRequest converts an EmbeddingRequest to a V2 InferRequest.
func (r *EmbeddingRequest) ToV2InferRequest() *InferRequest {
	var inputs []string
	switch v := r.Input.(type) {
	case string:
		inputs = []string{v}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				inputs = append(inputs, s)
			}
		}
	}

	data := make([]interface{}, len(inputs))
	for i, s := range inputs {
		data[i] = s
	}

	return &InferRequest{
		Inputs: []InferInput{
			{
				Name:     "text",
				Shape:    []int64{int64(len(inputs))},
				Datatype: DatatypeBYTES,
				Data:     data,
			},
		},
		Parameters: map[string]interface{}{
			"model": r.Model,
		},
	}
}
