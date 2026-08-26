/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=llmisvc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LLMInferenceService is the Schema for the LLM inference services API.
// It manages the full lifecycle of LLM inference workloads including
// deployment, routing, scheduling, and autoscaling.
type LLMInferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMInferenceServiceSpec   `json:"spec,omitempty"`
	Status LLMInferenceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LLMInferenceServiceList contains a list of LLMInferenceService.
type LLMInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMInferenceService `json:"items"`
}

// LLMInferenceServiceSpec defines the desired state of LLMInferenceService.
type LLMInferenceServiceSpec struct {
	// Model specifies the model to serve.
	Model ModelSpec `json:"model"`

	// Replicas is the number of model server replicas (decode workers).
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Parallelism configures distributed inference across GPUs/nodes.
	// +optional
	Parallelism *ParallelismSpec `json:"parallelism,omitempty"`

	// Scaling configures autoscaling behavior.
	// +optional
	Scaling *ScalingSpec `json:"scaling,omitempty"`

	// Template defines the pod template for model server pods.
	// The operator enforces "Guaranteed QoS" by synchronizing resource requests
	// to match limits. If TerminationGracePeriodSeconds is not specified, it
	// defaults to 30s for graceful model shutdown.
	Template corev1.PodTemplateSpec `json:"template"`

	// Prefill configures disaggregated prefill workers.
	// When set, the operator creates separate prefill pods that handle
	// the compute-intensive prefill phase independently. Prefill pods also
	// follow the "Guaranteed QoS" and 30s termination grace period patterns.
	// +optional
	Prefill *PrefillSpec `json:"prefill,omitempty"`

	// Worker configures worker nodes for multi-node distributed inference
	// using LeaderWorkerSet.
	// +optional
	Worker *WorkerSpec `json:"worker,omitempty"`

	// Router configures gateway, route, and scheduler for traffic management.
	Router RouterSpec `json:"router"`

	// BaseRefs is reserved for configuration composition. Non-empty values are
	// rejected until reference resolution is implemented.
	// +optional
	BaseRefs []ConfigReference `json:"baseRefs,omitempty"`

	// AutoOptimize is reserved for automatic hardware optimization. Non-nil
	// values are rejected until the hardware profile path is implemented.
	// +optional
	AutoOptimize *bool `json:"autoOptimize,omitempty"`

	// AllowedTenants is reserved for per-service tenant allow-list enforcement.
	// Non-empty values are rejected until the OPA admission path is wired.
	// +optional
	AllowedTenants []string `json:"allowedTenants,omitempty"`

	// CostAllocationTags are arbitrary key-value labels propagated to OTel metric
	// attributes, Deployment labels, and KEDA ScaledObject annotations so FinOps
	// tooling can group GPU-second and token costs by team, project, or cost-center.
	// +optional
	CostAllocationTags map[string]string `json:"costAllocationTags,omitempty"`

	// SLO is reserved for service-specific objective evaluation. Values are
	// rejected until targets are rendered and evaluated by the operator.
	// +optional
	SLO *SLOSpec `json:"slo,omitempty"`

	// Canary configures weighted traffic splitting between this (canary) service
	// and a stable base service. When set, the gateway reconciler produces an
	// HTTPRoute with two weighted backends instead of a single backend.
	// +optional
	Canary *CanarySpec `json:"canary,omitempty"`

	// SpeculativeDecoding configures speculative decoding for vLLM v0.24.0+.
	// MTP (Multi-Token Prediction) provides ~2× throughput on Llama/Mistral without quality loss.
	// +optional
	SpeculativeDecoding *SpeculativeDecodingSpec `json:"speculativeDecoding,omitempty"`

	// KVCache configures KV cache dtype and CPU offload for vLLM v0.24.0+.
	// Use Dtype:"fp8" for ~50% VRAM reduction on Hopper+ GPUs.
	// +optional
	KVCache *KVCacheSpec `json:"kvCache,omitempty"`

	// Quantization configures weight quantization for reduced memory footprint.
	// AWQ and GPTQ require pre-quantized model weights. GGUF routes to the
	// quant-cpp engine automatically. bitsandbytes and fp8 quantize at load time.
	// +optional
	Quantization *QuantizationSpec `json:"quantization,omitempty"`

	// Engine specifies the inference engine to use.
	// Defaults to 'vllm'. Supported: 'vllm', 'quant-cpp'.
	// +kubebuilder:default="vllm"
	// +kubebuilder:validation:Enum=vllm;quant-cpp
	// +optional
	Engine string `json:"engine,omitempty"`

	// ToolSurface declares reachable APIs and external connectors for this service.
	// +optional
	ToolSurface *ToolSurface `json:"toolSurface,omitempty"`

	// Observability configures telemetry sinks (logs, traces, metrics).
	// +optional
	Observability *ObservabilitySpec `json:"observability,omitempty"`
}

func init() {
	SchemeBuilder.Register(&LLMInferenceService{}, &LLMInferenceServiceList{})
}
