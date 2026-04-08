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
// +kubebuilder:resource:shortName=llmconfig

// LLMInferenceServiceConfig defines reusable configuration templates
// that can be composed into LLMInferenceService resources.
// Merge order: WellKnown → BaseRefs → LLMInferenceService Spec.
type LLMInferenceServiceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LLMInferenceServiceConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// LLMInferenceServiceConfigList contains a list of LLMInferenceServiceConfig.
type LLMInferenceServiceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMInferenceServiceConfig `json:"items"`
}

// ComplianceProfile names a regulatory framework that constrains this config.
// +kubebuilder:validation:Enum=hipaa;soc2;fedramp
type ComplianceProfile string

const (
	// ComplianceHIPAA enforces HIPAA requirements:
	//   - EnableAuth=true (JWT required on all inference endpoints)
	//   - Model caching disabled (PHI must not be cached on disk)
	//   - Audit retention >= 7 years (2555 days)
	ComplianceHIPAA ComplianceProfile = "hipaa"

	// ComplianceSOC2 enforces SOC2 Type II requirements:
	//   - EnableSecurity=true (OPA + eBPF policy enforcement active)
	//   - Durable audit sink (not stdout)
	//   - PII redaction enabled
	ComplianceSOC2 ComplianceProfile = "soc2"

	// ComplianceFedRAMP enforces FedRAMP High requirements:
	//   - OPA image allowlist restricts to FedRAMP-authorized registries only
	//   - hf:// direct model downloads blocked (must use approved mirror)
	//   - EnableAuth=true, EnableSecurity=true
	ComplianceFedRAMP ComplianceProfile = "fedramp"
)

// LLMInferenceServiceConfigSpec defines shared configuration.
type LLMInferenceServiceConfigSpec struct {
	// Template provides default pod template settings.
	// +optional
	Template *corev1.PodTemplateSpec `json:"template,omitempty"`

	// Router provides default router settings.
	// +optional
	Router *RouterSpec `json:"router,omitempty"`

	// Scaling provides default scaling settings.
	// +optional
	Scaling *ScalingSpec `json:"scaling,omitempty"`

	// Parallelism provides default parallelism settings.
	// +optional
	Parallelism *ParallelismSpec `json:"parallelism,omitempty"`

	// Worker provides default worker spec for distributed inference.
	// +optional
	Worker *WorkerSpec `json:"worker,omitempty"`

	// VLLMDefaults provides default vLLM container configuration.
	// +optional
	VLLMDefaults *VLLMDefaultsSpec `json:"vllmDefaults,omitempty"`

	// ComplianceProfiles activates regulatory enforcement constraints.
	// Multiple profiles can be combined; constraints are additive.
	// Supported values: "hipaa", "soc2", "fedramp".
	// +optional
	ComplianceProfiles []ComplianceProfile `json:"complianceProfiles,omitempty"`
}

// VLLMDefaultsSpec defines default vLLM container configurations.
type VLLMDefaultsSpec struct {
	// Image is the default vLLM container image.
	// +kubebuilder:default="vllm/vllm-openai:latest"
	// +optional
	Image string `json:"image,omitempty"`

	// Args are default vLLM command-line arguments.
	// +optional
	Args []string `json:"args,omitempty"`

	// Resources are default resource requirements.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// EnableTurboQuant activates 6x KV cache compression for long-context stability.
	// Requires a vllm-turboquant compatible image.
	// +optional
	EnableTurboQuant bool `json:"enableTurboQuant,omitempty"`

	// TurboQuantMetadataPath is the path to the quantization metadata in the container.
	// +optional
	TurboQuantMetadataPath string `json:"turboquantMetadataPath,omitempty"`
}

func init() {
	SchemeBuilder.Register(&LLMInferenceServiceConfig{}, &LLMInferenceServiceConfigList{})
}
