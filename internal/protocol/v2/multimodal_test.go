/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
