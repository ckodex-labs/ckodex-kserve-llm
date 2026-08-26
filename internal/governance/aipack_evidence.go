/*
Copyright 2026 CKodex Authors.
*/

package governance

import (
	"context"
	"fmt"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
)

// HasRequiredAIPackAttestations reports whether the attestation block contains
// entries for all predicates required by the artifact's kind. It does not
// perform cryptographic verification — use VerifyAIPackAttestation for that.
func HasRequiredAIPackAttestations(kind servingv1alpha2.ArtifactKind, attestation *servingv1alpha2.AIPackAttestation) bool {
	if attestation == nil {
		return len(aipack.RequiredPredicates(kind)) == 0
	}
	present := predicateURIs(attestation)
	return len(aipack.MissingPredicates(kind, present)) == 0
}

// AIPackVerificationResult records the outcome of attestation verification.
type AIPackVerificationResult struct {
	// Verified reports whether all required predicates verified successfully.
	Verified bool

	// VerifiedAt is the time the verification completed (UTC).
	VerifiedAt time.Time

	// FailedPredicates lists predicates that failed verification.
	FailedPredicates []string

	// Message is a human-readable summary.
	Message string
}

// VerifyAIPackAttestation verifies the cosign attestation for an AIPack artifact.
// It checks:
//  1. All required predicates for kind are present (AIPACK-ATTEST-001/002)
//  2. Predicate entries have non-empty URIs (AIPACK-ATTEST-003)
//  3. cryptographic verification is required before a positive result
//
// Full cosign integration is [S]. Until it is wired, predicate presence is
// deliberately insufficient and this function returns Verified=false.
// TODO(ckodex): integrate cosign.VerifyImageAttestations per AIPACK-SPEC §7
func VerifyAIPackAttestation(_ context.Context, kind servingv1alpha2.ArtifactKind, ref string, attestation *servingv1alpha2.AIPackAttestation) (*AIPackVerificationResult, error) {
	if ref == "" {
		return missingArtifactReferenceResult(), nil
	}
	if attestation == nil {
		return verifyMissingAttestation(kind, ref), nil
	}

	present := predicateURIs(attestation)
	missing := aipack.MissingPredicates(kind, present)
	if len(missing) > 0 {
		return missingPredicatesResult(ref, missing), nil
	}

	emptyURI := emptyPredicateEntries(attestation)
	if len(emptyURI) > 0 {
		return emptyPredicateResult(ref, emptyURI), nil
	}

	return unavailableCosignResult(ref), nil
}

func missingArtifactReferenceResult() *AIPackVerificationResult {
	return &AIPackVerificationResult{
		Verified:         false,
		VerifiedAt:       time.Now().UTC(),
		FailedPredicates: []string{"artifact-reference"},
		Message:          "artifact reference is required for attestation verification",
	}
}

func verifyMissingAttestation(kind servingv1alpha2.ArtifactKind, ref string) *AIPackVerificationResult {
	required := aipack.RequiredPredicates(kind)
	if len(required) == 0 {
		return &AIPackVerificationResult{
			Verified:   true,
			VerifiedAt: time.Now().UTC(),
			Message:    "no attestation required for kind",
		}
	}
	return &AIPackVerificationResult{
		Verified:         false,
		VerifiedAt:       time.Now().UTC(),
		FailedPredicates: required,
		Message:          fmt.Sprintf("artifact %q (kind %s) has no attestation block but requires %d predicate(s)", ref, kind, len(required)),
	}
}

func missingPredicatesResult(ref string, missing []string) *AIPackVerificationResult {
	return &AIPackVerificationResult{
		Verified:         false,
		VerifiedAt:       time.Now().UTC(),
		FailedPredicates: missing,
		Message:          fmt.Sprintf("artifact %q is missing required predicates: %v", ref, missing),
	}
}

func emptyPredicateEntries(attestation *servingv1alpha2.AIPackAttestation) []string {
	var empty []string
	for _, predicate := range attestation.Predicates {
		if predicate.PredicateURI == "" {
			empty = append(empty, fmt.Sprintf("entry[%d]", len(empty)))
		}
	}
	return empty
}

func emptyPredicateResult(ref string, empty []string) *AIPackVerificationResult {
	return &AIPackVerificationResult{
		Verified:         false,
		VerifiedAt:       time.Now().UTC(),
		FailedPredicates: empty,
		Message:          fmt.Sprintf("artifact %q has %d predicate entries with empty PredicateURI", ref, len(empty)),
	}
}

func unavailableCosignResult(ref string) *AIPackVerificationResult {
	return &AIPackVerificationResult{
		Verified:         false,
		VerifiedAt:       time.Now().UTC(),
		FailedPredicates: []string{"cosign-signature"},
		Message:          fmt.Sprintf("artifact %q predicates are present but cryptographic cosign verification is not implemented", ref),
	}
}
