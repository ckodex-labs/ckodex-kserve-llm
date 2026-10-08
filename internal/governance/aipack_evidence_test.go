package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyAIPackAttestation_DoesNotTreatPresenceAsProof(t *testing.T) {
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_REF", "")
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_PATH", "")
	t.Setenv("CKODEX_COSIGN_CERT_IDENTITY", "")
	t.Setenv("CKODEX_COSIGN_CERT_OIDC_ISSUER", "")
	kind := servingv1alpha2.KindBaseModel

	result, err := VerifyAIPackAttestation(
		context.Background(),
		kind,
		"registry.example.com/model@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		makeAttestationWithRequiredPredicates(kind),
	)

	require.NoError(t, err)
	require.False(t, result.Verified)
	require.Contains(t, result.FailedPredicates, "cosign-signature")
	require.Contains(t, result.Message, "verification config is invalid")
}

func TestVerifyAIPackAttestation_VerifiesRequiredPredicatesAndSignature(t *testing.T) {
	tempDir := t.TempDir()
	callsFile := filepath.Join(tempDir, "calls.log")
	cosignScript := filepath.Join(tempDir, "cosign.sh")
	writeCosignFakeScript(t, cosignScript, callsFile)

	originalCosignPath := cosignBinaryPath
	cosignBinaryPath = cosignScript
	defer func() { cosignBinaryPath = originalCosignPath }()

	keyPath := filepath.Join(tempDir, "cosign.pub")
	require.NoError(t, os.WriteFile(keyPath, []byte("public-key"), 0o600))
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_REF", "env://COSIGN_PUBKEY")
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_PATH", keyPath)
	t.Setenv("CKODEX_COSIGN_REKOR_URL", "https://rekor.example.test")

	kind := servingv1alpha2.KindBaseModel
	predicates := makePredicateEntries(kind)
	predicates[0].Digest = fixturePredicateDigest()

	result, err := VerifyAIPackAttestation(
		context.Background(),
		kind,
		"oci://registry.example.com/model@sha256:feedface",
		&servingv1alpha2.AIPackAttestation{
			Predicates:   predicates,
			CosignKeyRef: "env://COSIGN_PUBKEY",
		},
	)

	require.NoError(t, err)
	assert.True(t, result.Verified)
	assert.Empty(t, result.FailedPredicates)

	calls, err := os.ReadFile(callsFile)
	require.NoError(t, err)
	callText := string(calls)
	assert.Contains(t, callText, "verify --key "+keyPath+" --rekor-url https://rekor.example.test registry.example.com/model@sha256:feedface")
	for _, predicate := range aipack.RequiredPredicates(kind) {
		assert.Contains(t, callText, "verify-attestation --key "+keyPath+" --rekor-url https://rekor.example.test --type "+predicate+" --output text registry.example.com/model@sha256:feedface")
	}
}

func TestVerifyAIPackAttestation_UsesOIDCIdentityAndIssuerWhenNoKey(t *testing.T) {
	tempDir := t.TempDir()
	callsFile := filepath.Join(tempDir, "calls.log")
	cosignScript := filepath.Join(tempDir, "cosign.sh")
	writeCosignFakeScript(t, cosignScript, callsFile)

	originalCosignPath := cosignBinaryPath
	cosignBinaryPath = cosignScript
	defer func() { cosignBinaryPath = originalCosignPath }()

	identity := "spiffe://tenant/repo"
	issuer := "https://token.actions.githubusercontent.com"
	t.Setenv("CKODEX_COSIGN_CERT_IDENTITY", identity)
	t.Setenv("CKODEX_COSIGN_CERT_OIDC_ISSUER", issuer)
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_REF", "")
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_PATH", "")

	kind := servingv1alpha2.KindBaseModel
	predicates := makePredicateEntries(kind)

	result, err := VerifyAIPackAttestation(
		context.Background(),
		kind,
		"registry.example.com/model@sha256:feedface",
		&servingv1alpha2.AIPackAttestation{Predicates: predicates},
	)

	require.NoError(t, err)
	assert.True(t, result.Verified)

	calls, err := os.ReadFile(callsFile)
	require.NoError(t, err)
	callText := string(calls)
	assert.True(t, strings.TrimSpace(callText) != "", "script call log should be non-empty")
	assert.Contains(t, callText, "--certificate-identity "+identity)
	assert.Contains(t, callText, "--certificate-oidc-issuer "+issuer)
}

func TestVerifyAIPackAttestation_FailsWhenVerifierConfigMissing(t *testing.T) {
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_PATH", "")
	t.Setenv("CKODEX_COSIGN_CERT_IDENTITY", "identity-without-issuer")
	t.Setenv("CKODEX_COSIGN_CERT_OIDC_ISSUER", "")
	result, err := VerifyAIPackAttestation(
		context.Background(),
		servingv1alpha2.KindBaseModel,
		"registry.example.com/model@sha256:feedface",
		makeAttestationWithRequiredPredicates(servingv1alpha2.KindBaseModel),
	)

	require.NoError(t, err)
	require.False(t, result.Verified)
	require.Equal(t, []string{"cosign-signature"}, result.FailedPredicates)
	require.Contains(t, result.Message, "verification config is invalid")
}

func TestVerifyAIPackAttestation_FailsWhenVerifiedPredicateDigestDoesNotMatch(t *testing.T) {
	tempDir := t.TempDir()
	callsFile := filepath.Join(tempDir, "calls.log")
	cosignScript := filepath.Join(tempDir, "cosign.sh")
	writeCosignFakeScript(t, cosignScript, callsFile)

	originalCosignPath := cosignBinaryPath
	cosignBinaryPath = cosignScript
	defer func() { cosignBinaryPath = originalCosignPath }()

	keyPath := filepath.Join(tempDir, "cosign.pub")
	require.NoError(t, os.WriteFile(keyPath, []byte("public-key"), 0o600))
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_REF", "env://COSIGN_PUBKEY")
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_PATH", keyPath)

	kind := servingv1alpha2.KindBaseModel
	predicates := makePredicateEntries(kind)
	predicates[0].Digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	result, err := VerifyAIPackAttestation(
		context.Background(),
		kind,
		"registry.example.com/model@sha256:feedface",
		&servingv1alpha2.AIPackAttestation{Predicates: predicates},
	)

	require.NoError(t, err)
	require.False(t, result.Verified)
	require.Contains(t, result.FailedPredicates, aipack.RequiredPredicates(kind)[0])
}

func TestVerifyAIPackAttestation_DoesNotTrustAIPackSelectedKey(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "attacker.pub")
	require.NoError(t, os.WriteFile(keyPath, []byte("attacker-controlled-key"), 0o600))
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_PATH", "")
	t.Setenv("CKODEX_COSIGN_TRUSTED_KEY_REF", "")
	t.Setenv("CKODEX_COSIGN_CERT_IDENTITY", "")
	t.Setenv("CKODEX_COSIGN_CERT_OIDC_ISSUER", "")

	result, err := VerifyAIPackAttestation(
		context.Background(),
		servingv1alpha2.KindBaseModel,
		"registry.example.com/model@sha256:feedface",
		&servingv1alpha2.AIPackAttestation{
			Predicates:   makePredicateEntries(servingv1alpha2.KindBaseModel),
			CosignKeyRef: keyPath,
		},
	)

	require.NoError(t, err)
	require.False(t, result.Verified)
	require.Equal(t, []string{"cosign-signature"}, result.FailedPredicates)
	require.Contains(t, result.Message, "not in the operator trusted-key configuration")
}

func TestVerifyAIPackAttestation_RejectsUnsupportedArtifactScheme(t *testing.T) {
	result, err := VerifyAIPackAttestation(
		context.Background(),
		servingv1alpha2.KindBaseModel,
		"registry://example.com/model",
		&servingv1alpha2.AIPackAttestation{
			Predicates:   makePredicateEntries(servingv1alpha2.KindBaseModel),
			CosignKeyRef: "test-key",
		},
	)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "unsupported artifact scheme")
}

func TestVerifyAIPackAttestation_RejectsUnboundRekorLogID(t *testing.T) {
	attestation := makeAttestationWithRequiredPredicates(servingv1alpha2.KindBaseModel)
	attestation.Predicates[0].RekorLogID = "unverified-entry-id"

	result, err := VerifyAIPackAttestation(
		context.Background(),
		servingv1alpha2.KindBaseModel,
		"registry.example.com/model@sha256:feedface",
		attestation,
	)

	require.NoError(t, err)
	require.False(t, result.Verified)
	require.Contains(t, result.FailedPredicates, aipack.RequiredPredicates(servingv1alpha2.KindBaseModel)[0])
}

func TestVerifyAIPackAttestation_FailsOnEmptyPredicateURI(t *testing.T) {
	kind := servingv1alpha2.KindTool
	attestation := makeAttestationWithRequiredPredicates(kind)
	if len(attestation.Predicates) > 0 {
		attestation.Predicates[0].PredicateURI = ""
	}
	attestation.CosignKeyRef = "test-key"

	result, err := VerifyAIPackAttestation(
		context.Background(),
		kind,
		"registry.example.com/tool@sha256:facefeed",
		attestation,
	)

	require.NoError(t, err)
	require.False(t, result.Verified)
	require.Contains(t, result.FailedPredicates, "entry[0]")
	require.Contains(t, result.Message, "empty PredicateURI")
}

func makePredicateEntries(kind servingv1alpha2.ArtifactKind) []servingv1alpha2.PredicateEntry {
	predicates := make([]servingv1alpha2.PredicateEntry, 0, len(aipack.RequiredPredicates(kind)))
	for _, predicate := range aipack.RequiredPredicates(kind) {
		predicates = append(predicates, servingv1alpha2.PredicateEntry{PredicateURI: predicate})
	}
	return predicates
}

func fixturePredicateDigest() string {
	digest := sha256.Sum256([]byte("{\"source\":\"fixture\"}"))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func makeAttestationWithRequiredPredicates(kind servingv1alpha2.ArtifactKind) *servingv1alpha2.AIPackAttestation {
	return &servingv1alpha2.AIPackAttestation{Predicates: makePredicateEntries(kind)}
}

func writeCosignFakeScript(t *testing.T, path, callsFile string) {
	t.Helper()
	body := `#!/bin/sh
echo "$@" >> "` + callsFile + `"
if [ "$1" = "verify" ]; then
  printf 'ok'
  exit 0
elif [ "$1" = "verify-attestation" ]; then
  predicate=""
  previous=""
  for argument in "$@"; do
    if [ "$previous" = "--type" ]; then
      predicate="$argument"
      break
    fi
    previous="$argument"
  done
  printf '{"predicateType":"%s","predicate":{"source":"fixture"}}\n' "$predicate"
  exit 0
else
  exit 1
fi
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o700))
}
