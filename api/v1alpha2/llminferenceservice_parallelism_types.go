/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// ParallelismSpec configures distributed inference parallelism.
type ParallelismSpec struct {
	// Tensor is the tensor parallelism degree — splits model layers across GPUs within a node.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Tensor *int32 `json:"tensor,omitempty"`

	// Data is the data parallelism degree — runs multiple model replicas.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Data *int32 `json:"data,omitempty"`

	// DataLocal controls GPUs per node for optimal NUMA affinity.
	// +kubebuilder:validation:Minimum=1
	// +optional
	DataLocal *int32 `json:"dataLocal,omitempty"`

	// Expert enables expert parallelism for MoE models.
	// When true, distributes Mixture-of-Experts across GPUs.
	// +optional
	Expert bool `json:"expert,omitempty"`

	// Pipeline is the pipeline parallelism degree (--pipeline-parallel-size).
	// Splits model layers sequentially across nodes. Combine with Tensor for 2D parallelism.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Pipeline *int32 `json:"pipeline,omitempty"`

	// EPLBEnabled enables Expert Parallelism Load Balancing for MoE models (--enable-eplb).
	// Dynamically rebalances expert routing — critical for DeepSeek-V4 and Gemma 4 MoE.
	// +optional
	EPLBEnabled bool `json:"eplbEnabled,omitempty"`
}

// SpeculativeDecodingSpec configures speculative decoding (vLLM v0.24.0+).
