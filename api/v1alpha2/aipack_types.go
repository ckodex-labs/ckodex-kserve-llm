/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ArtifactKind identifies the semantic type of an AIPACK artifact per AIPACK-SPEC v0.1.1 §3.3.
// Each kind maps to exactly one ArtifactFamily per §3.5 (normative).
type ArtifactKind string

const (
	KindBaseModel      ArtifactKind = "BaseModel"      // A1 — model family
	KindLoRA           ArtifactKind = "LoRA"           // A2 — model family
	KindFineTune       ArtifactKind = "FineTune"       // A3 — model family
	KindSkill          ArtifactKind = "Skill"          // A4 — capability family
	KindTool           ArtifactKind = "Tool"           // A5 — capability family
	KindMCPServer      ArtifactKind = "MCPServer"      // A6 — capability family
	KindPromptTemplate ArtifactKind = "PromptTemplate" // A7 — control family
	KindGuardrail      ArtifactKind = "Guardrail"      // A8 — control family
	KindRetrievalIndex ArtifactKind = "RetrievalIndex" // A9 — knowledge family
	KindDataset        ArtifactKind = "Dataset"        // A10 — knowledge family
	KindHarness        ArtifactKind = "Harness"        // A11 — assurance family
	KindEval           ArtifactKind = "Eval"           // A12 — assurance family
	KindWorkflow       ArtifactKind = "Workflow"       // A13 — capability family
	KindPolicyBundle   ArtifactKind = "PolicyBundle"   // A14 — control family
	KindAgent          ArtifactKind = "Agent"          // C1  — composite family
)

// ArtifactFamily groups artifact kinds by semantic role per AIPACK-SPEC v0.1.1 §3.2.
type ArtifactFamily string

const (
	FamilyModel      ArtifactFamily = "model"
	FamilyCapability ArtifactFamily = "capability"
	FamilyControl    ArtifactFamily = "control"
	FamilyKnowledge  ArtifactFamily = "knowledge"
	FamilyAssurance  ArtifactFamily = "assurance"
	FamilyComposite  ArtifactFamily = "composite"
)

// AIPackSource identifies the OCI artifact by registry + digest.
// Tags alone are rejected (AIPACK-COMP-001): digest pinning is mandatory.
type AIPackSource struct {
	// Ref is a fully qualified OCI reference with sha256 digest.
	// Format: <registry>/<repository>@sha256:<64-hex-chars>
	// Tag-only references are rejected at webhook admission time.
	// +kubebuilder:validation:Pattern=`^.+@sha256:[0-9a-f]{64}$`
	Ref string `json:"ref"`

	// MediaType is the AIPACK manifest media type from §4.1.
	// Example: application/vnd.ai.basemodel.v1+json
	// When absent, it is inferred from Kind at validation time.
	// +optional
	MediaType string `json:"mediaType,omitempty"`
}

// AIPackComposition defines the slot-referenced component set for Agent artifacts (C1).
// Slots align with §5.3. All refs must be digest-pinned (AIPACK-COMP-001).
// The composed agent digest is computed via RFC 8785 canonical JSON sha256.
type AIPackComposition struct {
	// BaseModel is the primary model (BaseModel | LoRA | FineTune). At most one.
	// +optional
	BaseModel *AIPackRef `json:"baseModel,omitempty"`

	// Adapters are additional LoRA adapters stacked on the base model.
	// +optional
	Adapters []AIPackRef `json:"adapters,omitempty"`

	// Skills are skill artifacts registered in the agent's capability set.
	// +optional
	Skills []AIPackRef `json:"skills,omitempty"`

	// Tools are Tool or MCPServer artifacts the agent can invoke.
	// +optional
	Tools []AIPackRef `json:"tools,omitempty"`

	// MCPServers are MCP protocol server artifacts.
	// +optional
	MCPServers []AIPackRef `json:"mcpServers,omitempty"`

	// SystemPrompt is the PromptTemplate that seeds the agent's system context.
	// +optional
	SystemPrompt *AIPackRef `json:"systemPrompt,omitempty"`

	// GuardrailsInput are guardrails applied to the agent's input stream.
	// +optional
	GuardrailsInput []AIPackRef `json:"guardrailsInput,omitempty"`

	// GuardrailsOutput are guardrails applied to the agent's output stream.
	// +optional
	GuardrailsOutput []AIPackRef `json:"guardrailsOutput,omitempty"`

	// Retrieval is the RetrievalIndex artifact for RAG-style workflows.
	// +optional
	Retrieval *AIPackRef `json:"retrieval,omitempty"`

	// Workflow is the Workflow artifact orchestrating multi-step behaviour.
	// +optional
	Workflow *AIPackRef `json:"workflow,omitempty"`

	// Policy is the PolicyBundle artifact governing this agent's allowed composition.
	// +optional
	Policy *AIPackRef `json:"policy,omitempty"`

	// CompositeDigest is the RFC 8785 canonical JSON sha256 of the assembled manifest.
	// Set by the operator after composition validation succeeds.
	// +optional
	CompositeDigest string `json:"compositeDigest,omitempty"`
}

// AIPackRef is a pointer to another AIPack artifact by OCI digest reference.
type AIPackRef struct {
	// Ref is a fully qualified OCI digest reference (sha256 required).
	// +kubebuilder:validation:Pattern=`^.+@sha256:[0-9a-f]{64}$`
	Ref string `json:"ref"`

	// Kind is the expected ArtifactKind of the referenced artifact.
	// Used to validate slot compatibility at admission time.
	// +optional
	Kind ArtifactKind `json:"kind,omitempty"`
}

// AIPackPolicyRef links to the PolicyBundle artifact governing this artifact.
type AIPackPolicyRef struct {
	// Ref is a digest-pinned reference to the PolicyBundle artifact.
	// +kubebuilder:validation:Pattern=`^.+@sha256:[0-9a-f]{64}$`
	Ref string `json:"ref"`
}

// AIPackConditionType identifies a named condition on an AIPack.
type AIPackConditionType string

const (
	AIPackConditionReady          AIPackConditionType = "Ready"
	AIPackConditionAttested       AIPackConditionType = "Attested"
	AIPackConditionComposed       AIPackConditionType = "Composed"
	AIPackConditionPolicyApproved AIPackConditionType = "PolicyApproved"
	AIPackConditionRiskBand       AIPackConditionType = "RiskBand"
)

// RVBand is the risk valence band per AIPACK-SPEC v0.1.1 §13.3.
type RVBand string

const (
	RVBandGreen  RVBand = "GREEN"  // 0–24
	RVBandYellow RVBand = "YELLOW" // 25–49
	RVBandOrange RVBand = "ORANGE" // 50–74
	RVBandRed    RVBand = "RED"    // 75–100 — blocks composition without derogation
)

// AIPackStatus reflects the observed state of an AIPack artifact.
type AIPackStatus struct {
	// Conditions lists standard condition types for this artifact.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Family is the resolved ArtifactFamily as derived from Kind per §3.5.
	// Set by the operator; read-only.
	// +optional
	Family ArtifactFamily `json:"family,omitempty"`

	// RiskScore is the computed risk valence score (0–100) from §13.
	// Set by the operator after all 13 signals are evaluated.
	// +optional
	RiskScore *int32 `json:"riskScore,omitempty"`

	// RiskBand is the risk valence band derived from RiskScore.
	// +optional
	RiskBand RVBand `json:"riskBand,omitempty"`

	// DeprecationPhase is the current deprecation lifecycle phase per §16.
	// +optional
	DeprecationPhase string `json:"deprecationPhase,omitempty"`

	// ObservedGeneration is the generation observed by the operator.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aip
// +kubebuilder:printcolumn:name="Kind",type="string",JSONPath=".spec.kind"
// +kubebuilder:printcolumn:name="Family",type="string",JSONPath=".status.family"
// +kubebuilder:printcolumn:name="RiskBand",type="string",JSONPath=".status.riskBand"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"

// AIPack is the Schema for the aipacks API.
// It represents a single versioned AI artifact (model, skill, guardrail, agent, etc.)
// described by AIPACK-SPEC v0.1.1.
type AIPack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIPackSpec   `json:"spec,omitempty"`
	Status AIPackStatus `json:"status,omitempty"`
}

// AIPackSpec defines the desired state of an AIPack artifact.
type AIPackSpec struct {
	// Kind is the artifact kind per §3.3. Mandatory.
	// +kubebuilder:validation:Enum=BaseModel;LoRA;FineTune;Skill;Tool;MCPServer;PromptTemplate;Guardrail;RetrievalIndex;Dataset;Harness;Eval;Workflow;PolicyBundle;Agent
	Kind ArtifactKind `json:"kind"`

	// Family is the artifact family per §3.2. Optional.
	// When provided, it MUST match the canonical §3.5 mapping for Kind.
	// The operator rejects mismatches at admission (AIPACK-KIND-001).
	// +optional
	// +kubebuilder:validation:Enum=model;capability;control;knowledge;assurance;composite
	Family *ArtifactFamily `json:"family,omitempty"`

	// Source is the OCI artifact location. Mandatory.
	Source AIPackSource `json:"source"`

	// Composition is the slot-referenced component set. Required for Kind=Agent (C1).
	// Must be absent for all atomic kinds (A1–A14).
	// +optional
	Composition *AIPackComposition `json:"composition,omitempty"`

	// Attestation holds the artifact's attestation bundle.
	// Required for promotion to staging and above.
	// +optional
	Attestation *AIPackAttestation `json:"attestation,omitempty"`

	// Policy links to a PolicyBundle artifact governing this artifact's usage.
	// +optional
	Policy *AIPackPolicyRef `json:"policy,omitempty"`

	// BaseModel contains BaseModel-specific configuration (Kind=BaseModel).
	// +optional
	BaseModel *BaseModelSpec `json:"baseModel,omitempty"`

	// LoRA contains LoRA adapter configuration (Kind=LoRA).
	// +optional
	LoRA *LoRASpec `json:"lora,omitempty"`

	// FineTune contains fine-tuned model configuration (Kind=FineTune).
	// +optional
	FineTune *FineTuneSpec `json:"fineTune,omitempty"`

	// Skill contains skill artifact configuration (Kind=Skill).
	// +optional
	Skill *SkillSpec `json:"skill,omitempty"`

	// Tool contains tool artifact configuration (Kind=Tool).
	// +optional
	Tool *ToolSpec `json:"tool,omitempty"`

	// MCPServer contains MCP server configuration (Kind=MCPServer).
	// +optional
	MCPServer *MCPServerSpec `json:"mcpServer,omitempty"`

	// PromptTemplate contains prompt template configuration (Kind=PromptTemplate).
	// +optional
	PromptTemplate *PromptTemplateSpec `json:"promptTemplate,omitempty"`

	// Guardrail contains guardrail configuration (Kind=Guardrail).
	// +optional
	Guardrail *GuardrailSpec `json:"guardrail,omitempty"`

	// RetrievalIndex contains retrieval index configuration (Kind=RetrievalIndex).
	// +optional
	RetrievalIndex *RetrievalIndexSpec `json:"retrievalIndex,omitempty"`

	// Dataset contains dataset configuration (Kind=Dataset).
	// +optional
	Dataset *DatasetSpec `json:"dataset,omitempty"`

	// Harness contains eval harness configuration (Kind=Harness).
	// +optional
	Harness *HarnessSpec `json:"harness,omitempty"`

	// Eval contains eval configuration (Kind=Eval).
	// +optional
	Eval *EvalSpec `json:"eval,omitempty"`

	// Workflow contains workflow configuration (Kind=Workflow).
	// +optional
	Workflow *WorkflowSpec `json:"workflow,omitempty"`

	// PolicyBundleSpec contains policy bundle configuration (Kind=PolicyBundle).
	// +optional
	PolicyBundleSpec *PolicyBundleSpec `json:"policyBundle,omitempty"`
}

// +kubebuilder:object:root=true

// AIPackList contains a list of AIPack artifacts.
type AIPackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIPack `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIPack{}, &AIPackList{})
}
