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
//  3. cosign signature verification if CosignKeyRef is set (AIPACK-ATTEST-004)
//
// Full cosign integration is [S] — the current implementation performs
// predicate presence + URI sanity checks only.
// TODO(ckodex): integrate cosign.VerifyImageAttestations per AIPACK-SPEC §7
func VerifyAIPackAttestation(_ context.Context, kind servingv1alpha2.ArtifactKind, ref string, attestation *servingv1alpha2.AIPackAttestation) (*AIPackVerificationResult, error) {
	if attestation == nil {
		required := aipack.RequiredPredicates(kind)
		if len(required) > 0 {
			return &AIPackVerificationResult{
				Verified:         false,
				VerifiedAt:       time.Now().UTC(),
				FailedPredicates: required,
				Message:          fmt.Sprintf("artifact %q (kind %s) has no attestation block but requires %d predicate(s)", ref, kind, len(required)),
			}, nil
		}
		return &AIPackVerificationResult{
			Verified:   true,
			VerifiedAt: time.Now().UTC(),
			Message:    "no attestation required for kind",
		}, nil
	}

	present := predicateURIs(attestation)
	missing := aipack.MissingPredicates(kind, present)
	if len(missing) > 0 {
		return &AIPackVerificationResult{
			Verified:         false,
			VerifiedAt:       time.Now().UTC(),
			FailedPredicates: missing,
			Message:          fmt.Sprintf("artifact %q is missing required predicates: %v", ref, missing),
		}, nil
	}

	var emptyURI []string
	for _, pred := range attestation.Predicates {
		if pred.PredicateURI == "" {
			emptyURI = append(emptyURI, fmt.Sprintf("entry[%d]", len(emptyURI)))
		}
	}
	if len(emptyURI) > 0 {
		return &AIPackVerificationResult{
			Verified:         false,
			VerifiedAt:       time.Now().UTC(),
			FailedPredicates: emptyURI,
			Message:          fmt.Sprintf("artifact %q has %d predicate entries with empty PredicateURI", ref, len(emptyURI)),
		}, nil
	}

	return &AIPackVerificationResult{
		Verified:   true,
		VerifiedAt: time.Now().UTC(),
		Message:    fmt.Sprintf("artifact %q predicate presence verified (%d predicate(s)); cosign verification deferred [S]", ref, len(attestation.Predicates)),
	}, nil
}
