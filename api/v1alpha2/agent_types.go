/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelCapability defines a model's supported capabilities.
type ModelCapability string

const (
	CapabilityChat      ModelCapability = "chat"
	CapabilityEmbedding ModelCapability = "embedding"
	CapabilityVision    ModelCapability = "vision"
	CapabilityAudio     ModelCapability = "audio"
	CapabilityToolUse   ModelCapability = "tool-use"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=agent
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// Agent defines an AI agent with model bindings and tool capabilities.
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentConfiguration `json:"spec,omitempty"`
	Status AgentStatus        `json:"status,omitempty"`
}

// AgentConfiguration defines the desired state of an Agent.
type AgentConfiguration struct {
	// Identity defines the agent's name, description, and version.
	Identity AgentIdentity `json:"identity"`

	// ModelRef references the LLMInferenceService to use for inference.
	ModelRef string `json:"modelRef"`

	// Skills lists skill references from a SkillRegistry.
	// +optional
	Skills []SkillRef `json:"skills,omitempty"`

	// Tools lists function-calling tool definitions.
	// +optional
	Tools []ToolDefinition `json:"tools,omitempty"`

	// MaxTokens is the maximum token budget per agent invocation.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxTokens int32 `json:"maxTokens,omitempty"`

	// Template defines optional pod overrides for agent-specific containers.
	// +optional
	Template *corev1.PodTemplateSpec `json:"template,omitempty"`
}

// AgentIdentity defines the agent's identity.
type AgentIdentity struct {
	// Name is the agent's display name.
	Name string `json:"name"`

	// Description is a human-readable description of the agent's purpose.
	// +optional
	Description string `json:"description,omitempty"`

	// Version is the semver version of this agent configuration.
	// +optional
	Version string `json:"version,omitempty"`
}

// SkillRef references a skill in a SkillRegistry.
type SkillRef struct {
	// RegistryRef is the name of the SkillRegistry.
	RegistryRef string `json:"registryRef"`

	// SkillName identifies the skill within the registry.
	SkillName string `json:"skillName"`

	// Version constrains the skill version (semver range).
	// +optional
	Version string `json:"version,omitempty"`
}

// ToolDefinition defines a tool for function calling.
type ToolDefinition struct {
	// Name identifies the tool.
	Name string `json:"name"`

	// Description explains what the tool does.
	Description string `json:"description"`

	// InputSchema is the JSON Schema defining the tool's input parameters.
	// +optional
	InputSchema string `json:"inputSchema,omitempty"`

	// Endpoint is the URL or service address for the tool.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// AgentStatus defines the observed state.
type AgentStatus struct {
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates whether the agent is operational.
	// +optional
	Ready bool `json:"ready"`
}

// +kubebuilder:object:root=true

// AgentList contains a list of Agent.
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SkillRegistry defines a catalog of versioned skills.
type SkillRegistry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkillRegistrySpec   `json:"spec,omitempty"`
	Status SkillRegistryStatus `json:"status,omitempty"`
}

// SkillRegistrySpec defines the registry contents.
type SkillRegistrySpec struct {
	// Entries lists the skills available in this registry.
	// +optional
	Entries []SkillEntry `json:"entries,omitempty"`
}

// SkillEntry defines a single skill in the registry.
type SkillEntry struct {
	// Name identifies the skill.
	Name string `json:"name"`

	// Version is the semver version of this skill.
	Version string `json:"version"`

	// Description explains what the skill does.
	Description string `json:"description"`

	// Endpoint is the service URL for the skill.
	Endpoint string `json:"endpoint"`

	// InputSchema is the JSON Schema defining the skill's input parameters.
	// +optional
	InputSchema string `json:"inputSchema,omitempty"`

	// Capabilities lists the model capabilities this skill requires.
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`
}

// SkillRegistryStatus defines the observed state.
type SkillRegistryStatus struct {
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// EntryCount is the total number of skills in the registry.
	// +optional
	EntryCount int32 `json:"entryCount"`
}

// +kubebuilder:object:root=true

// SkillRegistryList contains a list of SkillRegistry.
type SkillRegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SkillRegistry `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ModelOnboarding defines a declarative pipeline for model promotion.
type ModelOnboarding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelOnboardingSpec   `json:"spec,omitempty"`
	Status ModelOnboardingStatus `json:"status,omitempty"`
}

// ModelOnboardingSpec defines the onboarding pipeline.
type ModelOnboardingSpec struct {
	// ModelRef references the LLMInferenceService to onboard.
	ModelRef string `json:"modelRef"`

	// Stages defines the ordered stages of the onboarding pipeline.
	// +optional
	Stages []OnboardingStage `json:"stages,omitempty"`

	// RollbackOnFailure enables automatic rollback if a stage fails.
	// +kubebuilder:default=true
	// +optional
	RollbackOnFailure bool `json:"rollbackOnFailure,omitempty"`
}

// OnboardingStage defines a single stage in the onboarding pipeline.
type OnboardingStage struct {
	// Name identifies this stage.
	Name string `json:"name"`

	// Type is the stage type.
	// +kubebuilder:validation:Enum=validation;canary;promotion;gate
	Type string `json:"type"`

	// Gate defines promotion gate criteria.
	// +optional
	Gate *GateCriteria `json:"gate,omitempty"`
}

// GateCriteria defines pass/fail criteria for a promotion gate.
type GateCriteria struct {
	// MinSuccessRate is the minimum success rate percentage.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	MinSuccessRate int32 `json:"minSuccessRate"`

	// MaxLatencyP99 is the maximum P99 latency in milliseconds.
	// +optional
	MaxLatencyP99 *int64 `json:"maxLatencyP99,omitempty"`
}

// ModelOnboardingStatus defines the observed state.
type ModelOnboardingStatus struct {
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// CurrentStage is the name of the currently executing stage.
	// +optional
	CurrentStage string `json:"currentStage,omitempty"`

	// Phase is the overall pipeline phase.
	// +kubebuilder:validation:Enum=Pending;InProgress;Completed;Failed;RolledBack
	// +optional
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true

// ModelOnboardingList contains a list of ModelOnboarding.
type ModelOnboardingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelOnboarding `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&Agent{}, &AgentList{},
		&SkillRegistry{}, &SkillRegistryList{},
		&ModelOnboarding{}, &ModelOnboardingList{},
	)
}
