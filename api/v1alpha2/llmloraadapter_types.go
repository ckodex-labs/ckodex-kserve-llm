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

	// Behavior defines the expected impact of this adapter on core behavior axes.
	// +optional
	Behavior *BehaviorMetadata `json:"behavior,omitempty"`

	// PolicyEnvelope constrains allowed loading and tool use for this adapter.
	// +optional
	PolicyEnvelope *PolicyEnvelope `json:"policyEnvelope,omitempty"`

	// ToolSurface declares reachable APIs and external connectors for this adapter.
	// +optional
	ToolSurface *ToolSurface `json:"toolSurface,omitempty"`

	// Sandbox enables header-based virtual routing for this adapter.
	// When enabled, requests with 'x-ckodex-adapter: <headerValue>' are
	// matched against this adapter.
	// +optional
	Sandbox *SandboxConfig `json:"sandbox,omitempty"`
}

// SandboxConfig defines virtual routing parameters for developer sandboxes.
type SandboxConfig struct {
	// Enable activates the sandbox route in the associated Gateway HTTPRoute.
	Enable bool `json:"enable,omitempty"`

	// HeaderValue is the value to match in the 'x-ckodex-adapter' header.
	// Example: 'sql-test-v1'
	HeaderValue string `json:"headerValue,omitempty"`
}

// BehaviorMetadata defines declared attributes of the adapter.
type BehaviorMetadata struct {
	// Safety level (0-10) where 10 is safest.
	Safety int `json:"safety,omitempty"`
	// Refusal fidelity (strength of system prompt adherence).
	Refusal string `json:"refusal,omitempty"`
	// ToolPropensity - declared intensity of tool usage ('conservative', 'moderate', 'aggressive').
	ToolPropensity string `json:"toolPropensity,omitempty"`
}

// PolicyEnvelope defines constraints on adapter usage.
type PolicyEnvelope struct {
	// Domain restrictions (e.g. 'coding', 'general').
	AllowedDomains []string `json:"allowedDomains,omitempty"`
	// Required trust levels for execution.
	MinTrustLevel string `json:"minTrustLevel,omitempty"`
}

// LLMLoraAdapterStatus defines the observed state of LLMLoraAdapter.
type LLMLoraAdapterStatus struct {
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ActiveRevision tracks the generation that was successfully loaded.
	// +optional
	ActiveRevision int64 `json:"activeRevision,omitempty"`

	// StatePlanes represents the governed composite state of the model system.
	// +optional
	StatePlanes StatePlanes `json:"statePlanes,omitempty"`

	// EvidenceBundle stores receipts, attestations and provenance data.
	// +optional
	EvidenceBundle EvidenceBundle `json:"evidenceBundle,omitempty"`
}

// StatePlanes represents orthogonal views of the system's state.
type StatePlanes struct {
	Lifecycle   string `json:"lifecycle,omitempty"`
	Trust       string `json:"trust,omitempty"`
	Binding     string `json:"binding,omitempty"`
	Composition string `json:"composition,omitempty"`
	Risk        string `json:"risk,omitempty"`
}

// EvidenceBundle stores verification data for the model system.
type EvidenceBundle struct {
	// SLSA attestation URI.
	AttestationURI string `json:"attestationUri,omitempty"`
	// Cosign signature digest.
	SignatureDigest string `json:"signatureDigest,omitempty"`
	// CycloneDX SBOM digest.
	SBOMDigest string `json:"sbomDigest,omitempty"`
	// Last verification timestamp.
	LastVerifiedAt *metav1.Time `json:"lastVerifiedAt,omitempty"`
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
