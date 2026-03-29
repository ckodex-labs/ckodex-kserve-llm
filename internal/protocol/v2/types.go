/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package v2 implements the Open Inference Protocol (V2) client
// with Binary Tensor Data Extension support.
//
// Specification: https://kserve.github.io/website/docs/concepts/architecture/data-plane/v2-protocol
// Extension: https://kserve.github.io/website/docs/concepts/architecture/data-plane/v2-protocol/binary-tensor-data-extension
package v2

// ----- Tensor Data Types -----

// Datatype constants for V2 protocol tensor elements.
const (
	DatatypeBOOL   = "BOOL"
	DatatypeUINT8  = "UINT8"
	DatatypeUINT16 = "UINT16"
	DatatypeUINT32 = "UINT32"
	DatatypeUINT64 = "UINT64"
	DatatypeINT8   = "INT8"
	DatatypeINT16  = "INT16"
	DatatypeINT32  = "INT32"
	DatatypeINT64  = "INT64"
	DatatypeFP16   = "FP16"
	DatatypeFP32   = "FP32"
	DatatypeFP64   = "FP64"
	DatatypeBF16   = "BF16"
	DatatypeBYTES  = "BYTES"
)

// DatatypeSize returns the byte size of a single element for the given datatype.
// Returns 0 for variable-length types (BYTES).
func DatatypeSize(dt string) int {
	switch dt {
	case DatatypeBOOL, DatatypeUINT8, DatatypeINT8:
		return 1
	case DatatypeUINT16, DatatypeINT16, DatatypeFP16, DatatypeBF16:
		return 2
	case DatatypeUINT32, DatatypeINT32, DatatypeFP32:
		return 4
	case DatatypeUINT64, DatatypeINT64, DatatypeFP64:
		return 8
	case DatatypeBYTES:
		return 0 // variable length
	default:
		return 0
	}
}

// ----- V2 Protocol Request/Response Types -----

// InferRequest is the V2 inference request payload.
// POST /v2/models/{model_name}[/versions/{version}]/infer
type InferRequest struct {
	// ID is an optional request identifier; echoed in the response.
	ID string `json:"id,omitempty"`

	// Parameters are optional request-level parameters.
	// The binary_data_output parameter (bool) requests all outputs as binary.
	Parameters map[string]any `json:"parameters,omitempty"`

	// Inputs are the input tensors.
	Inputs []InferInput `json:"inputs"`

	// Outputs optionally specifies which outputs to return.
	Outputs []RequestedOutput `json:"outputs,omitempty"`
}

// InferInput describes a single input tensor.
type InferInput struct {
	// Name of the input tensor.
	Name string `json:"name"`

	// Shape of the tensor as an array of dimensions.
	Shape []int64 `json:"shape"`

	// Datatype of the tensor elements (see Datatype constants).
	Datatype string `json:"datatype"`

	// Parameters are optional input-level parameters.
	// binary_data_size (int64) indicates the tensor is binary data of this size.
	Parameters map[string]any `json:"parameters,omitempty"`

	// Data contains the tensor data in JSON format (row-major order).
	// Omitted when using binary tensor data extension.
	Data any `json:"data,omitempty"`
}

// RequestedOutput describes a requested output tensor.
type RequestedOutput struct {
	// Name of the output tensor.
	Name string `json:"name"`

	// Parameters are optional output-level parameters.
	// binary_data (bool) requests this specific output as binary.
	Parameters map[string]any `json:"parameters,omitempty"`
}

// InferResponse is the V2 inference response payload.
type InferResponse struct {
	// ModelName that produced this response.
	ModelName string `json:"model_name"`

	// ModelVersion that produced this response.
	ModelVersion string `json:"model_version,omitempty"`

	// ID echoed from the request.
	ID string `json:"id,omitempty"`

	// Parameters are optional response-level parameters.
	Parameters map[string]any `json:"parameters,omitempty"`

	// Outputs are the output tensors.
	Outputs []InferOutput `json:"outputs"`
}

// InferOutput describes a single output tensor.
type InferOutput struct {
	// Name of the output tensor.
	Name string `json:"name"`

	// Shape of the tensor.
	Shape []int64 `json:"shape"`

	// Datatype of the tensor elements.
	Datatype string `json:"datatype"`

	// Parameters are optional output-level parameters.
	// binary_data_size (int64) indicates the output is binary of this size.
	Parameters map[string]any `json:"parameters,omitempty"`

	// Data contains the tensor data in JSON format.
	// Omitted when using binary tensor data extension.
	Data any `json:"data,omitempty"`
}

// ----- Health / Metadata Types -----

// ServerLiveResponse from GET /v2/health/live.
type ServerLiveResponse struct {
	Live bool `json:"live"`
}

// ServerReadyResponse from GET /v2/health/ready.
type ServerReadyResponse struct {
	Ready bool `json:"ready"`
}

// ModelReadyResponse from GET /v2/models/{name}/ready.
type ModelReadyResponse struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

// ServerMetadata from GET /v2.
type ServerMetadata struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Extensions []string `json:"extensions"`
}

// ModelMetadata from GET /v2/models/{name}[/versions/{version}].
type ModelMetadata struct {
	Name     string           `json:"name"`
	Versions []string         `json:"versions,omitempty"`
	Platform string           `json:"platform"`
	Inputs   []TensorMetadata `json:"inputs"`
	Outputs  []TensorMetadata `json:"outputs"`
}

// TensorMetadata describes an input or output tensor's schema.
type TensorMetadata struct {
	Name     string  `json:"name"`
	Datatype string  `json:"datatype"`
	Shape    []int64 `json:"shape"`
}

// V2Error is the standard error response for all V2 API failures.
type V2Error struct {
	Error string `json:"error"`
}

// ----- Binary Tensor Extension Constants -----
// https://kserve.github.io/website/docs/concepts/architecture/data-plane/v2-protocol/binary-tensor-data-extension

const (
	// ParamBinaryDataSize is the parameter key for binary tensor size (int64).
	// Used in input and output tensor parameters.
	ParamBinaryDataSize = "binary_data_size"

	// ParamBinaryData is the parameter key to request binary output (bool).
	// Used in requested output parameters.
	ParamBinaryData = "binary_data"

	// ParamBinaryDataOutput is the parameter key to request all outputs as binary (bool).
	// Used in request-level parameters.
	ParamBinaryDataOutput = "binary_data_output"

	// HeaderInferenceHeaderContentLength is the HTTP header containing the
	// length of the JSON portion when binary data is appended to the body.
	HeaderInferenceHeaderContentLength = "Inference-Header-Content-Length"
)
