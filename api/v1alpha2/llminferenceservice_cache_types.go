/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import corev1 "k8s.io/api/core/v1"

// SpeculativeDecodingSpec configures speculative decoding (vLLM v0.24.0+).
type SpeculativeDecodingSpec struct {
	// Method selects the draft strategy: "mtp", "eagle", "medusa", "ngram".
	// +kubebuilder:validation:Enum=mtp;eagle;medusa;ngram
	Method string `json:"method"`

	// NumTokens is the number of speculative tokens per step (--spec-tokens).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=16
	// +optional
	NumTokens *int32 `json:"numTokens,omitempty"`

	// DraftModel URI for external drafter (eagle/medusa). Empty for mtp/ngram.
	// +optional
	DraftModel string `json:"draftModel,omitempty"`
}

// KVCacheSpec configures KV cache storage behavior.
type KVCacheSpec struct {
	// Dtype overrides KV cache type: "auto", "fp8", "fp16", "bf16".
	// "fp8" enables FP8 KV quantization (~50% memory reduction, requires Hopper+).
	// +kubebuilder:validation:Enum=auto;fp8;fp16;bf16
	// +kubebuilder:default=auto
	// +optional
	Dtype string `json:"dtype,omitempty"`

	// SwapSpaceGB sets CPU RAM offload in GiB (--cpu-offload-gb).
	// The JSON name is retained for API compatibility with earlier releases.
	// +kubebuilder:validation:Minimum=0
	// +optional
	SwapSpaceGB *int32 `json:"swapSpaceGB,omitempty"`

	// Transfer configures distributed KV-cache transfer for prefill/decode
	// disaggregation. The connector is passed to vLLM through
	// --kv-transfer-config and must be backed by a cluster-local data path.
	// +optional
	Transfer *KVTransferSpec `json:"transfer,omitempty"`
}

// KVTransferSpec configures the vLLM KV connector used by distributed serving.
// Connector names match the vLLM connector implementations: NixlConnector,
// LMCacheConnectorV1, or MooncakeConnector.
type KVTransferSpec struct {
	// Connector selects the transfer implementation.
	// +kubebuilder:validation:Enum=nixl;lmcache;mooncake
	Connector string `json:"connector"`

	// Role is kv_producer for prefill, kv_consumer for decode, or kv_both for
	// a combined worker. When omitted, the operator uses kv_both.
	// +kubebuilder:validation:Enum=kv_producer;kv_consumer;kv_both
	// +optional
	Role string `json:"role,omitempty"`

	// ExtraConfig is connector-specific JSON-compatible configuration. Values
	// are intentionally strings so the CRD remains portable across connector
	// versions and clusters.
	// +optional
	ExtraConfig map[string]string `json:"extraConfig,omitempty"`

	// Env adds connector-specific runtime environment variables to every
	// producer/consumer pod. This is primarily used by LMCache for
	// LMCACHE_CONFIG_FILE or backend credentials supplied through SecretKeyRef;
	// values already present in the pod template take precedence.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// LMCache provides a typed setup path. When omitted, the legacy connector,
	// extraConfig, and env behavior is unchanged.
	// +optional
	LMCache *LMCacheSpec `json:"lmcache,omitempty"`
}

// LMCacheMode selects the LMCache integration topology.
// +kubebuilder:validation:Enum=inProcess;multiprocess
type LMCacheMode string

const (
	LMCacheModeInProcess    LMCacheMode = "inProcess"
	LMCacheModeMultiprocess LMCacheMode = "multiprocess"
)

// LMCacheSpec configures either the in-process connector or an upstream
// LMCacheEngine multiprocess server.
type LMCacheSpec struct {
	// Mode defaults to inProcess when the typed block is present.
	// +kubebuilder:default=inProcess
	// +optional
	Mode LMCacheMode `json:"mode,omitempty"`

	// ChunkSize is the token chunk size used by the in-process connector.
	// +kubebuilder:default=256
	// +kubebuilder:validation:Minimum=1
	// +optional
	ChunkSize *int32 `json:"chunkSize,omitempty"`

	// LocalCPU enables the in-process local CPU cache.
	// +kubebuilder:default=true
	// +optional
	LocalCPU *bool `json:"localCPU,omitempty"`

	// LocalCPUSizeGiB bounds the in-process local CPU cache.
	// +kubebuilder:default=20
	// +kubebuilder:validation:Minimum=1
	// +optional
	LocalCPUSizeGiB *int32 `json:"localCPUSizeGiB,omitempty"`

	// EngineRef names the LMCacheEngine whose <name>-connection ConfigMap is
	// consumed in multiprocess mode.
	// +optional
	EngineRef *corev1.LocalObjectReference `json:"engineRef,omitempty"`
}

// QuantizationSpec configures weight quantization for reduced memory footprint.
type QuantizationSpec struct {
	// Method selects the quantization algorithm.
	// "awq" and "gptq" require pre-quantized model weights.
	// "gguf" is rejected because no conformant LLM runtime is admitted for it.
	// "bitsandbytes" and "fp8" quantize at load time.
	// +kubebuilder:validation:Enum=awq;gptq;gguf;bitsandbytes;fp8
	Method string `json:"method"`

	// CheckpointPath is retained in the schema for compatibility. The active
	// runtime does not consume it and admission rejects non-empty values.
	// +optional
	CheckpointPath string `json:"checkpointPath,omitempty"`
}

// ScalingSpec configures autoscaling for the inference service.
