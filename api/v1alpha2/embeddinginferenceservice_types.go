/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EmbeddingRuntime selects the serving runtime for embedding models.
//
// +kubebuilder:validation:Enum=infinity;text-embeddings-inference
type EmbeddingRuntime string

const (
	// EmbeddingRuntimeInfinity uses the Infinity embedding server (michaelfeil/infinity-emb).
	// Supports any sentence-transformers-compatible model: BAAI/bge-*, intfloat/e5-*,
	// sentence-transformers/*, etc. Exposes the OpenAI-compatible /v1/embeddings endpoint.
	// Default runtime when spec.runtime is omitted.
	EmbeddingRuntimeInfinity EmbeddingRuntime = "infinity"

	// EmbeddingRuntimeTextEmbeddingsInference uses the HuggingFace TEI server.
	// Optimised for BERT-family encoder models with Metal / CUDA / CPU backends.
	// Exposes /embed (native) and /v1/embeddings (OpenAI-compatible via --json-output).
	EmbeddingRuntimeTextEmbeddingsInference EmbeddingRuntime = "text-embeddings-inference"
)

// DefaultEmbeddingRuntimeImage returns the default container image for a given runtime.
// An empty string means no public default exists — the user must set spec.runtimeImage.
func DefaultEmbeddingRuntimeImage(r EmbeddingRuntime) string {
	switch r {
	case EmbeddingRuntimeInfinity:
		return "michaelfeil/infinity-emb:latest-cpu"
	case EmbeddingRuntimeTextEmbeddingsInference:
		return "ghcr.io/huggingface/text-embeddings-inference:cpu-latest"
	default:
		return ""
	}
}

// EmbeddingServerPort is the port on which both runtimes expose the /v1/embeddings endpoint.
// Infinity defaults to 7997; TEI is configured to use the same port via --port flag.
const EmbeddingServerPort = 7997

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=embsvc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Runtime",type="string",JSONPath=".spec.runtime"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EmbeddingInferenceService manages the lifecycle of text embedding model workloads.
// It creates a Deployment + Service for the selected runtime and exposes the
// OpenAI-compatible /v1/embeddings endpoint.
//
// Two runtime modes are supported:
//   - infinity:                   for any sentence-transformers-compatible model (default)
//   - text-embeddings-inference:  for BERT-family encoder models with HF TEI
type EmbeddingInferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EmbeddingInferenceServiceSpec   `json:"spec,omitempty"`
	Status EmbeddingInferenceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EmbeddingInferenceServiceList contains a list of EmbeddingInferenceService.
type EmbeddingInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EmbeddingInferenceService `json:"items"`
}

// EmbeddingInferenceServiceSpec defines the desired state of an EmbeddingInferenceService.
type EmbeddingInferenceServiceSpec struct {
	// Model specifies the embedding model to serve.
	// For infinity, use hf://BAAI/bge-large-en-v1.5 or similar sentence-transformers model.
	// For text-embeddings-inference, use hf://BAAI/bge-large-en-v1.5 or similar BERT model.
	Model ModelSpec `json:"model"`

	// Runtime selects the serving runtime. Defaults to infinity.
	// +kubebuilder:default=infinity
	// +kubebuilder:validation:Enum=infinity;text-embeddings-inference
	// +optional
	Runtime EmbeddingRuntime `json:"runtime,omitempty"`

	// RuntimeImage overrides the default container image for the selected runtime.
	// When omitted, the operator uses the built-in default for the selected runtime.
	// +optional
	RuntimeImage string `json:"runtimeImage,omitempty"`

	// Replicas is the desired number of serving pods.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// BatchSize controls the maximum number of inputs processed in a single batch.
	// Larger batches improve throughput at the cost of latency. Defaults to 32.
	// +kubebuilder:default=32
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=2048
	// +optional
	BatchSize *int32 `json:"batchSize,omitempty"`

	// Scaling configures autoscaling behaviour (HPA / KEDA).
	// +optional
	Scaling *ScalingSpec `json:"scaling,omitempty"`

	// Template allows customising the pod template (resources, tolerations,
	// node selectors, additional sidecars, etc.).
	// The operator injects the primary runtime container at position 0.
	// Omit the template to use the operator-managed pod defaults.
	// +optional
	Template *corev1.PodTemplateSpec `json:"template,omitempty"`
}

// EmbeddingInferenceServiceStatus defines the observed state of an EmbeddingInferenceService.
type EmbeddingInferenceServiceStatus struct {
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// URL is the embeddings endpoint.
	// Example: http://my-emb.ckodex-inference.svc.cluster.local/v1/embeddings
	// +optional
	URL string `json:"url,omitempty"`

	// Replicas is the number of currently ready pods.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types for EmbeddingInferenceService.
const (
	// EmbeddingConditionReady indicates the service is ready to produce embeddings.
	EmbeddingConditionReady = "Ready"

	// EmbeddingConditionDeploymentReady indicates the underlying Deployment is available.
	EmbeddingConditionDeploymentReady = "DeploymentReady"
)

func init() {
	SchemeBuilder.Register(&EmbeddingInferenceService{}, &EmbeddingInferenceServiceList{})
}
