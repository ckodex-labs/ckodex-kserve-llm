/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=llmisvc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LLMInferenceService is the stable v1 API for LLM inference workloads.
// It manages the full lifecycle of a served LLM including deployment, routing,
// scheduling, and autoscaling.
//
// Compared to v1alpha2:
//   - spec.prefill and spec.worker are moved to spec.experimental.prefill / spec.experimental.worker
//   - All other fields are stable and forward-compatible
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

	// Replicas is the number of model server replicas.
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
	Template corev1.PodTemplateSpec `json:"template"`

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
	// Keys and values must match the Kubernetes label value regex: [a-zA-Z0-9_-./]+
	// +optional
	CostAllocationTags map[string]string `json:"costAllocationTags,omitempty"`

	// SLO is reserved for service-specific objective evaluation. Values are
	// rejected until targets are rendered and evaluated by the operator.
	// +optional
	SLO *SLOSpec `json:"slo,omitempty"`

	// Canary configures weighted traffic splitting between this (canary) service
	// and a stable base service.
	// +optional
	Canary *CanarySpec `json:"canary,omitempty"`

	// Experimental holds fields that were experimental in v1alpha2 and are retained
	// in v1 for workloads that need them, but are not part of the stable surface.
	// Fields in this sub-struct may change or be removed in future versions.
	// +optional
	Experimental *ExperimentalSpec `json:"experimental,omitempty"`
}

func init() {
	SchemeBuilder.Register(&LLMInferenceService{}, &LLMInferenceServiceList{})
}
