/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// PromptTemplateSpec describes an A7 PromptTemplate artifact per AIPACK-SPEC v0.1.1 §3.3.
type PromptTemplateSpec struct {
	// Role declares where in the message sequence this template is applied.
	// Values: "system", "user", "assistant"
	// +kubebuilder:validation:Enum=system;user;assistant
	Role string `json:"role"`

	// Format describes the template format/engine.
	// Examples: "jinja2", "handlebars", "f-string", "mustache"
	// +optional
	Format string `json:"format,omitempty"`

	// Variables lists the expected variable names interpolated in the template.
	// +optional
	Variables []string `json:"variables,omitempty"`

	// JailbreakResistanceTested declares whether the template was tested for
	// jailbreak resistance. Backed by attestation urn:prompt:jailbreak-resistance:v1.
	// +optional
	JailbreakResistanceTested *bool `json:"jailbreakResistanceTested,omitempty"`

	// ContentHash is the sha256 of the rendered template content.
	// Backed by attestation urn:prompt:content-hash:v1.
	// +optional
	ContentHash string `json:"contentHash,omitempty"`

	// TokenBudget is the maximum number of tokens this template consumes when rendered.
	// +optional
	TokenBudget *int32 `json:"tokenBudget,omitempty"`
}

// GuardrailSpec describes an A8 Guardrail artifact per AIPACK-SPEC v0.1.1 §3.3.
type GuardrailSpec struct {
	// Direction declares whether this guardrail is applied to input, output, or both.
	// +kubebuilder:validation:Enum=input;output;both
	Direction string `json:"direction"`

	// Engine declares the guardrail evaluation engine.
	// Examples: "llm-guard", "nemo-guardrails", "presidio", "custom"
	// +optional
	Engine string `json:"engine,omitempty"`

	// FPRTarget is the declared maximum false positive rate (0.0–1.0).
	// Backed by attestation urn:guardrail:fpr:v1.
	// +optional
	FPRTarget *float64 `json:"fprTarget,omitempty"`

	// CoverageTest declares whether a coverage test has been executed.
	// Backed by attestation urn:guardrail:coverage-test:v1.
	// +optional
	CoverageTest *bool `json:"coverageTest,omitempty"`

	// LatencyBudgetMs is the maximum allowed evaluation latency in milliseconds.
	// +optional
	LatencyBudgetMs *int32 `json:"latencyBudgetMs,omitempty"`

	// BlockingMode declares whether this guardrail blocks or only logs violations.
	// +kubebuilder:validation:Enum=blocking;logging
	// +optional
	BlockingMode string `json:"blockingMode,omitempty"`
}

// PolicyBundleSpec describes an A14 PolicyBundle artifact per AIPACK-SPEC v0.1.1 §3.3 + §19.
type PolicyBundleSpec struct {
	// Engine declares the policy evaluation engine.
	// Examples: "opa", "cedar", "cue", "rego"
	// +optional
	Engine string `json:"engine,omitempty"`

	// ProfileID is the predefined profile URN this bundle implements.
	// Predefined profiles from §19.4:
	//   urn:aipack:profile:fedramp-moderate-ai:v1
	//   urn:aipack:profile:cmmc-l3-defense:v1
	//   urn:aipack:profile:eu-ai-act-high-risk:v1
	//   urn:aipack:profile:hipaa-clinical:v1
	// +optional
	ProfileID string `json:"profileID,omitempty"`

	// ForbiddenFamilies lists artifact families that are denied by this policy.
	// Evaluated in order: forbiddenFamilies → allowedFamilies → forbiddenArtifactTypes → allowedArtifactTypes.
	// +optional
	ForbiddenFamilies []ArtifactFamily `json:"forbiddenFamilies,omitempty"`

	// AllowedFamilies lists the only artifact families permitted.
	// When non-empty, acts as an allowlist. Empty (absent) means allow-all.
	// +optional
	AllowedFamilies []ArtifactFamily `json:"allowedFamilies,omitempty"`

	// ForbiddenArtifactTypes lists specific artifact kinds that are denied.
	// +optional
	ForbiddenArtifactTypes []ArtifactKind `json:"forbiddenArtifactTypes,omitempty"`

	// AllowedArtifactTypes lists the only artifact kinds permitted.
	// Empty array [] is a deny-all sentinel (not the same as absent).
	// +optional
	AllowedArtifactTypes []ArtifactKind `json:"allowedArtifactTypes,omitempty"`

	// RequiredPredicates lists predicate URNs that composing artifacts must carry.
	// +optional
	RequiredPredicates []string `json:"requiredPredicates,omitempty"`

	// MaxRiskBand is the maximum risk valence band permitted by this policy.
	// Artifacts with RiskBand > MaxRiskBand are denied (AIPACK-RV-002).
	// +optional
	// +kubebuilder:validation:Enum=GREEN;YELLOW;ORANGE;RED
	MaxRiskBand RVBand `json:"maxRiskBand,omitempty"`
}
