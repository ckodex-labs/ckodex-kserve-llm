package governance

import (
	"context"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
	"github.com/stretchr/testify/require"
)

func TestVerifyAIPackAttestation_DoesNotTreatPresenceAsProof(t *testing.T) {
	kind := servingv1alpha2.KindBaseModel
	predicates := make([]servingv1alpha2.PredicateEntry, 0, len(aipack.RequiredPredicates(kind)))
	for _, predicate := range aipack.RequiredPredicates(kind) {
		predicates = append(predicates, servingv1alpha2.PredicateEntry{PredicateURI: predicate})
	}

	result, err := VerifyAIPackAttestation(
		context.Background(),
		kind,
		"registry.example.com/model@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		&servingv1alpha2.AIPackAttestation{Predicates: predicates},
	)

	require.NoError(t, err)
	require.False(t, result.Verified)
	require.Contains(t, result.FailedPredicates, "cosign-signature")
	require.Contains(t, result.Message, "cryptographic cosign verification is not implemented")
}
