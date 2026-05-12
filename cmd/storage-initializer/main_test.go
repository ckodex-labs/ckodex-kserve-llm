package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssessProvenance_FindsArtifacts(t *testing.T) {
	tempDir := t.TempDir()
	artifact := filepath.Join(tempDir, "model.sig")
	require.NoError(t, os.WriteFile(artifact, []byte("sig"), 0o600))

	assessment, err := assessProvenance("hf://org/model", "hf", tempDir, verifierConfig{})
	require.NoError(t, err)
	assert.Equal(t, []string{artifact}, assessment.ArtifactPaths)
	assert.False(t, assessment.CryptographicallyVerified)
}

func TestAssessProvenance_FailsWhenOfflineKeyMissing(t *testing.T) {
	tempDir := t.TempDir()

	_, err := assessProvenance("oci://registry.example.com/model:latest", "oci", tempDir, verifierConfig{
		LocalKeyPath: filepath.Join(tempDir, "missing.pub"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configured offline verification key")
}

func TestVerifyOCIProvenance_UsesCosignForSignatureAttestationAndSBOM(t *testing.T) {
	tempDir := t.TempDir()
	callsFile := filepath.Join(tempDir, "calls.log")
	cosignScript := filepath.Join(tempDir, "cosign.sh")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"" + callsFile + "\"\n" +
		"case \"$1\" in\n" +
		"  verify)\n" +
		"    printf '[{\"critical\":{\"image\":{\"docker-manifest-digest\":\"sha256:feedface\"}}}]'\n" +
		"    ;;\n" +
		"  verify-attestation)\n" +
		"    printf '[{\"payloadType\":\"%s\"}]' \"$4\"\n" +
		"    ;;\n" +
		"  *)\n" +
		"    exit 1\n" +
		"    ;;\n" +
		"esac\n"
	require.NoError(t, os.WriteFile(cosignScript, []byte(script), 0o700))

	originalCosignPath := cosignBinaryPath
	cosignBinaryPath = cosignScript
	defer func() { cosignBinaryPath = originalCosignPath }()

	keyPath := filepath.Join(tempDir, "cosign.pub")
	require.NoError(t, os.WriteFile(keyPath, []byte("pubkey"), 0o600))

	record, err := verifyOCIProvenance("oci://registry.example.com/model:latest", verifierConfig{
		LocalKeyPath: keyPath,
	})
	require.NoError(t, err)
	assert.True(t, record.Verified())
	assert.Equal(t, "sha256:feedface", record.SignatureDigest)
	assert.Equal(t, "oci://registry.example.com/model:latest#attestation:slsaprovenance1", record.AttestationURI)
	assert.NotEmpty(t, record.SBOMDigest)

	calls, err := os.ReadFile(callsFile)
	require.NoError(t, err)
	assert.Contains(t, string(calls), "verify --key "+keyPath+" registry.example.com/model:latest")
	assert.Contains(t, string(calls), "verify-attestation --key "+keyPath+" --type slsaprovenance1 registry.example.com/model:latest")
	assert.Contains(t, string(calls), "verify-attestation --key "+keyPath+" --type cyclonedx registry.example.com/model:latest")
}

func TestVerifyOCIProvenance_AcceptsOCISScheme(t *testing.T) {
	tempDir := t.TempDir()
	cosignScript := filepath.Join(tempDir, "cosign.sh")
	script := "#!/bin/sh\n" +
		"printf '[{\"critical\":{\"image\":{\"docker-manifest-digest\":\"sha256:feedface\"}}}]'\n"
	require.NoError(t, os.WriteFile(cosignScript, []byte(script), 0o700))

	originalCosignPath := cosignBinaryPath
	cosignBinaryPath = cosignScript
	defer func() { cosignBinaryPath = originalCosignPath }()

	keyPath := filepath.Join(tempDir, "cosign.pub")
	require.NoError(t, os.WriteFile(keyPath, []byte("pubkey"), 0o600))

	record, err := verifyOCIProvenance("ocis://registry.example.com/model:latest", verifierConfig{
		LocalKeyPath: keyPath,
	})
	require.NoError(t, err)
	assert.Equal(t, "ocis", record.Scheme)
	assert.Equal(t, "ocis://registry.example.com/model:latest", record.Subject)
}

func TestLoadCachedRuntimeVerificationRecord(t *testing.T) {
	tempDir := t.TempDir()
	record := provenance.RuntimeVerificationRecord{
		Subject:             "oci://registry.example.com/model@sha256:abc",
		Scheme:              "oci",
		SignatureVerified:   true,
		AttestationVerified: true,
		SBOMVerified:        true,
		SignatureDigest:     "sha256:abc",
		AttestationURI:      "oci://registry.example.com/model@sha256:abc#attestation:slsaprovenance1",
		SBOMDigest:          "sha256:def",
	}

	require.NoError(t, writeCacheRuntimeVerificationRecord(tempDir, record))

	loaded, err := loadCachedRuntimeVerificationRecord(tempDir)
	require.NoError(t, err)

	encodedExpected, err := json.Marshal(record)
	require.NoError(t, err)
	encodedLoaded, err := json.Marshal(loaded)
	require.NoError(t, err)
	assert.JSONEq(t, string(encodedExpected), string(encodedLoaded))
}
