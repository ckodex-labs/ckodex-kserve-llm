/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package admission implements a 4-stage admission engine for AI workloads,
// evaluating Data, Model, Deployment, and Request stages using multi-dimensional
// vector states, attaching mandatory obligations and tamper-evident audit receipts.
package admission

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Engine executes 4-stage admission evaluation.
type Engine struct {
	producer   ProducerBinding
	privateKey ed25519.PrivateKey
}

// EngineOption configures the admission engine.
type EngineOption func(*Engine)

// WithPrivateKey configures an Ed25519 signing key for audit receipts.
func WithPrivateKey(key ed25519.PrivateKey) EngineOption {
	return func(e *Engine) {
		e.privateKey = key
	}
}

// NewEngine initializes a new admission engine instance.
func NewEngine(producer ProducerBinding, opts ...EngineOption) *Engine {
	e := &Engine{producer: producer}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// AdmitData evaluates Stage 1: Data admission.
func (e *Engine) AdmitData(ctx context.Context, in DataAdmissionInput) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, fmt.Errorf("%w: %w", ErrContextCanceled, err)
	}
	if err := validateDataInput(in); err != nil {
		return Decision{}, err
	}
	orderedKeys := []string{"integrity", "provenance", "licensing", "privacy"}
	states := map[string]VectorState{
		"integrity":  in.IntegrityState,
		"provenance": in.ProvenanceState,
		"licensing":  in.LicensingState,
		"privacy":    in.PrivacyState,
	}
	disp, reason := resolveVectorStates(orderedKeys, states)
	var obls []Obligation
	if disp == DispositionAdmitWithObligations {
		obls = dataObligations(in)
	}
	return e.buildDecision(StageData, disp, reason, in.SubjectDigest, states, obls)
}

// AdmitModel evaluates Stage 2: Model admission.
func (e *Engine) AdmitModel(ctx context.Context, in ModelAdmissionInput) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, fmt.Errorf("%w: %w", ErrContextCanceled, err)
	}
	if err := validateModelInput(in); err != nil {
		return Decision{}, err
	}
	orderedKeys := []string{"artifactIntegrity", "signature", "attestation", "sbom", "license"}
	states := map[string]VectorState{
		"artifactIntegrity": in.ArtifactIntegrityState,
		"signature":         in.SignatureState,
		"attestation":       in.AttestationState,
		"sbom":              in.SBOMState,
		"license":           in.LicenseState,
	}
	disp, reason := resolveVectorStates(orderedKeys, states)
	var obls []Obligation
	if disp == DispositionAdmitWithObligations {
		obls = modelObligations(in)
	}
	return e.buildDecision(StageModel, disp, reason, in.SubjectDigest, states, obls)
}

// AdmitDeployment evaluates Stage 3: Deployment admission.
func (e *Engine) AdmitDeployment(ctx context.Context, in DeploymentAdmissionInput) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, fmt.Errorf("%w: %w", ErrContextCanceled, err)
	}
	if err := validateDeploymentInput(in); err != nil {
		return Decision{}, err
	}
	orderedKeys := []string{"policy", "runtimeCompatibility", "acceleratorTopology", "airGapProfile", "capacity"}
	states := map[string]VectorState{
		"policy":               in.PolicyState,
		"runtimeCompatibility": in.RuntimeCompatibilityState,
		"acceleratorTopology":  in.AcceleratorTopologyState,
		"airGapProfile":        in.AirGapProfileState,
		"capacity":             in.CapacityState,
	}
	disp, reason := resolveVectorStates(orderedKeys, states)
	var obls []Obligation
	if disp == DispositionAdmitWithObligations {
		obls = deploymentObligations(in)
	}
	return e.buildDecision(StageDeployment, disp, reason, in.SubjectDigest, states, obls)
}

// AdmitRequest evaluates Stage 4: Request admission.
func (e *Engine) AdmitRequest(ctx context.Context, in RequestAdmissionInput) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, fmt.Errorf("%w: %w", ErrContextCanceled, err)
	}
	if err := validateRequestInput(in); err != nil {
		return Decision{}, err
	}
	orderedKeys := []string{"risk", "identityTenant", "capabilityLease", "quotaBudget"}
	states := map[string]VectorState{
		"risk":            in.RiskState,
		"identityTenant":  in.IdentityTenantState,
		"capabilityLease": in.CapabilityLeaseState,
		"quotaBudget":     in.QuotaBudgetState,
	}
	disp, reason := resolveVectorStates(orderedKeys, states)
	var obls []Obligation
	if disp == DispositionAdmitWithObligations {
		obls = requestObligations(in)
	}
	return e.buildDecision(StageRequest, disp, reason, in.SubjectDigest, states, obls)
}

// EvaluatePipeline runs the 4-stage admission engine in sequence with short-circuiting.
func (e *Engine) EvaluatePipeline(
	ctx context.Context,
	data DataAdmissionInput,
	model ModelAdmissionInput,
	deploy DeploymentAdmissionInput,
	req RequestAdmissionInput,
) (PipelineDecision, error) {
	if err := ctx.Err(); err != nil {
		return PipelineDecision{}, fmt.Errorf("%w: %w", ErrContextCanceled, err)
	}
	stageFuncs := []func() (Decision, error){
		func() (Decision, error) { return e.AdmitData(ctx, data) },
		func() (Decision, error) { return e.AdmitModel(ctx, model) },
		func() (Decision, error) { return e.AdmitDeployment(ctx, deploy) },
		func() (Decision, error) { return e.AdmitRequest(ctx, req) },
	}
	var decisions []Decision
	var allObligations []Obligation
	var allReceipts []AuditReceipt
	for _, sf := range stageFuncs {
		dec, err := sf()
		if err != nil {
			return PipelineDecision{}, err
		}
		decisions = append(decisions, dec)
		allObligations = append(allObligations, dec.Obligations...)
		allReceipts = append(allReceipts, dec.Receipt)
		if dec.Disposition == DispositionQuarantine || dec.Disposition == DispositionDeny {
			break
		}
	}
	return PipelineDecision{
		OverallDisposition: calculateOverallDisposition(decisions),
		StageDecisions:     decisions,
		Obligations:        allObligations,
		AuditReceipts:      allReceipts,
	}, nil
}

func (e *Engine) buildDecision(
	stage StageName,
	disp Disposition,
	reason, subjectDigest string,
	states map[string]VectorState,
	obls []Obligation,
) (Decision, error) {
	strStates := make(map[string]string, len(states))
	for k, v := range states {
		strStates[k] = string(v)
	}
	receipt, err := e.createReceipt(stage, disp, subjectDigest, strStates)
	if err != nil {
		return Decision{}, err
	}
	return Decision{
		Stage:        stage,
		Disposition:  disp,
		Reason:       reason,
		VectorStates: strStates,
		Obligations:  obls,
		Receipt:      receipt,
	}, nil
}

func resolveVectorStates(orderedKeys []string, states map[string]VectorState) (Disposition, string) {
	for _, k := range orderedKeys {
		if states[k] == StateQuarantined {
			return DispositionQuarantine, fmt.Sprintf("%s dimension is quarantined", k)
		}
	}
	for _, k := range orderedKeys {
		if states[k] == StateViolated || states[k] == StateIncompatible {
			return DispositionDeny, fmt.Sprintf("%s dimension violated admission policy", k)
		}
	}
	for _, k := range orderedKeys {
		if states[k] == StateUnknown || states[k] == StateFlagged {
			return DispositionEscalate, fmt.Sprintf("%s dimension requires escalation", k)
		}
	}
	for _, k := range orderedKeys {
		if states[k] == StatePending || states[k] == StateExhausted {
			return DispositionSafeHold, fmt.Sprintf("%s dimension placed on safe-hold", k)
		}
	}
	for _, k := range orderedKeys {
		if states[k] == StateConditional || states[k] == StateDegraded {
			return DispositionAdmitWithObligations, fmt.Sprintf("%s dimension admitted with obligations", k)
		}
	}
	return DispositionAdmit, "all stage dimensions verified and compliant"
}

func (e *Engine) createReceipt(stage StageName, disp Disposition, subjectDigest string, states map[string]string) (AuditReceipt, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if subjectDigest == "" {
		subjectDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	}
	receiptID := fmt.Sprintf("rcpt-%s-%d", strings.ToLower(string(stage)), time.Now().UnixNano())
	claims := struct {
		ReceiptID     string            `json:"receiptId"`
		Stage         StageName         `json:"stage"`
		Disposition   Disposition       `json:"disposition"`
		SubjectDigest string            `json:"subjectDigest"`
		Producer      ProducerBinding   `json:"producer"`
		ProducedAt    string            `json:"producedAt"`
		VectorStates  map[string]string `json:"vectorStates"`
	}{
		ReceiptID: receiptID, Stage: stage, Disposition: disp,
		SubjectDigest: subjectDigest, Producer: e.producer,
		ProducedAt: now, VectorStates: states,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return AuditReceipt{}, fmt.Errorf("%w: marshal receipt claims: %w", ErrReceiptGeneration, err)
	}
	hash := sha256.Sum256(payload)
	digest := "sha256:" + hex.EncodeToString(hash[:])
	receipt := AuditReceipt{
		ReceiptID: receiptID, Stage: stage, Disposition: disp,
		SubjectDigest: subjectDigest, Producer: e.producer,
		ProducedAt: now, Digest: digest,
	}
	if len(e.privateKey) == ed25519.PrivateKeySize {
		sig := ed25519.Sign(e.privateKey, []byte(digest))
		receipt.Signature = base64.StdEncoding.EncodeToString(sig)
	}
	return receipt, nil
}

// VerifyReceipt verifies the Ed25519 cryptographic signature of an AuditReceipt.
func VerifyReceipt(receipt AuditReceipt, publicKey ed25519.PublicKey) error {
	if receipt.ReceiptID == "" || receipt.Stage == "" || receipt.Digest == "" {
		return fmt.Errorf("%w: missing required receipt fields", ErrReceiptVerification)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid public key length", ErrReceiptVerification)
	}
	if receipt.Signature == "" {
		return fmt.Errorf("%w: receipt has no signature", ErrReceiptVerification)
	}
	sig, err := base64.StdEncoding.DecodeString(receipt.Signature)
	if err != nil {
		return fmt.Errorf("%w: decode signature: %w", ErrReceiptSignature, err)
	}
	if !ed25519.Verify(publicKey, []byte(receipt.Digest), sig) {
		return fmt.Errorf("%w: signature verification mismatch", ErrReceiptSignature)
	}
	return nil
}

func calculateOverallDisposition(decisions []Decision) Disposition {
	if len(decisions) == 0 {
		return DispositionDeny
	}
	precedence := map[Disposition]int{
		DispositionQuarantine:           6,
		DispositionDeny:                 5,
		DispositionEscalate:             4,
		DispositionSafeHold:             3,
		DispositionAdmitWithObligations: 2,
		DispositionAdmit:                1,
	}
	highest := DispositionAdmit
	for _, d := range decisions {
		if precedence[d.Disposition] > precedence[highest] {
			highest = d.Disposition
		}
	}
	return highest
}

func dataObligations(in DataAdmissionInput) []Obligation {
	var obls []Obligation
	if in.LicensingState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-data-attribution",
			Type:        "data.licensing.attribution",
			Description: "Dataset attribution notice must be preserved and surfaced",
		})
	}
	if in.PrivacyState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-data-privacy-masking",
			Type:        "data.privacy.masking",
			Description: "Differential privacy and runtime redaction filters must remain active",
		})
	}
	return obls
}

func modelObligations(in ModelAdmissionInput) []Obligation {
	var obls []Obligation
	if in.LicenseState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-model-openrail-restrictions",
			Type:        "model.license.openrail",
			Description: "Downstream use restrictions and risk disclosures must be distributed with model artifacts",
		})
	}
	if in.SBOMState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-model-sbom-remediation",
			Type:        "model.sbom.remediation",
			Description: "Non-critical SBOM component advisories must be remediated on schedule",
		})
	}
	return obls
}

func deploymentObligations(in DeploymentAdmissionInput) []Obligation {
	var obls []Obligation
	if in.AirGapProfileState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-deploy-airgap-monitoring",
			Type:        "deployment.airgap.egress_audit",
			Description: "Log and audit all gateway-mediated egress through air-gap boundary proxy",
		})
	}
	if in.CapacityState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-deploy-capacity-lease",
			Type:        "deployment.capacity.off_peak",
			Description: "Workload admitted under off-peak preemption lease agreement",
		})
	}
	if in.PolicyState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-deploy-security-telemetry",
			Type:        "deployment.policy.telemetry",
			Description: "Security telemetry export must be enabled and bound to tenant stream",
		})
	}
	return obls
}

func requestObligations(in RequestAdmissionInput) []Obligation {
	var obls []Obligation
	if in.RiskState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-req-watermarking",
			Type:        "request.governance.watermarking",
			Description: "Apply cryptographic watermarking and prompt logging to inference stream",
		})
	}
	if in.CapabilityLeaseState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-req-lease-limit",
			Type:        "request.lease.concurrency_bound",
			Description: "Bound request concurrency to conditional capability lease terms",
		})
	}
	if in.QuotaBudgetState == StateConditional {
		obls = append(obls, Obligation{
			ID:          "obl-req-budget-audit",
			Type:        "request.quota.burst_audit",
			Description: "Audit and reconcile token burst against monthly allocation",
		})
	}
	return obls
}

func validateDataInput(in DataAdmissionInput) error {
	if in.SubjectID == "" {
		return fmt.Errorf("%w: subject ID required", ErrInvalidInput)
	}
	if in.IntegrityState == "" || in.ProvenanceState == "" || in.LicensingState == "" || in.PrivacyState == "" {
		return fmt.Errorf("%w: all vector states must be populated", ErrInvalidInput)
	}
	return nil
}

func validateModelInput(in ModelAdmissionInput) error {
	if in.SubjectID == "" {
		return fmt.Errorf("%w: subject ID required", ErrInvalidInput)
	}
	if in.ArtifactIntegrityState == "" || in.SBOMState == "" || in.SignatureState == "" || in.AttestationState == "" || in.LicenseState == "" {
		return fmt.Errorf("%w: all vector states must be populated", ErrInvalidInput)
	}
	return nil
}

func validateDeploymentInput(in DeploymentAdmissionInput) error {
	if in.SubjectID == "" {
		return fmt.Errorf("%w: subject ID required", ErrInvalidInput)
	}
	if in.RuntimeCompatibilityState == "" || in.AcceleratorTopologyState == "" || in.CapacityState == "" || in.PolicyState == "" || in.AirGapProfileState == "" {
		return fmt.Errorf("%w: all vector states must be populated", ErrInvalidInput)
	}
	return nil
}

func validateRequestInput(in RequestAdmissionInput) error {
	if in.SubjectID == "" {
		return fmt.Errorf("%w: subject ID required", ErrInvalidInput)
	}
	if in.IdentityTenantState == "" || in.CapabilityLeaseState == "" || in.QuotaBudgetState == "" || in.RiskState == "" {
		return fmt.Errorf("%w: all vector states must be populated", ErrInvalidInput)
	}
	return nil
}
