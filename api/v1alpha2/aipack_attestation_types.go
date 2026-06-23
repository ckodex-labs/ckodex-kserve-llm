/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// Attestation predicate URN constants per AIPACK-SPEC v0.1.1 §6 and §7.
// These are the normative predicate identifiers for cosign / SLSA / in-toto predicates.
const (
	// Universal — required for every artifact kind
	PredSLSAProvenance = "slsa.dev/provenance/v1"
	PredCycloneDXBOM   = "cyclonedx.org/bom"

	// A1 BaseModel
	PredAIBOM             = "urn:aibom:spec:v1.5"
	PredTrainingResidency = "urn:model:training-residency:v1"
	PredZKTrainingExcl    = "urn:zk:training-exclusion:v1"

	// A2 LoRA
	PredLoRABaseRef     = "urn:lora:base-ref:v1"
	PredLoRAHyperparams = "urn:lora:hyperparameters:v1"
	PredLoRALossCurve   = "urn:lora:loss-curve:v1"

	// A3 FineTune
	PredFineTuneHyperparams     = "urn:finetune:hyperparameters:v1"
	PredFineTuneBaseCompat      = "urn:finetune:base-compat:v1"
	PredFineTuneSafetyRetention = "urn:finetune:safety-retention:v1"

	// A4 Skill
	PredSkillSafetyReview          = "urn:skill:safety-review:v1"
	PredSkillCapabilityDeclaration = "urn:skill:capability-declaration:v1"
	PredSkillStaticAnalysis        = "urn:skill:static-analysis:v1"
	PredSkillRedTeam               = "urn:skill:red-team-eval:v1"

	// A5 Tool
	PredToolSchemaValidation = "urn:tool:schema-validation:v1"
	PredToolStaticAnalysis   = "urn:tool:static-analysis:v1"
	PredToolRedTeam          = "urn:tool:red-team-eval:v1"

	// A6 MCPServer
	PredMCPSandboxPolicy = "urn:mcp:sandbox-policy:v1"
	PredMCPToolList      = "urn:mcp:tool-list:v1"
	PredMCPRedTeam       = "urn:mcp:red-team-eval:v1"

	// A7 PromptTemplate
	PredPromptContentHash         = "urn:prompt:content-hash:v1"
	PredPromptJailbreakResistance = "urn:prompt:jailbreak-resistance:v1"
	PredPromptABTest              = "urn:prompt:ab-test:v1"

	// A8 Guardrail
	PredGuardrailCoverageTest = "urn:guardrail:coverage-test:v1"
	PredGuardrailFPR          = "urn:guardrail:fpr:v1"
	PredGuardrailRedTeam      = "urn:guardrail:red-team-eval:v1"
	PredZKGuardrailCoverage   = "urn:zk:guardrail-coverage:v1"

	// A9 RetrievalIndex
	PredRetrievalPIIScan  = "urn:retrieval:pii-scan:v1"
	PredRetrievalEmbedRef = "urn:retrieval:embedding-ref:v1"
	PredRetrievalRefresh  = "urn:retrieval:refresh-attestation:v1"

	// A10 Dataset
	PredDatasetProvenance       = "urn:dataset:provenance:v1"
	PredDatasetConsent          = "urn:dataset:consent:v1"
	PredDatasetLicensing        = "urn:dataset:licensing:v1"
	PredDatasetBiasAnalysis     = "urn:dataset:bias-analysis:v1"
	PredDatasetDeidentification = "urn:dataset:deidentification:v1"

	// A11 Harness
	PredHarnessMethodology     = "urn:eval-suite:methodology:v1"
	PredHarnessReproducibility = "urn:eval-suite:reproducibility:v1"
	PredHarnessRefOutputs      = "urn:eval-suite:reference-outputs:v1"

	// C1 Agent (composite)
	PredAgentComposition      = "urn:agent:composition:v1"
	PredAgentBehavioralEval   = "urn:agent:behavioral-eval:v1"
	PredAgentPolicyCompliance = "urn:agent:policy-compliance:v1"
	PredAgentScorecard        = "urn:scorecard:agent:v1"

	// Operational predicates (§11–§22)
	PredLineageEnvelope         = "urn:aipack:lineage-envelope:v1"
	PredLineageAttestation      = "urn:aipack:lineage-attestation:v1"
	PredBlastRadiusDeclaration  = "urn:aipack:blast-radius-declaration:v1"
	PredQuarantineCascade       = "urn:aipack:quarantine-cascade:v1"
	PredDependencyIndexSnapshot = "urn:aipack:dependency-index-snapshot:v1"
	PredRiskValence             = "urn:aipack:risk-valence:v1"
	PredOutlierSignal           = "urn:aipack:outlier-signal:v1"
	PredOutlierDismissal        = "urn:aipack:outlier-dismissal:v1"
	PredDeprecationNotice       = "urn:aipack:deprecation:v1"
	PredDeprecationRevocation   = "urn:aipack:deprecation-revocation:v1"
	PredSunsetDerogation        = "urn:aipack:sunset-derogation:v1"
	PredProfileDerogation       = "urn:aipack:profile-derogation:v1"
	PredAirGapBundle            = "urn:aipack:airgap-bundle:v1"
	PredAirGapHandoff           = "urn:aipack:airgap-handoff:v1"
	PredReplayLog               = "urn:aipack:replay-log:v1"
	PredQuarantineTrigger       = "urn:aipack:trigger-fired:v1"
	PredVADResult               = "urn:aipack:vad-result:v1"
)

// PredicateStatus is the verification status of a single attestation predicate.
type PredicateStatus string

const (
	PredicateStatusPresent  PredicateStatus = "Present"
	PredicateStatusMissing  PredicateStatus = "Missing"
	PredicateStatusInvalid  PredicateStatus = "Invalid"
	PredicateStatusExpired  PredicateStatus = "Expired"
	PredicateStatusVerified PredicateStatus = "Verified"
)

// PredicateEntry records a single predicate in the attestation bundle.
type PredicateEntry struct {
	// PredicateURI is the normative predicate identifier from §6.
	PredicateURI string `json:"predicateURI"`

	// Digest is the sha256 digest of the predicate payload.
	// +optional
	Digest string `json:"digest,omitempty"`

	// RekorLogID is the Rekor transparency log entry ID, if applicable.
	// +optional
	RekorLogID string `json:"rekorLogID,omitempty"`

	// Status is the runtime verification status of this predicate.
	// +optional
	// +kubebuilder:validation:Enum=Present;Missing;Invalid;Expired;Verified
	Status PredicateStatus `json:"status,omitempty"`

	// Required declares whether this predicate is mandatory for the artifact's kind.
	// +optional
	Required bool `json:"required,omitempty"`
}

// AIPackAttestation holds the attestation bundle for an AIPack artifact.
// Attestation is required for promotion to staging and above (§6.1).
// The operator verifies each predicate via cosign + Rekor at reconciliation time.
type AIPackAttestation struct {
	// Predicates is the list of attestation predicates carried by this artifact.
	// Each entry should resolve to a signed in-toto statement.
	// The RequiredPredicates() function in internal/aipack/predicates.go enumerates
	// the must-carry predicates per kind.
	// +optional
	Predicates []PredicateEntry `json:"predicates,omitempty"`

	// CosignKeyRef is the cosign key reference used to verify signatures.
	// Format: "k8s://<namespace>/<secret>" or "gcr://<path>" or "env://<VAR>"
	// +optional
	CosignKeyRef string `json:"cosignKeyRef,omitempty"`

	// RekorURL is the Rekor transparency log URL.
	// Defaults to https://rekor.sigstore.dev when not specified.
	// +optional
	RekorURL string `json:"rekorURL,omitempty"`

	// Verified declares whether the operator has successfully verified all
	// required predicates. Set by the operator; read-only.
	// +optional
	Verified *bool `json:"verified,omitempty"`

	// VerifiedAt is the RFC 3339 timestamp of the last successful verification.
	// +optional
	VerifiedAt string `json:"verifiedAt,omitempty"`

	// FailedPredicates lists predicate URIs that failed verification.
	// Set by the operator; read-only.
	// +optional
	FailedPredicates []string `json:"failedPredicates,omitempty"`
}
