/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package aipack_conformance validates AIPACK-SPEC v0.1.1 conformance vectors.
// Naming convention:
//
//	V-NNN — valid/pass vectors (must not return an error)
//	I-NNN — invalid/fail vectors (must return an error matching wantCode)
package aipack_conformance

import (
	"errors"
	"testing"

	v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
)

// kindVector is a test vector for kind + family validation.
type kindVector struct {
	id       string
	kind     v1alpha2.ArtifactKind
	family   *v1alpha2.ArtifactFamily
	wantCode string // empty = expect no error
}

func familyPtr(f v1alpha2.ArtifactFamily) *v1alpha2.ArtifactFamily { return &f }

// TestKindValidation covers V-001..V-015 (all 15 valid kinds) and
// I-001..I-004 (unknown kind, family mismatch, empty kind, numeric kind).
func TestKindValidation(t *testing.T) {
	vectors := []kindVector{
		// Valid kinds — V-001..V-015
		{id: "V-001", kind: v1alpha2.KindBaseModel},
		{id: "V-002", kind: v1alpha2.KindLoRA},
		{id: "V-003", kind: v1alpha2.KindFineTune},
		{id: "V-004", kind: v1alpha2.KindSkill},
		{id: "V-005", kind: v1alpha2.KindTool},
		{id: "V-006", kind: v1alpha2.KindMCPServer},
		{id: "V-007", kind: v1alpha2.KindPromptTemplate},
		{id: "V-008", kind: v1alpha2.KindGuardrail},
		{id: "V-009", kind: v1alpha2.KindRetrievalIndex},
		{id: "V-010", kind: v1alpha2.KindDataset},
		{id: "V-011", kind: v1alpha2.KindHarness},
		{id: "V-012", kind: v1alpha2.KindEval},
		{id: "V-013", kind: v1alpha2.KindWorkflow},
		{id: "V-014", kind: v1alpha2.KindPolicyBundle},
		{id: "V-015", kind: v1alpha2.KindAgent},
		// Valid family + kind pairs — V-016..V-021
		{id: "V-016", kind: v1alpha2.KindBaseModel, family: familyPtr(v1alpha2.FamilyModel)},
		{id: "V-017", kind: v1alpha2.KindSkill, family: familyPtr(v1alpha2.FamilyCapability)},
		{id: "V-018", kind: v1alpha2.KindGuardrail, family: familyPtr(v1alpha2.FamilyControl)},
		{id: "V-019", kind: v1alpha2.KindDataset, family: familyPtr(v1alpha2.FamilyKnowledge)},
		{id: "V-020", kind: v1alpha2.KindHarness, family: familyPtr(v1alpha2.FamilyAssurance)},
		{id: "V-021", kind: v1alpha2.KindAgent, family: familyPtr(v1alpha2.FamilyComposite)},
		// Invalid — I-001..I-004
		{id: "I-001", kind: "UnknownKind", wantCode: "AIPACK-KIND-000"},
		{id: "I-002", kind: "", wantCode: "AIPACK-KIND-000"},
		{id: "I-003", kind: "42", wantCode: "AIPACK-KIND-000"},
		{id: "I-004", kind: v1alpha2.KindBaseModel, family: familyPtr(v1alpha2.FamilyCapability), wantCode: "AIPACK-KIND-001"},
	}

	for _, v := range vectors {
		v := v
		t.Run(v.id, func(t *testing.T) {
			err := aipack.ValidateKind(v.kind)

			if v.family != nil && err == nil {
				expected, ok := aipack.FamilyForKind(v.kind)
				if ok && *v.family != expected {
					err = &aipack.AIPackError{Code: aipack.ErrFamilyMismatch, Message: "family mismatch"}
				}
			}

			assertError(t, v.id, err, v.wantCode)
		})
	}
}

// TestKindFamilyMap validates the normative §3.5 kind→family mapping for all 15 kinds.
func TestKindFamilyMap(t *testing.T) {
	pairs := []struct {
		kind   v1alpha2.ArtifactKind
		family v1alpha2.ArtifactFamily
	}{
		{v1alpha2.KindBaseModel, v1alpha2.FamilyModel},
		{v1alpha2.KindLoRA, v1alpha2.FamilyModel},
		{v1alpha2.KindFineTune, v1alpha2.FamilyModel},
		{v1alpha2.KindSkill, v1alpha2.FamilyCapability},
		{v1alpha2.KindTool, v1alpha2.FamilyCapability},
		{v1alpha2.KindMCPServer, v1alpha2.FamilyCapability},
		{v1alpha2.KindWorkflow, v1alpha2.FamilyCapability},
		{v1alpha2.KindPromptTemplate, v1alpha2.FamilyControl},
		{v1alpha2.KindGuardrail, v1alpha2.FamilyControl},
		{v1alpha2.KindPolicyBundle, v1alpha2.FamilyControl},
		{v1alpha2.KindRetrievalIndex, v1alpha2.FamilyKnowledge},
		{v1alpha2.KindDataset, v1alpha2.FamilyKnowledge},
		{v1alpha2.KindHarness, v1alpha2.FamilyAssurance},
		{v1alpha2.KindEval, v1alpha2.FamilyAssurance},
		{v1alpha2.KindAgent, v1alpha2.FamilyComposite},
	}

	for _, p := range pairs {
		p := p
		t.Run(string(p.kind), func(t *testing.T) {
			got, ok := aipack.FamilyForKind(p.kind)
			if !ok {
				t.Fatalf("FamilyForKind(%s) returned false", p.kind)
			}
			if got != p.family {
				t.Fatalf("FamilyForKind(%s) = %s, want %s", p.kind, got, p.family)
			}
		})
	}
}

// refVector is a test vector for OCI digest ref validation.
type refVector struct {
	id       string
	ref      string
	wantCode string
}

// TestRefValidation covers valid refs and I-005..I-012 (invalid refs).
func TestRefValidation(t *testing.T) {
	goodDigest := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	vectors := []refVector{
		// Valid refs
		{id: "V-REF-001", ref: "registry.example.com/repo/image@" + goodDigest},
		{id: "V-REF-002", ref: "ghcr.io/org/image@" + goodDigest},
		// Invalid refs — I-005..I-012
		{id: "I-005", ref: "image:latest", wantCode: "AIPACK-COMP-001"},
		{id: "I-006", ref: "image:v1.0.0", wantCode: "AIPACK-COMP-001"},
		{id: "I-007", ref: "", wantCode: "AIPACK-COMP-001"},
		{id: "I-008", ref: "registry.example.com/image@sha256:short", wantCode: "AIPACK-COMP-001"},
		{id: "I-009", ref: "registry.example.com/image@md5:abcdefabcdef", wantCode: "AIPACK-COMP-001"},
		{id: "I-010", ref: "@" + goodDigest, wantCode: "AIPACK-COMP-001"},
		{id: "I-011", ref: "image", wantCode: "AIPACK-COMP-001"},
		{id: "I-012", ref: goodDigest, wantCode: "AIPACK-COMP-001"},
	}

	for _, v := range vectors {
		v := v
		t.Run(v.id, func(t *testing.T) {
			err := aipack.ValidateRef(v.ref)
			assertError(t, v.id, err, v.wantCode)
		})
	}
}

// TestMediaTypes validates that all 15 kinds map to a non-empty media type.
func TestMediaTypes(t *testing.T) {
	kinds := []v1alpha2.ArtifactKind{
		v1alpha2.KindBaseModel, v1alpha2.KindLoRA, v1alpha2.KindFineTune,
		v1alpha2.KindSkill, v1alpha2.KindTool, v1alpha2.KindMCPServer,
		v1alpha2.KindPromptTemplate, v1alpha2.KindGuardrail, v1alpha2.KindRetrievalIndex,
		v1alpha2.KindDataset, v1alpha2.KindHarness, v1alpha2.KindEval,
		v1alpha2.KindWorkflow, v1alpha2.KindPolicyBundle, v1alpha2.KindAgent,
	}
	for _, k := range kinds {
		k := k
		t.Run(string(k), func(t *testing.T) {
			mt, ok := aipack.MediaTypeForKind(k)
			if !ok {
				t.Fatalf("MediaTypeForKind(%s) returned false", k)
			}
			if mt == "" {
				t.Fatalf("MediaTypeForKind(%s) returned empty string", k)
			}
		})
	}
}

// TestErrorCodes validates all 37 error codes are defined and distinct.
func TestErrorCodes(t *testing.T) {
	codes := []aipack.ErrorCode{
		// KIND
		aipack.ErrKindUnknown, aipack.ErrFamilyMismatch,
		// COMP
		aipack.ErrTagOnlyRef, aipack.ErrCyclicDAG, aipack.ErrDAGDepthExceeded, aipack.ErrSlotTypeMismatch,
		// ATTEST
		aipack.ErrMissingPredicate, aipack.ErrInvalidSignature, aipack.ErrExpiredPredicate, aipack.ErrUnresolvableRef,
		// COMPAT
		aipack.ErrLoRABaseRefMissing, aipack.ErrRetrievalEmbedMismatch,
		// RUNTIME
		aipack.ErrMediaTypeMismatch, aipack.ErrDigestVerifyFailed,
		// LIN
		aipack.ErrLineageEnvelopeMissing, aipack.ErrLineageHashMismatch,
		// BLAST
		aipack.ErrBlastRadiusExceeded, aipack.ErrDependencyIndexStale,
		// RV
		aipack.ErrRVWeightsSumInvalid, aipack.ErrRVRedBandBlocked,
		// DEP
		aipack.ErrDeprecationBlocked, aipack.ErrSunsetExpired,
		// TEA
		aipack.ErrTEAEndpointUnreachable, aipack.ErrTEAQueryInvalid,
		// OUTLIER
		aipack.ErrOutlierUnacknowledged,
		// AIRGAP
		aipack.ErrAirGapBundleExpired, aipack.ErrAirGapTrustRootMissing, aipack.ErrAirGapTSAMissing,
		// PATTERN
		aipack.ErrPatternViolation, aipack.ErrManifoldDistanceExceeded,
		// PROFILE
		aipack.ErrProfileFamilyDenied, aipack.ErrProfilePredicateDenied,
		// TRIGGER
		aipack.ErrQuarantineTriggerFired, aipack.ErrQuarantineEscalationFail,
		// VAD
		aipack.ErrVADConsensusFailed, aipack.ErrVADClassUnknown, aipack.ErrVADPerturbationFail,
	}
	seen := make(map[aipack.ErrorCode]bool)
	for _, c := range codes {
		if c == "" {
			t.Fatalf("empty error code in codes list")
		}
		if seen[c] {
			t.Fatalf("duplicate error code: %s", c)
		}
		seen[c] = true
	}
	if len(seen) != 37 {
		t.Fatalf("expected 37 distinct error codes, got %d", len(seen))
	}
}

// TestRiskValence covers the 4 band thresholds per AIPACK-SPEC §13.
func TestRiskValence(t *testing.T) {
	vectors := []struct {
		id    string
		score int
		want  v1alpha2.RVBand
	}{
		{id: "V-RV-001", score: 0, want: v1alpha2.RVBandGreen},
		{id: "V-RV-002", score: 24, want: v1alpha2.RVBandGreen},
		{id: "V-RV-003", score: 25, want: v1alpha2.RVBandYellow},
		{id: "V-RV-004", score: 49, want: v1alpha2.RVBandYellow},
		{id: "V-RV-005", score: 50, want: v1alpha2.RVBandOrange},
		{id: "V-RV-006", score: 74, want: v1alpha2.RVBandOrange},
		{id: "V-RV-007", score: 75, want: v1alpha2.RVBandRed},
		{id: "V-RV-008", score: 100, want: v1alpha2.RVBandRed},
	}
	for _, v := range vectors {
		v := v
		t.Run(v.id, func(t *testing.T) {
			got := aipack.BandForScore(v.score)
			if got != v.want {
				t.Fatalf("[%s] score %d: want band %s got %s", v.id, v.score, v.want, got)
			}
		})
	}
}

// assertError is a test helper that checks error codes.
func assertError(t *testing.T, id string, err error, wantCode string) {
	t.Helper()
	if wantCode == "" {
		if err != nil {
			t.Fatalf("[%s] unexpected error: %v", id, err)
		}
		return
	}
	if err == nil {
		t.Fatalf("[%s] expected error with code %s but got nil", id, wantCode)
	}
	var aipErr *aipack.AIPackError
	if !errors.As(err, &aipErr) {
		t.Fatalf("[%s] expected *aipack.AIPackError, got %T: %v", id, err, err)
	}
	if string(aipErr.Code) != wantCode {
		t.Fatalf("[%s] wantCode=%s got=%s msg=%s", id, wantCode, aipErr.Code, aipErr.Message)
	}
}
