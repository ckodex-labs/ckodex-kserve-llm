/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=llmlora
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetService"
// +kubebuilder:printcolumn:name="Adapter",type="string",JSONPath=".spec.adapterName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LLMLoraAdapter is the Schema for the LLM LoRA Adapter API.
// It allows users to dynamically hot-swap fine-tuned weights (PEFT)
// into an active LLMInferenceService without downtime.
type LLMLoraAdapter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMLoraAdapterSpec   `json:"spec,omitempty"`
	Status LLMLoraAdapterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LLMLoraAdapterList contains a list of LLMLoraAdapter.
type LLMLoraAdapterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMLoraAdapter `json:"items"`
}

// LLMLoraAdapterSpec defines the desired state of LLMLoraAdapter.
type LLMLoraAdapterSpec struct {
	// TargetService is the name of the LLMInferenceService to attach this LoRA to.
	// The service must be in the same namespace.
	TargetService string `json:"targetService"`

	// AdapterName is the logical identifier used at inference time.
	// Example: 'sql-helper'
	// +kubebuilder:validation:MinLength=1
	AdapterName string `json:"adapterName"`

	// Model specifies the LoRA weights to serve.
	Model ModelSpec `json:"model"`
}

// LLMLoraAdapterStatus defines the observed state of LLMLoraAdapter.
type LLMLoraAdapterStatus struct {
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ActiveRevision tracks the generation that was successfully loaded.
	// +optional
	ActiveRevision int64 `json:"activeRevision,omitempty"`
}

const (
	// AdapterConditionReady indicates the adapter is successfully loaded in vLLM.
	AdapterConditionReady = "Ready"

	// AdapterConditionDownloaded indicates the weights are cached locally.
	AdapterConditionDownloaded = "Downloaded"

	// AdapterConditionRegistered indicates the control plane knows about the adapter.
	AdapterConditionRegistered = "Registered"
)

func init() {
	SchemeBuilder.Register(&LLMLoraAdapter{}, &LLMLoraAdapterList{})
}
