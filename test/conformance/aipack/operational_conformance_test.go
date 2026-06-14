/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package aipack_conformance — operational conformance vectors (§11-§22).
// V-EXT-NNN  = valid/pass
// I-EXT-NNN  = invalid/fail
package aipack_conformance

import (
	"testing"

	v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
)

const goodHex = "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

// TestLineageConformance covers §11 lineage envelope.
// V-EXT-001: non-nil LineageEnvelope accepted
// I-EXT-001: nil LineageEnvelope returns ErrLineageEnvelopeMissing
func TestLineageConformance(t *testing.T) {
	t.Run("V-EXT-001", func(t *testing.T) {
		env := &v1alpha2.AIPackLineageEnvelope{SourceRef: "registry.example.com/src@" + goodHex}
		err := aipack.ValidateLineageEnvelope(env)
		if err != nil {
			t.Fatalf("[V-EXT-001] unexpected error: %v", err)
		}
	})
	t.Run("I-EXT-001", func(t *testing.T) {
		err := aipack.ValidateLineageEnvelope(nil)
		assertError(t, "I-EXT-001", err, "AIPACK-LIN-001")
	})
}

// TestBlastRadiusConformance covers §12.
// V-EXT-002: blast radius within declared bound accepted
// I-EXT-002: exceeded blast radius returns ErrBlastRadiusExceeded
func TestBlastRadiusConformance(t *testing.T) {
	t.Run("V-EXT-002", func(t *testing.T) {
		err := aipack.ValidateBlastRadius(50, 100)
		if err != nil {
			t.Fatalf("[V-EXT-002] unexpected error: %v", err)
		}
	})
	t.Run("I-EXT-002", func(t *testing.T) {
		err := aipack.ValidateBlastRadius(101, 100)
		assertError(t, "I-EXT-002", err, "AIPACK-BLAST-001")
	})
}

// TestRiskValenceBlockConformance covers §13.4 RED-band block.
// V-EXT-003: ORANGE band does not block
// I-EXT-003: RED band without derogation returns ErrRVRedBandBlocked
func TestRiskValenceBlockConformance(t *testing.T) {
	t.Run("V-EXT-003", func(t *testing.T) {
		err := aipack.CheckRVBandBlock(v1alpha2.RVBandOrange, false)
		if err != nil {
			t.Fatalf("[V-EXT-003] unexpected error: %v", err)
		}
	})
	t.Run("I-EXT-003", func(t *testing.T) {
		err := aipack.CheckRVBandBlock(v1alpha2.RVBandRed, false)
		assertError(t, "I-EXT-003", err, "AIPACK-RV-002")
	})
	t.Run("V-EXT-003b", func(t *testing.T) {
		// RED band with derogation is allowed
		err := aipack.CheckRVBandBlock(v1alpha2.RVBandRed, true)
		if err != nil {
			t.Fatalf("[V-EXT-003b] RED+derogation should be allowed: %v", err)
		}
	})
}

// TestDeprecationConformance covers §16.
// V-EXT-004: active artifact (not deprecated) passes
// I-EXT-004: deprecated artifact without sunset derogation returns ErrDeprecationBlocked
// I-EXT-005: sunset date in past returns ErrSunsetExpired
func TestDeprecationConformance(t *testing.T) {
	t.Run("V-EXT-004", func(t *testing.T) {
		err := aipack.ValidateDeprecationState(nil)
		if err != nil {
			t.Fatalf("[V-EXT-004] unexpected error: %v", err)
		}
	})
	t.Run("I-EXT-004", func(t *testing.T) {
		notice := &v1alpha2.AIPackDeprecationNotice{
			Phase:  v1alpha2.DeprecationPhaseDeprecated,
			Reason: "superseded",
		}
		err := aipack.ValidateDeprecationState(notice)
		assertError(t, "I-EXT-004", err, "AIPACK-DEP-001")
	})
	t.Run("I-EXT-005", func(t *testing.T) {
		notice := &v1alpha2.AIPackDeprecationNotice{
			Phase:      v1alpha2.DeprecationPhaseEndOfLife,
			Reason:     "eol",
			SunsetDate: "2000-01-01", // past date
		}
		err := aipack.ValidateDeprecationState(notice)
		assertError(t, "I-EXT-005", err, "AIPACK-DEP-002")
	})
}

// TestAirGapConformance covers §17.
// V-EXT-005: valid air-gap bundle passes
// I-EXT-006: missing trust root returns ErrAirGapTrustRootMissing
// I-EXT-007: missing TSA returns ErrAirGapTSAMissing
func TestAirGapConformance(t *testing.T) {
	t.Run("V-EXT-005", func(t *testing.T) {
		bundle := &v1alpha2.AIPackAirGapBundle{
			TrustRootRef: "registry.internal/trust-root@" + goodHex,
			TSACertRef:   "registry.internal/tsa@" + goodHex,
		}
		err := aipack.ValidateAirGapBundle(bundle)
		if err != nil {
			t.Fatalf("[V-EXT-005] unexpected error: %v", err)
		}
	})
	t.Run("I-EXT-006", func(t *testing.T) {
		bundle := &v1alpha2.AIPackAirGapBundle{
			TSACertRef: "registry.internal/tsa@" + goodHex,
		}
		err := aipack.ValidateAirGapBundle(bundle)
		assertError(t, "I-EXT-006", err, "AIPACK-AIRGAP-002")
	})
	t.Run("I-EXT-007", func(t *testing.T) {
		bundle := &v1alpha2.AIPackAirGapBundle{
			TrustRootRef: "registry.internal/trust-root@" + goodHex,
		}
		err := aipack.ValidateAirGapBundle(bundle)
		assertError(t, "I-EXT-007", err, "AIPACK-AIRGAP-003")
	})
}

// TestCompositionPatternConformance covers §18.
// V-EXT-006: known pattern name passes
// I-EXT-008: unknown pattern returns ErrManifoldDistanceExceeded
func TestCompositionPatternConformance(t *testing.T) {
	t.Run("V-EXT-006", func(t *testing.T) {
		err := aipack.ValidateCompositionPattern("rag-retriever")
		if err != nil {
			t.Fatalf("[V-EXT-006] unexpected error: %v", err)
		}
	})
	t.Run("I-EXT-008", func(t *testing.T) {
		err := aipack.ValidateCompositionPattern("unknown-pattern-xyz")
		assertError(t, "I-EXT-008", err, "AIPACK-PATTERN-002")
	})
}

// TestPolicyBundleConformance covers §19.
// V-EXT-007: kind allowed by policy passes
// I-EXT-009: kind denied by forbidden list returns ErrProfileFamilyDenied
// I-EXT-010: deny-all sentinel (empty allowedArtifactTypes) returns ErrProfileFamilyDenied
func TestPolicyBundleConformance(t *testing.T) {
	t.Run("V-EXT-007", func(t *testing.T) {
		policy := &v1alpha2.AIPackPolicySpec{
			AllowedArtifactTypes: []v1alpha2.ArtifactKind{v1alpha2.KindBaseModel},
		}
		err := aipack.EvaluatePolicyBundle(policy, v1alpha2.KindBaseModel)
		if err != nil {
			t.Fatalf("[V-EXT-007] unexpected error: %v", err)
		}
	})
	t.Run("I-EXT-009", func(t *testing.T) {
		policy := &v1alpha2.AIPackPolicySpec{
			ForbiddenArtifactTypes: []v1alpha2.ArtifactKind{v1alpha2.KindLoRA},
		}
		err := aipack.EvaluatePolicyBundle(policy, v1alpha2.KindLoRA)
		assertError(t, "I-EXT-009", err, "AIPACK-PROFILE-001")
	})
	t.Run("I-EXT-010", func(t *testing.T) {
		// empty AllowedArtifactTypes = deny-all sentinel (§19.2)
		policy := &v1alpha2.AIPackPolicySpec{
			AllowedArtifactTypes: []v1alpha2.ArtifactKind{}, // deny-all
		}
		err := aipack.EvaluatePolicyBundle(policy, v1alpha2.KindSkill)
		assertError(t, "I-EXT-010", err, "AIPACK-PROFILE-001")
	})
}

// TestQuarantineConformance covers §21.
// V-EXT-008: quiescent trigger (not fired) passes
// I-EXT-011: fired trigger returns ErrQuarantineTriggerFired
func TestQuarantineConformance(t *testing.T) {
	t.Run("V-EXT-008", func(t *testing.T) {
		trigger := &v1alpha2.AIPackQuarantineTrigger{Fired: false}
		err := aipack.ValidateQuarantineTrigger(trigger)
		if err != nil {
			t.Fatalf("[V-EXT-008] unexpected error: %v", err)
		}
	})
	t.Run("I-EXT-011", func(t *testing.T) {
		trigger := &v1alpha2.AIPackQuarantineTrigger{Fired: true, Reason: "blast-radius exceeded"}
		err := aipack.ValidateQuarantineTrigger(trigger)
		assertError(t, "I-EXT-011", err, "AIPACK-TRIGGER-001")
	})
}

// TestVADConformance covers §22.
// V-EXT-009: valid VAD class passes
// I-EXT-012: unknown VAD class returns ErrVADClassUnknown
func TestVADConformance(t *testing.T) {
	t.Run("V-EXT-009", func(t *testing.T) {
		err := aipack.ValidateVADClass("prompt-injection")
		if err != nil {
			t.Fatalf("[V-EXT-009] unexpected error: %v", err)
		}
	})
	t.Run("I-EXT-012", func(t *testing.T) {
		err := aipack.ValidateVADClass("unknown-vad-class")
		assertError(t, "I-EXT-012", err, "AIPACK-VAD-002")
	})
}

// TestOutlierConformance covers §14.
// V-EXT-010: acknowledged outlier signal passes
// V-EXT-011: no outlier signal passes
func TestOutlierConformance(t *testing.T) {
	t.Run("V-EXT-010", func(t *testing.T) {
		signal := &v1alpha2.AIPackOutlierSignal{
			Category:         "statistical-outlier",
			Acknowledged:     true,
			AcknowledgedBy:   "security-reviewer",
		}
		err := aipack.ValidateOutlierSignal(signal)
		if err != nil {
			t.Fatalf("[V-EXT-010] unexpected error: %v", err)
		}
	})
	t.Run("V-EXT-011", func(t *testing.T) {
		err := aipack.ValidateOutlierSignal(nil)
		if err != nil {
			t.Fatalf("[V-EXT-011] nil signal should pass: %v", err)
		}
	})
}
