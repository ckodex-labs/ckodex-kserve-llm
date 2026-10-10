/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package composition

import "errors"

// Standard domain errors.
var (
	ErrNilComponent             = errors.New("component cannot be nil")
	ErrInvalidKind              = errors.New("invalid component kind for slot")
	ErrMissingIdentity          = errors.New("component identity missing or incomplete")
	ErrMissingDigest            = errors.New("component digest missing or invalid")
	ErrMissingRequiredComponent = errors.New("required pipeline component missing")
	ErrCompatibilityViolation   = errors.New("inter-component compatibility violation")
	ErrForbiddenVectorTuple     = errors.New("forbidden state vector tuple detected")
	ErrAdmissionDenied          = errors.New("component admission denied")
	ErrExecutionHalted          = errors.New("pipeline execution halted by policy")
)

// ComponentKind identifies the semantic kind of a component per Requirements §7.
type ComponentKind string

const (
	KindModel        ComponentKind = "Model"
	KindTokenizer    ComponentKind = "Tokenizer"
	KindQuantization ComponentKind = "Quantization"
	KindAdapter      ComponentKind = "Adapter"
	KindProjection   ComponentKind = "Projection"
	KindFilter       ComponentKind = "Filter"
	KindGuard        ComponentKind = "Guard"
	KindPlugin       ComponentKind = "Plugin"
	KindRuntime      ComponentKind = "Runtime"
)

// ExecutionStage identifies the pipeline execution phase.
type ExecutionStage string

const (
	StageUnspecified   ExecutionStage = "Unspecified"
	StagePreExecution  ExecutionStage = "PreExecution"
	StageTokenize      ExecutionStage = "Tokenize"
	StageInference     ExecutionStage = "Inference"
	StagePostExecution ExecutionStage = "PostExecution"
)

// LifecycleState defines the operational lifecycle dimension.
type LifecycleState string

const (
	LifecycleUnspecified LifecycleState = "Unspecified"
	LifecycleProposed    LifecycleState = "Proposed"
	LifecycleActive      LifecycleState = "Active"
	LifecycleQuarantined LifecycleState = "Quarantined"
	LifecycleDeprecated  LifecycleState = "Deprecated"
	LifecycleRetired     LifecycleState = "Retired"
)

// TrustState defines the cryptographic trust dimension.
type TrustState string

const (
	TrustUnspecified TrustState = "Unspecified"
	TrustDenied      TrustState = "Denied"
	TrustUnknown     TrustState = "Unknown"
	TrustAsserted    TrustState = "Asserted"
	TrustVerified    TrustState = "Verified"
	TrustTrusted     TrustState = "Trusted"
)

// RiskBand defines the risk valuation dimension.
type RiskBand string

const (
	RiskUnspecified RiskBand = "Unspecified"
	RiskLow         RiskBand = "Low"
	RiskNormal      RiskBand = "Normal"
	RiskHigh        RiskBand = "High"
	RiskCritical    RiskBand = "Critical"
)

// AdmissionDecision defines the admission plane governance verdict.
type AdmissionDecision string

const (
	AdmissionUnspecified AdmissionDecision = "Unspecified"
	AdmissionAdmitted    AdmissionDecision = "Admitted"
	AdmissionAudit       AdmissionDecision = "Audit"
	AdmissionPending     AdmissionDecision = "Pending"
	AdmissionDenied      AdmissionDecision = "Denied"
)

// CompatibilityState defines the end-to-end compatibility state vector dimension.
type CompatibilityState string

const (
	CompatibilityUnspecified  CompatibilityState = "Unspecified"
	CompatibilityCompatible   CompatibilityState = "Compatible"
	CompatibilityDegraded     CompatibilityState = "Degraded"
	CompatibilityIncompatible CompatibilityState = "Incompatible"
)

// IntegrityState defines the artifact and supply-chain integrity dimension.
type IntegrityState string

const (
	IntegrityUnspecified IntegrityState = "Unspecified"
	IntegrityVerified    IntegrityState = "Verified"
	IntegrityUnsigned    IntegrityState = "Unsigned"
	IntegrityTampered    IntegrityState = "Tampered"
	IntegrityMissing     IntegrityState = "Missing"
)

// ExecutionStatus defines the termination status of a pipeline execution.
type ExecutionStatus string

const (
	StatusUnspecified ExecutionStatus = "Unspecified"
	StatusSuccess     ExecutionStatus = "Success"
	StatusHalted      ExecutionStatus = "Halted"
	StatusFailed      ExecutionStatus = "Failed"
)

// StateVector encapsulates the multidimensional state planes of a component or composite.
// In accordance with CKODEX governance, state is represented as vectors, not booleans.
type StateVector struct {
	Lifecycle     LifecycleState     `json:"lifecycle"`
	Trust         TrustState         `json:"trust"`
	Risk          RiskBand           `json:"risk"`
	Admission     AdmissionDecision  `json:"admission"`
	Compatibility CompatibilityState `json:"compatibility"`
	Integrity     IntegrityState     `json:"integrity"`
}

// Identity uniquely addresses a component.
type Identity struct {
	ID   string        `json:"id"`
	Name string        `json:"name"`
	Kind ComponentKind `json:"kind"`
}

// CompatibilitySpec defines compatibility boundaries and declared constraints.
type CompatibilitySpec struct {
	Architecture      string         `json:"architecture,omitempty"`
	BaseModelDigest   string         `json:"baseModelDigest,omitempty"`
	SupportedRuntimes []string       `json:"supportedRuntimes,omitempty"`
	SupportedFormats  []string       `json:"supportedFormats,omitempty"`
	InputDimension    int            `json:"inputDimension,omitempty"`
	OutputDimension   int            `json:"outputDimension,omitempty"`
	TargetModules     []string       `json:"targetModules,omitempty"`
	Stage             ExecutionStage `json:"stage,omitempty"`
}

// Provenance tracks origin and attestation lineage.
type Provenance struct {
	SourceURI        string `json:"sourceUri"`
	Publisher        string `json:"publisher"`
	SignerIdentity   string `json:"signerIdentity,omitempty"`
	SupplyChainLevel string `json:"supplyChainLevel,omitempty"`
}

// AdmissionRecord records the admission evaluation.
type AdmissionRecord struct {
	Decision        AdmissionDecision `json:"decision"`
	PolicyRef       string            `json:"policyRef,omitempty"`
	ReceiptDigest   string            `json:"receiptDigest,omitempty"`
	EvaluatedAtUnix int64             `json:"evaluatedAtUnix,omitempty"`
}

// LifecycleRecord tracks operational phase transitions.
type LifecycleRecord struct {
	State         LifecycleState `json:"state"`
	Phase         string         `json:"phase,omitempty"`
	UpdatedAtUnix int64          `json:"updatedAtUnix,omitempty"`
}

// EvidenceRecord tracks cryptographic verification and SBOMs.
type EvidenceRecord struct {
	AttestationURI  string         `json:"attestationUri,omitempty"`
	SignatureDigest string         `json:"signatureDigest,omitempty"`
	SBOMDigest      string         `json:"sbomDigest,omitempty"`
	Integrity       IntegrityState `json:"integrity"`
}

// Component represents a governed pipeline component with full identity,
// version, digest, compatibility, provenance, admission, lifecycle, and evidence.
type Component struct {
	Identity      Identity          `json:"identity"`
	Version       string            `json:"version"`
	Digest        string            `json:"digest"`
	Compatibility CompatibilitySpec `json:"compatibility"`
	Provenance    Provenance        `json:"provenance"`
	Admission     AdmissionRecord   `json:"admission"`
	Lifecycle     LifecycleRecord   `json:"lifecycle"`
	Evidence      EvidenceRecord    `json:"evidence"`
	StateVector   StateVector       `json:"stateVector"`
}

// ValidationReport details end-to-end compatibility and state vector analysis.
type ValidationReport struct {
	CompatibilityStatus CompatibilityState `json:"compatibilityStatus"`
	StateVector         StateVector        `json:"stateVector"`
	Violations          []string           `json:"violations,omitempty"`
	CompositeDigest     string             `json:"compositeDigest,omitempty"`
}

// Payload conveys data and metadata through the pipeline execution stages.
type Payload struct {
	Data       string            `json:"data"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Tokens     []int             `json:"tokens,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// CompositionReceipt is the immutable record of an execution run.
type CompositionReceipt struct {
	PipelineID      string          `json:"pipelineId"`
	CompositeDigest string          `json:"compositeDigest"`
	Status          ExecutionStatus `json:"status"`
	StateVector     StateVector     `json:"stateVector"`
	StageTraces     []string        `json:"stageTraces"`
	ExecutedAtUnix  int64           `json:"executedAtUnix"`
}
