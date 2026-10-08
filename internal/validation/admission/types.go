/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package admission

import "errors"

// Dispositions represent the definitive action emitted by admission evaluation.
type Disposition string

const (
	DispositionAdmit                Disposition = "ADMIT"
	DispositionAdmitWithObligations Disposition = "ADMIT_WITH_OBLIGATIONS"
	DispositionDeny                 Disposition = "DENY"
	DispositionEscalate             Disposition = "ESCALATE"
	DispositionSafeHold             Disposition = "SAFE_HOLD"
	DispositionQuarantine           Disposition = "QUARANTINE"
)

// StageName defines the name of each admission stage.
type StageName string

const (
	StageData       StageName = "Data"
	StageModel      StageName = "Model"
	StageDeployment StageName = "Deployment"
	StageRequest    StageName = "Request"
)

// VectorState represents non-boolean multi-dimensional evaluation states.
type VectorState string

const (
	StateVerified     VectorState = "VERIFIED"
	StateCompliant    VectorState = "COMPLIANT"
	StateConditional  VectorState = "CONDITIONAL"
	StateUnknown      VectorState = "UNKNOWN"
	StatePending      VectorState = "PENDING"
	StateExhausted    VectorState = "EXHAUSTED"
	StateDegraded     VectorState = "DEGRADED"
	StateIncompatible VectorState = "INCOMPATIBLE"
	StateFlagged      VectorState = "FLAGGED"
	StateViolated     VectorState = "VIOLATED"
	StateQuarantined  VectorState = "QUARANTINED"
)

var (
	ErrInvalidInput        = errors.New("invalid admission input")
	ErrContextCanceled     = errors.New("admission evaluation canceled")
	ErrReceiptGeneration   = errors.New("receipt generation failed")
	ErrReceiptSignature    = errors.New("invalid receipt signature")
	ErrReceiptVerification = errors.New("receipt verification failed")
)

// Obligation defines a governance or operational requirement bound to an admission.
type Obligation struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Parameters  map[string]string `json:"parameters,omitempty"`
}

// ProducerBinding identifies the producer workload authority emitting receipts.
type ProducerBinding struct {
	SPIFFEID  string `json:"spiffeId"`
	KeyDigest string `json:"keyDigest"`
}

// AuditReceipt is a content-free, cryptographically bound evidence receipt.
type AuditReceipt struct {
	ReceiptID     string          `json:"receiptId"`
	Stage         StageName       `json:"stage"`
	Disposition   Disposition     `json:"disposition"`
	SubjectDigest string          `json:"subjectDigest"`
	Producer      ProducerBinding `json:"producer"`
	ProducedAt    string          `json:"producedAt"`
	Digest        string          `json:"digest"`
	Signature     string          `json:"signature,omitempty"`
}

// Decision contains the complete admission result for a single stage.
type Decision struct {
	Stage        StageName         `json:"stage"`
	Disposition  Disposition       `json:"disposition"`
	Reason       string            `json:"reason"`
	VectorStates map[string]string `json:"vectorStates"`
	Obligations  []Obligation      `json:"obligations,omitempty"`
	Receipt      AuditReceipt      `json:"receipt"`
}

// PipelineDecision aggregates the 4-stage admission engine execution result.
type PipelineDecision struct {
	OverallDisposition Disposition    `json:"overallDisposition"`
	StageDecisions     []Decision     `json:"stageDecisions"`
	Obligations        []Obligation   `json:"obligations,omitempty"`
	AuditReceipts      []AuditReceipt `json:"auditReceipts"`
}

// Stage 1 Input: Data admission dimensions.
type DataAdmissionInput struct {
	SubjectID       string
	SubjectDigest   string
	ProvenanceState VectorState
	LicensingState  VectorState
	PrivacyState    VectorState
	IntegrityState  VectorState
	Metadata        map[string]string
}

// Stage 2 Input: Model admission dimensions.
type ModelAdmissionInput struct {
	SubjectID              string
	SubjectDigest          string
	ArtifactIntegrityState VectorState
	SBOMState              VectorState
	SignatureState         VectorState
	AttestationState       VectorState
	LicenseState           VectorState
	Metadata               map[string]string
}

// Stage 3 Input: Deployment admission dimensions.
type DeploymentAdmissionInput struct {
	SubjectID                 string
	SubjectDigest             string
	RuntimeCompatibilityState VectorState
	AcceleratorTopologyState  VectorState
	CapacityState             VectorState
	PolicyState               VectorState
	AirGapProfileState        VectorState
	Metadata                  map[string]string
}

// Stage 4 Input: Request admission dimensions.
type RequestAdmissionInput struct {
	SubjectID            string
	SubjectDigest        string
	IdentityTenantState  VectorState
	CapabilityLeaseState VectorState
	QuotaBudgetState     VectorState
	RiskState            VectorState
	Metadata             map[string]string
}
