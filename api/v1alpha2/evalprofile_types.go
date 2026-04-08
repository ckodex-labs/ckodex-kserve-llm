/*
Copyright 2026 CKodex Authors.
*/

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EvalProfileSpec defines the desired state of EvalProfile
type EvalProfileSpec struct {
	// PromptSet is a list of prompts used for benchmarking.
	// +optional
	PromptSet []string `json:"promptSet,omitempty"`

	// MandatoryMetrics defines the required score thresholds for approval.
	// +optional
	MandatoryMetrics map[string]float64 `json:"mandatoryMetrics,omitempty"`

	// TargetEngine specifies the engine to run the eval on (e.g., vllm, quant-cpp).
	// +optional
	TargetEngine string `json:"targetEngine,omitempty"`

	// MaxDurationSeconds is the maximum time allowed for the eval job.
	// +optional
	MaxDurationSeconds int32 `json:"maxDurationSeconds,omitempty"`
}

// EvalProfileStatus defines the observed state of EvalProfile
type EvalProfileStatus struct {
	// UsageCount is the number of times this profile has been used.
	UsageCount int32 `json:"usageCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EvalProfile is the Schema for the evalprofiles API
type EvalProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EvalProfileSpec   `json:"spec,omitempty"`
	Status EvalProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EvalProfileList contains a list of EvalProfile
type EvalProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EvalProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EvalProfile{}, &EvalProfileList{})
}
