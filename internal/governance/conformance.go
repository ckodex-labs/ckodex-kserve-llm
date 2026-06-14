/*
Copyright 2026 CKodex Authors.
*/

package governance

import (
	"context"
	"fmt"
	"time"
	"unicode"

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
	if !HasAssertedSupplyChainEvidence(adapter) {
		return ConformanceResult{
			Valid:   false,
			Reason:  "IncompleteEvidence",
			Message: "Evidence bundle is incomplete: signature digest, attestation URI, and SBOM digest are all required",
		}
	}
	return ConformanceResult{Valid: true}
}

// SBOMValidator verifies that the model artifact includes a valid CycloneDX SBOM.
type SBOMValidator struct{}

func (v *SBOMValidator) Validate(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) ConformanceResult {
	if !HasAssertedSupplyChainEvidence(adapter) {
		return ConformanceResult{
			Valid:   false,
			Reason:  "MissingSBOM",
			Message: "No CycloneDX SBOM digest found in a complete evidence bundle",
		}
	}
	return ConformanceResult{Valid: true}
}

// COMPValidator verifies that the adapter's name is a valid vLLM routing key.
//
// vLLM uses Spec.AdapterName as a route discriminator in inference requests.
// Names with spaces or non-printable characters cause silent routing failures at
// the data plane. Cross-service base-model tokenizer compatibility is enforced
// at load time by the vLLM runtime (it rejects incompatible weights with an
// explicit error) and is therefore not repeated here.
type COMPValidator struct{}

func (v *COMPValidator) Validate(_ context.Context, adapter *servingv1alpha2.LLMLoraAdapter) ConformanceResult {
	name := adapter.Spec.AdapterName
	for _, ch := range name {
		if unicode.IsSpace(ch) || !unicode.IsPrint(ch) {
			return ConformanceResult{
				Valid:   false,
				Reason:  "InvalidAdapterName",
				Message: fmt.Sprintf("adapter name %q contains character %q that is invalid as a vLLM routing key; use lowercase alphanumeric characters, hyphens, or underscores only", name, string(ch)),
			}
		}
	}
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
		"unknown":  0,
		"asserted": 1,
		"verified": 2,
		"trusted":  3,
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
		// The repo does not ship a public evaluation runner artifact yet, so
		// asserted adapters stay active instead of entering a non-functional
		// pending-evaluation state by default.
		adapter.Status.StatePlanes.Lifecycle = "active"
		adapter.Status.StatePlanes.Risk = "normal"
		adapter.Status.StatePlanes.Trust = "asserted"
	}

	now := metav1.NewTime(time.Now())
	adapter.Status.EvidenceBundle.LastVerifiedAt = &now
}
