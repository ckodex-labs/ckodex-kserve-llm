/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package airgap

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSovereigntyProfiles_CreationAndHierarchy(t *testing.T) {
	profiles := []SovereigntyProfileLevel{
		ProfileConnected, ProfileEnterprise, ProfileRestricted, ProfileDisconnected, ProfileAirGap,
	}
	for _, p := range profiles {
		prof, err := NewSovereigntyProfile(p)
		require.NoError(t, err)
		assert.Equal(t, p, prof.Requirements.Level)
	}

	_, err := NewSovereigntyProfile("InvalidProfile")
	assert.ErrorIs(t, err, ErrUnknownProfile)

	airGapProf, err := NewSovereigntyProfile(ProfileAirGap)
	require.NoError(t, err)
	assert.True(t, airGapProf.Requirements.ZeroPhoneHome)
	assert.True(t, airGapProf.Requirements.RequireAirGapCapsule)
	assert.True(t, airGapProf.Requirements.RequireDigestPinning)
	assert.True(t, airGapProf.Requirements.RequireLocalSecrets)
}

func TestWorkloadValidation_ConnectedAndEnterprise(t *testing.T) {
	connTarget := WorkloadTarget{
		Name:            "conn-svc",
		ImageURI:        "quay.io/vllm/vllm:v0.4.0",
		ModelURI:        "hf://google/gemma-4",
		SecretStoreType: "kms",
		EgressTarget:    "api.openai.com",
	}
	vec := ValidateWorkload(ProfileConnected, connTarget)
	assert.Equal(t, StateCompliant, vec.State)
	assert.Equal(t, PostureConnected, vec.Network)

	entTarget := WorkloadTarget{
		Name:            "ent-svc",
		ImageURI:        "corp.internal/vllm:v0.4.0",
		ModelURI:        "oci://corp.internal/models/gemma",
		SPIFFEIdentity:  "spiffe://cluster.local/ns/default/sa/llm-runner",
		HasSignature:    true,
		SecretStoreType: "vault",
	}
	vecEnt := ValidateWorkload(ProfileEnterprise, entTarget)
	assert.Equal(t, StateCompliant, vecEnt.State)
	assert.Equal(t, PostureEnterpriseProxy, vecEnt.Network)
}

func TestWorkloadValidation_AirGapComplianceAndViolations(t *testing.T) {
	digest := "sha256:" + hex.EncodeToString(sha256.New().Sum([]byte("dummy-weights")))[:64]
	airGapTarget := WorkloadTarget{
		Name:            "airgap-svc",
		ImageURI:        "local.registry/vllm@" + digest,
		ModelURI:        "local.registry/models/gemma@" + digest,
		SPIFFEIdentity:  "spiffe://cluster.local/ns/default/sa/llm-runner",
		SecretStoreType: "localseeded",
		HasSignature:    true,
		HasAttestation:  true,
		HasSBOM:         true,
		HasAIBOM:        true,
	}
	vec := ValidateWorkload(ProfileAirGap, airGapTarget)
	assert.Equal(t, StateCompliant, vec.State)
	assert.Equal(t, PostureAirGapZeroEgress, vec.Network)
	assert.Empty(t, vec.Violations)

	// Violation: Phone-home attempt in air gap
	airGapTarget.EgressTarget = "10.0.0.1:443"
	vecViolated := ValidateWorkload(ProfileAirGap, airGapTarget)
	assert.Equal(t, StateNonCompliant, vecViolated.State)
	assert.Contains(t, vecViolated.Violations[0], "external network egress prohibited")
}

func TestWorkloadValidation_DigestPinningEnforcement(t *testing.T) {
	targetUnpinned := WorkloadTarget{
		Name:            "unpinned-svc",
		ImageURI:        "local.registry/vllm:latest",
		SPIFFEIdentity:  "spiffe://cluster.local/ns/default/sa/runner",
		SecretStoreType: "local",
		HasSignature:    true,
		HasAttestation:  true,
		HasSBOM:         true,
		HasAIBOM:        true,
	}
	vec := ValidateWorkload(ProfileRestricted, targetUnpinned)
	assert.Equal(t, StateNonCompliant, vec.State)
	assert.Equal(t, DigestUnpinned, vec.DigestPinning)

	targetMalformed := targetUnpinned
	targetMalformed.ImageURI = "local.registry/vllm@sha256:bad"
	vecMalformed := ValidateWorkload(ProfileRestricted, targetMalformed)
	assert.Equal(t, StateNonCompliant, vecMalformed.State)
	assert.Equal(t, DigestMalformed, vecMalformed.DigestPinning)
}

func helperMakeValidCapsule(now time.Time) *TransferCapsule {
	rawWeights := "safetensors-model-weights-bytes"
	h := sha256.Sum256([]byte(rawWeights))
	weightsDigest := "sha256:" + hex.EncodeToString(h[:])
	keyFP := "SHA256:trustedKeyFingerprint0123456789"

	return &TransferCapsule{
		APIVersion: "security.ckodex.com/v1alpha1",
		Kind:       "AirGapTransferCapsule",
		Metadata: CapsuleMetadata{
			ID:                "capsule-uuid-1234",
			CreatedAt:         now.Add(-1 * time.Hour),
			ExpiresAt:         now.Add(24 * time.Hour),
			TargetProfile:     ProfileAirGap,
			SourceEnvironment: "secure-export-zone",
		},
		Artifacts: []CapsuleArtifact{
			{Name: "model-weights", MediaType: "application/vnd.ckodex.model.weights", Digest: weightsDigest, Size: int64(len(rawWeights)), Payload: rawWeights},
		},
		Evidence: CapsuleEvidence{
			Signatures: []SignatureRecord{
				{ArtifactDigest: weightsDigest, SignerIdentity: "release@ckodex.com", KeyFingerprint: keyFP, SignatureValue: "valid-sig", Timestamp: now.Add(-1 * time.Hour)},
			},
			Attestations: []AttestationRecord{
				{PredicateType: "slsa.dev/provenance/v1", SubjectDigest: weightsDigest, PayloadDigest: weightsDigest, StatementJSON: "{}"},
			},
			AIBOM: &AIBOMRecord{
				ModelName: "gemma-4", ModelVersion: "1.0.0", Architecture: "transformer", Parameters: "9B", WeightsDigest: weightsDigest, Format: "safetensors",
			},
			SBOM: &SBOMRecord{Format: "CycloneDX", Digest: weightsDigest, Components: []string{"safetensors", "torch"}},
		},
		Policies: CapsulePolicies{
			TrustedKeyFingerprints: []string{keyFP},
			AllowedRegistries:      []string{"registry.corp.internal"},
			ZeroPhoneHomeRequired:  true,
		},
	}
}

func TestCapsuleValidation_AirGapSuccess(t *testing.T) {
	now := time.Now()
	capsule := helperMakeValidCapsule(now)

	vec, err := ValidateCapsule(capsule, ProfileAirGap, now)
	require.NoError(t, err)
	assert.Equal(t, StateCompliant, vec.State)
	assert.Equal(t, PostureAirGapZeroEgress, vec.Network)
	assert.Equal(t, DigestPinned, vec.DigestPinning)
	assert.Equal(t, SigVerified, vec.SignatureIntegrity)
	assert.Equal(t, AttestationVerified, vec.AttestationIntegrity)
	assert.Equal(t, BOMComplete, vec.BOMIntegrity)
}

func TestCapsuleValidation_PayloadTampering(t *testing.T) {
	now := time.Now()
	capsule := helperMakeValidCapsule(now)
	capsule.Artifacts[0].Payload = "tampered-corrupted-bytes"

	vec, err := ValidateCapsule(capsule, ProfileAirGap, now)
	assert.ErrorIs(t, err, ErrDigestMismatch)
	assert.Equal(t, StateNonCompliant, vec.State)
}

func TestCapsuleValidation_ExpiryAndMetadata(t *testing.T) {
	now := time.Now()
	_, err := ValidateCapsule(nil, ProfileAirGap, now)
	assert.ErrorIs(t, err, ErrNilCapsule)

	capsuleExpired := helperMakeValidCapsule(now)
	capsuleExpired.Metadata.ExpiresAt = now.Add(-10 * time.Minute)
	_, err = ValidateCapsule(capsuleExpired, ProfileAirGap, now)
	assert.ErrorIs(t, err, ErrCapsuleExpired)

	capsuleIncompatible := helperMakeValidCapsule(now)
	capsuleIncompatible.Metadata.TargetProfile = ProfileConnected
	_, err = ValidateCapsule(capsuleIncompatible, ProfileAirGap, now)
	assert.ErrorIs(t, err, ErrIncompatibleProfile)
}

func TestCapsuleValidation_UntrustedKeyAndUnknownArtifact(t *testing.T) {
	now := time.Now()
	capsuleUntrustedKey := helperMakeValidCapsule(now)
	capsuleUntrustedKey.Policies.TrustedKeyFingerprints = []string{"SHA256:differentKey"}
	vec, err := ValidateCapsule(capsuleUntrustedKey, ProfileAirGap, now)
	require.NoError(t, err)
	assert.Equal(t, StateNonCompliant, vec.State)
	assert.Equal(t, SigUntrusted, vec.SignatureIntegrity)

	capsuleUnknownArt := helperMakeValidCapsule(now)
	capsuleUnknownArt.Evidence.Signatures[0].ArtifactDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	vecUnknown, err := ValidateCapsule(capsuleUnknownArt, ProfileAirGap, now)
	require.NoError(t, err)
	assert.Equal(t, StateNonCompliant, vecUnknown.State)
	assert.Equal(t, SigUntrusted, vecUnknown.SignatureIntegrity)
}

func TestCapsuleValidation_MissingBOMAndZeroPhoneHome(t *testing.T) {
	now := time.Now()
	capsuleNoBOM := helperMakeValidCapsule(now)
	capsuleNoBOM.Evidence.AIBOM = nil
	vec, err := ValidateCapsule(capsuleNoBOM, ProfileAirGap, now)
	require.NoError(t, err)
	assert.Equal(t, StateNonCompliant, vec.State)
	assert.Equal(t, BOMPartial, vec.BOMIntegrity)

	capsulePhoneHome := helperMakeValidCapsule(now)
	capsulePhoneHome.Policies.ZeroPhoneHomeRequired = false
	_, err = ValidateCapsule(capsulePhoneHome, ProfileAirGap, now)
	assert.ErrorIs(t, err, ErrPhoneHomeNotForbidden)
}

func TestCapsule_ExportImportRoundtrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	orig := helperMakeValidCapsule(now)

	data, err := ExportCapsule(orig)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	imported, err := ImportCapsule(data)
	require.NoError(t, err)
	assert.Equal(t, orig.Metadata.ID, imported.Metadata.ID)
	assert.Equal(t, orig.Artifacts[0].Digest, imported.Artifacts[0].Digest)

	_, err = ExportCapsule(nil)
	assert.ErrorIs(t, err, ErrNilCapsule)

	_, err = ImportCapsule(nil)
	assert.Error(t, err)
}
