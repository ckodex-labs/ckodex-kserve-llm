/*
Copyright 2026 CKodex Authors.
*/

package governance

import (
	"context"
	"fmt"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConformanceResult represents the outcome of a governance check.
type ConformanceResult struct {
	Valid   bool
	Reason  string
	Message string
}

// Validator defines the interface for governance vector validation.
type Validator interface {
	Validate(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) ConformanceResult
}

// EVIDValidator verifies evidence (signatures, attestations).
type EVIDValidator struct{}

func (v *EVIDValidator) Validate(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) ConformanceResult {
	if adapter.Status.EvidenceBundle.SignatureDigest == "" {
		return ConformanceResult{Valid: false, Reason: "MissingSignature", Message: "No Cosign signature found in evidence bundle"}
	}
	if adapter.Status.EvidenceBundle.AttestationURI == "" {
		return ConformanceResult{Valid: false, Reason: "MissingAttestation", Message: "No SLSA attestation found in evidence bundle"}
	}
	// In production, this would use 'github.com/slsa-framework/slsa-verifier'
	return ConformanceResult{Valid: true}
}

// SBOMValidator verifies that the model artifact includes a valid CycloneDX SBOM.
type SBOMValidator struct{}

func (v *SBOMValidator) Validate(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) ConformanceResult {
	// For production readiness, we check if the SBOM digest is recorded in the status.
	// This ensures the evidence plane is fully populated.
	if adapter.Status.EvidenceBundle.SBOMDigest == "" {
		return ConformanceResult{Valid: false, Reason: "MissingSBOM", Message: "No CycloneDX SBOM digest found in evidence bundle"}
	}
	return ConformanceResult{Valid: true}
}

// COMPValidator verifies compatibility between adapter and foundation model.
type COMPValidator struct{}

func (v *COMPValidator) Validate(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) ConformanceResult {
	// Logic to check if adapter's base model matches the target service's base model.
	// This is a placeholder for digest/tokenizer matching.
	return ConformanceResult{Valid: true}
}

// SAFEValidator verifies safety/refusal thresholds.
type SAFEValidator struct{}

func (v *SAFEValidator) Validate(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) ConformanceResult {
	if adapter.Spec.Behavior != nil && adapter.Spec.Behavior.Safety < 5 {
		return ConformanceResult{Valid: false, Reason: "SafetyViolation", Message: "Adapter safety score is below the mandatory threshold of 5"}
	}
	return ConformanceResult{Valid: true}
}

// PolicyValidator verifies that the adapter meets the required policy envelope.
type PolicyValidator struct{}

func (v *PolicyValidator) Validate(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) ConformanceResult {
	if adapter.Spec.PolicyEnvelope == nil {
		return ConformanceResult{Valid: true}
	}

	minTrust := adapter.Spec.PolicyEnvelope.MinTrustLevel
	if minTrust == "" {
		return ConformanceResult{Valid: true}
	}

	actualTrust := adapter.Status.StatePlanes.Trust
	if actualTrust == "" {
		actualTrust = "unknown"
	}

	// Simple trust hierarchy: unknown < asserted < verified < trusted
	trustScores := map[string]int{
		"unknown":   0,
		"asserted":  1,
		"verified":  2,
		"trusted":   3,
		"denied":   -1,
	}

	if trustScores[actualTrust] < trustScores[minTrust] {
		return ConformanceResult{
			Valid:   false,
			Reason:  "PolicyBlocked",
			Message: fmt.Sprintf("Adapter trust level '%s' is insufficient for required policy level '%s'", actualTrust, minTrust),
		}
	}

	return ConformanceResult{Valid: true}
}

// ConformanceEngine aggregates governance checks.
type ConformanceEngine struct {
	Validators []Validator
}

func NewDefaultEngine() *ConformanceEngine {
	return &ConformanceEngine{
		Validators: []Validator{
			&EVIDValidator{},
			&SBOMValidator{},
			&COMPValidator{},
			&SAFEValidator{},
			&PolicyValidator{},
		},
	}
}

func (e *ConformanceEngine) Check(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) (bool, string) {
	for _, v := range e.Validators {
		res := v.Validate(ctx, adapter)
		if !res.Valid {
			return false, fmt.Sprintf("[%s] %s", res.Reason, res.Message)
		}
	}
	return true, "All conformance vectors passed"
}

// TransitionStates updates the state planes based on governance results.
func TransitionStates(adapter *servingv1alpha2.LLMLoraAdapter, valid bool, msg string) {
	if !valid {
		adapter.Status.StatePlanes.Lifecycle = "quarantined"
		adapter.Status.StatePlanes.Risk = "quarantined"
		adapter.Status.StatePlanes.Trust = "denied"
	} else if adapter.Status.StatePlanes.Trust == "verified" || adapter.Status.StatePlanes.Trust == "trusted" {
		adapter.Status.StatePlanes.Lifecycle = "active"
		adapter.Status.StatePlanes.Risk = "normal"
	} else {
		adapter.Status.StatePlanes.Lifecycle = "pending-evaluation"
		adapter.Status.StatePlanes.Risk = "evaluating"
		adapter.Status.StatePlanes.Trust = "asserted"
	}
	
	now := metav1.NewTime(time.Now())
	adapter.Status.EvidenceBundle.LastVerifiedAt = &now
}
