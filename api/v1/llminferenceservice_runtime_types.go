/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

import corev1 "k8s.io/api/core/v1"

// ExperimentalSpec groups fields that were top-level in v1alpha2 but are not yet
// stable. Moving them here preserves forward compatibility: v1 clients that don't
// use these features are unaffected; v1alpha2 clients continue to work via conversion.
type ExperimentalSpec struct {
	// Prefill configures disaggregated prefill workers.
	// Moved from spec.prefill in v1alpha2.
	// +optional
	Prefill *PrefillSpec `json:"prefill,omitempty"`

	// Worker configures worker nodes for multi-node distributed inference (LeaderWorkerSet).
	// Moved from spec.worker in v1alpha2.
	// +optional
	Worker *WorkerSpec `json:"worker,omitempty"`

	// KVCache configures distributed KV transfer for prefill/decode serving.
	// It remains experimental until connector APIs stabilize across runtimes.
	// +optional
	KVCache *KVCacheSpec `json:"kvCache,omitempty"`

	// SpeculativeDecoding configures speculative decoding for vLLM.
	// It remains experimental while runtime support evolves.
	// +optional
	SpeculativeDecoding *SpeculativeDecodingSpec `json:"speculativeDecoding,omitempty"`

	// Quantization configures weight quantization.
	// +optional
	Quantization *QuantizationSpec `json:"quantization,omitempty"`

	// Engine selects the inference engine.
	// +kubebuilder:validation:Enum=sglang;vllm
	// +optional
	Engine string `json:"engine,omitempty"`

	// ToolSurface declares reachable APIs and external connectors.
	// +optional
	ToolSurface *ToolSurface `json:"toolSurface,omitempty"`

	// Observability configures telemetry sinks.
	// +optional
	Observability *ObservabilitySpec `json:"observability,omitempty"`
}

type KVCacheSpec struct {
	Dtype       string          `json:"dtype,omitempty"`
	SwapSpaceGB *int32          `json:"swapSpaceGB,omitempty"`
	Transfer    *KVTransferSpec `json:"transfer,omitempty"`
}

type KVTransferSpec struct {
	Connector   string            `json:"connector"`
	Role        string            `json:"role,omitempty"`
	ExtraConfig map[string]string `json:"extraConfig,omitempty"`
	// Env adds connector-specific runtime environment variables to every
	// producer/consumer pod. Existing template values take precedence.
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

// SpeculativeDecodingSpec configures speculative decoding.
type SpeculativeDecodingSpec struct {
	Method     string `json:"method"`
	NumTokens  *int32 `json:"numTokens,omitempty"`
	DraftModel string `json:"draftModel,omitempty"`
}

// QuantizationSpec configures weight quantization.
type QuantizationSpec struct {
	Method string `json:"method"`
	// CheckpointPath is retained in the schema for compatibility. The active
	// runtime does not consume it and admission rejects non-empty values.
	CheckpointPath string `json:"checkpointPath,omitempty"`
}

// ObservabilitySpec configures telemetry sinks for the inference service.
type ObservabilitySpec struct {
	Sink *TelemetrySink `json:"sink,omitempty"`
}

// TelemetrySink identifies a telemetry destination.
type TelemetrySink struct {
	Type     string `json:"type"`
	Endpoint string `json:"endpoint,omitempty"`
}

// ToolSurface defines the network surface reachable by the workload.
type ToolSurface struct {
	AllowedAPIs  []string `json:"allowedApis,omitempty"`
	AllowedCIDRs []string `json:"allowedCidrs,omitempty"`
}
