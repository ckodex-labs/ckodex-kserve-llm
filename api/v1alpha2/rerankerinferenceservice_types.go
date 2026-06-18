/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RerankerServerPort is the port on which the reranker exposes /rerank and /v1/rerank.
const RerankerServerPort = 8080

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=reranker
// +kubebuilder:printcolumn:name="Model",type="string",JSONPath=".spec.model.name"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RerankerInferenceService manages cross-encoder reranking models.
// It exposes /rerank and /v1/rerank endpoints compatible with the Cohere Rerank API.
// Backed by vLLM with --task score.
type RerankerInferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RerankerInferenceServiceSpec   `json:"spec,omitempty"`
	Status RerankerInferenceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RerankerInferenceServiceList contains a list of RerankerInferenceService.
type RerankerInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RerankerInferenceService `json:"items"`
}

// RerankerInferenceServiceSpec defines the desired state of a RerankerInferenceService.
type RerankerInferenceServiceSpec struct {
	// Model specifies the cross-encoder model weights (e.g. hf://BAAI/bge-reranker-v2-m3).
	Model ModelSpec `json:"model"`

	// Quantization configures weight quantization for memory efficiency.
	// +optional
	Quantization *QuantizationSpec `json:"quantization,omitempty"`

	// MaxCandidates caps the number of documents accepted per rerank request.
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	// +optional
	MaxCandidates int32 `json:"maxCandidates,omitempty"`

	// Resources overrides default CPU/memory/GPU allocation.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Replicas sets the number of serving pods.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

// RerankerInferenceServiceStatus defines the observed state of a RerankerInferenceService.
type RerankerInferenceServiceStatus struct {
	// Conditions represent the latest observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Endpoint is the /rerank URL once ready.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Replicas is the number of currently ready pods.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types for RerankerInferenceService.
const (
	// RerankerConditionReady indicates the service is ready to score document pairs.
	RerankerConditionReady = "Ready"

	// RerankerConditionDeploymentReady indicates the underlying Deployment is available.
	RerankerConditionDeploymentReady = "DeploymentReady"
)

func init() {
	SchemeBuilder.Register(&RerankerInferenceService{}, &RerankerInferenceServiceList{})
}
