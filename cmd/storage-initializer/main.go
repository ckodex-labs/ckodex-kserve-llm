/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
	"github.com/ckodex-labs/kserve-llm-operator/internal/storage"
)

const (
	verificationStateDir  = ".ckodex"
	verificationStateFile = "runtime-verification.json"
	terminationLogPath    = "/dev/termination-log"
)

var cosignBinaryPath = envOrDefault("CKODEX_COSIGN_BINARY_PATH", "/cosign")

func main() {
	record, err := run(os.Args[1:])
	if err != nil {
		record.Error = err.Error()
	}
	if writeErr := writeRuntimeVerificationRecord(record); writeErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write runtime verification record: %v\n", writeErr)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(args []string) (provenance.RuntimeVerificationRecord, error) {
	// Parse flags before positional arguments.
	var skipChecksum bool
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--skip-checksum":
			skipChecksum = true
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) < 2 {
		return provenance.RuntimeVerificationRecord{}, fmt.Errorf("usage: %s [--skip-checksum] <uri> <destPath>", os.Args[0])
	}

	uri := positional[0]
	destPath := positional[1]

	// Propagate --skip-checksum to env so downstream clients can read it.
	if skipChecksum {
		_ = os.Setenv("SKIP_CHECKSUM", "1")
	}

	record := provenance.RuntimeVerificationRecord{Subject: uri}

	fmt.Printf("Starting CKodex Storage Initializer...\n")

	// Inject secrets from Vault if configured
	ctx := context.Background() // Define ctx earlier for Vault injection
	vaultPath := os.Getenv("VAULT_PATH")
	if vaultPath != "" {
		fmt.Printf("Fetching secrets from Vault path: %s\n", vaultPath)
		if err := storage.InjectVaultSecrets(ctx, vaultPath); err != nil {
			fmt.Printf("Warning: Failed to inject Vault secrets: %v\n", err)
		}
	}

	fmt.Printf("Source URI: %s\n", uri)
	fmt.Printf("Destination: %s\n", destPath)

	if skipChecksum {
		fmt.Printf("Checksum verification: DISABLED (--skip-checksum)\n")
	}

	// Determine the scheme
	parts := strings.SplitN(uri, "://", 2)
	if len(parts) < 2 {
		return record, fmt.Errorf("invalid URI: %s", uri)
	}
	scheme := parts[0]
	record.Scheme = scheme

	// Get the registered client
	client, err := storage.GetClient(scheme)
	if err != nil {
		return record, fmt.Errorf("get storage client: %w", err)
	}

	// Check if destination is already populated (Idempotency / Cache Hit).
	if entries, err := os.ReadDir(destPath); err == nil && len(entries) > 0 {
		if skipChecksum {
			fmt.Printf("Optimization: Destination %s already contains %d files. Skipping download.\n", destPath, len(entries))
			return record, nil
		}

		cachedRecord, cacheErr := loadCachedRuntimeVerificationRecord(destPath)
		if cacheErr == nil && cachedRecord.Subject == uri {
			if cachedRecord.Verified() {
				fmt.Printf("Optimization: Destination %s already contains a verified cache for %s. Skipping download.\n", destPath, uri)
				return *cachedRecord, nil
			}
			if cachedRecord.ContentIntegrityVerified {
				contentDigest, digestErr := directoryDigest(destPath)
				if digestErr == nil && contentDigest == cachedRecord.ContentDigest {
					fmt.Printf("Optimization: Destination %s already contains an integrity-checked cache for %s. Skipping download.\n", destPath, uri)
					return *cachedRecord, nil
				}
			}
		}
	}

	stagingPath, cleanup, err := prepareStagingDirectory(destPath, uri)
	if err != nil {
		return record, err
	}
	defer cleanup()

	// Pull into a transaction-owned directory. The mounted destination is only
	// populated after content integrity and provenance checks have completed.
	if err := client.Pull(ctx, uri, stagingPath); err != nil {
		return record, fmt.Errorf("pull failed: %w", err)
	}

	// Security Hardening: AI-BOM / Provenance Verification
	if !skipChecksum {
		fmt.Printf("Inspecting provenance artifacts (AI-BOM)...\n")
		assessment, err := assessProvenance(uri, scheme, stagingPath, resolveVerifierConfig())
		if err != nil {
			return record, fmt.Errorf("SECURITY FATAL: %w", err)
		}
		record = assessment.Record
		for _, path := range assessment.ArtifactPaths {
			fmt.Printf("Found provenance-related artifact: %s\n", path)
		}
		if record.KeyRef != "" {
			fmt.Printf("Verification material configured via %s\n", record.KeyRef)
		}
		if !record.Verified() {
			if len(assessment.ArtifactPaths) == 0 {
				fmt.Printf("No cryptographic provenance was present; content digest recorded; promotion verification remains unavailable.\n")
			} else {
				fmt.Printf("Warning: provenance material was present, but this binary did not complete a cryptographic verification step.\n")
			}
		} else {
			fmt.Printf("Cryptographic signature, provenance attestation, and SBOM attestation verified for %s\n", uri)
		}
		if writeErr := writeCacheRuntimeVerificationRecord(stagingPath, record); writeErr != nil {
			return record, fmt.Errorf("persist verification state: %w", writeErr)
		}
	} else {
		fmt.Printf("Warning: provenance artifact inspection bypassed via --skip-checksum.\n")
	}
	if err := commitStagingDirectory(stagingPath, destPath, uri); err != nil {
		return record, fmt.Errorf("commit verified model payload: %w", err)
	}

	fmt.Printf("Successfully downloaded model to %s\n", destPath)
	return record, nil
}

type verifierConfig struct {
	LocalKeyPath        string
	LocalKeyPEM         string
	CertificateIdentity string
	CertificateIssuer   string
}

func resolveVerifierConfig() verifierConfig {
	return verifierConfig{
		LocalKeyPath:        os.Getenv("CKODEX_LOCAL_COSIGN_KEY_PATH"),
		LocalKeyPEM:         os.Getenv("CKODEX_LOCAL_COSIGN_PUBLIC_KEY"),
		CertificateIdentity: os.Getenv("CKODEX_COSIGN_CERT_IDENTITY"),
		CertificateIssuer:   os.Getenv("CKODEX_COSIGN_CERT_OIDC_ISSUER"),
	}
}

type provenanceAssessment struct {
	ArtifactPaths             []string
	Record                    provenance.RuntimeVerificationRecord
	CryptographicallyVerified bool
}

func assessProvenance(uri, scheme, destPath string, verifier verifierConfig) (provenanceAssessment, error) {
	assessment := provenanceAssessment{
		ArtifactPaths: make([]string, 0, 3),
		Record: provenance.RuntimeVerificationRecord{
			Subject: uri,
			Scheme:  scheme,
		},
	}

	resolvedVerifier, cleanup, err := materializeVerifierConfig(verifier)
	if err != nil {
		return provenanceAssessment{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	assessment.Record.KeyRef = resolvedVerifier.LocalKeyPath
	assessment.Record.CertificateIdentity = resolvedVerifier.CertificateIdentity
	assessment.Record.CertificateIssuer = resolvedVerifier.CertificateIssuer

	if scheme == "oci" || scheme == "ocis" {
		record, verifyErr := verifyOCIProvenance(uri, resolvedVerifier)
		if verifyErr != nil {
			return provenanceAssessment{}, verifyErr
		}
		assessment.Record = record
		assessment.CryptographicallyVerified = record.Verified()
	} else {
		provenancePaths := []string{
			filepath.Join(destPath, "slsa.provenance.json"),
			filepath.Join(destPath, "provenance.sig"),
			filepath.Join(destPath, "model.sig"),
		}

		for _, path := range provenancePaths {
			if _, err := os.Stat(path); err == nil {
				assessment.ArtifactPaths = append(assessment.ArtifactPaths, path)
			}
		}

		if len(assessment.ArtifactPaths) == 0 {
			fmt.Printf("No cryptographic provenance artifact found for %s; retaining content digest only.\n", uri)
		}
	}

	digest, err := directoryDigest(destPath)
	if err != nil {
		return provenanceAssessment{}, fmt.Errorf("compute content integrity digest: %w", err)
	}
	assessment.Record.ContentDigest = digest
	assessment.Record.ContentIntegrityVerified = true

	return assessment, nil
}

func materializeVerifierConfig(cfg verifierConfig) (verifierConfig, func(), error) {
	if cfg.LocalKeyPath != "" {
		if _, err := os.Stat(cfg.LocalKeyPath); err == nil {
			return cfg, nil, nil
		} else if cfg.LocalKeyPEM == "" {
			return verifierConfig{}, nil, fmt.Errorf("configured offline verification key %q is unavailable: %w", cfg.LocalKeyPath, err)
		}
	}

	if cfg.LocalKeyPEM == "" {
		return cfg, nil, nil
	}

	tempDir, err := os.MkdirTemp("", "ckodex-cosign-key-*")
	if err != nil {
		return verifierConfig{}, nil, fmt.Errorf("create temp cosign key dir: %w", err)
	}
	keyPath := filepath.Join(tempDir, "cosign.pub")
	if err := os.WriteFile(keyPath, []byte(cfg.LocalKeyPEM), 0o600); err != nil {
		_ = os.RemoveAll(tempDir)
		return verifierConfig{}, nil, fmt.Errorf("write temp cosign key: %w", err)
	}
	cfg.LocalKeyPath = keyPath
	return cfg, func() { _ = os.RemoveAll(tempDir) }, nil
}

func verifyOCIProvenance(uri string, verifier verifierConfig) (provenance.RuntimeVerificationRecord, error) {
	if verifier.LocalKeyPath == "" && (verifier.CertificateIdentity == "" || verifier.CertificateIssuer == "") {
		return provenance.RuntimeVerificationRecord{
			Subject: uri,
			Scheme:  "oci",
		}, fmt.Errorf("OCI cryptographic verification requires CKODEX_LOCAL_COSIGN_KEY_PATH, CKODEX_LOCAL_COSIGN_PUBLIC_KEY, or CKODEX_COSIGN_CERT_IDENTITY plus CKODEX_COSIGN_CERT_OIDC_ISSUER")
	}

	ref := storage.TrimOCIScheme(uri)
	record := provenance.RuntimeVerificationRecord{
		Subject:             uri,
		Scheme:              "oci",
		KeyRef:              verifier.LocalKeyPath,
		CertificateIdentity: verifier.CertificateIdentity,
		CertificateIssuer:   verifier.CertificateIssuer,
	}
	if strings.HasPrefix(uri, "ocis://") {
		record.Scheme = "ocis"
	}

	signatureOutput, err := runCosign(ref, verifier, "verify")
	if err != nil {
		return record, fmt.Errorf("verify OCI signature for %s: %w", uri, err)
	}
	record.SignatureVerified = true
	record.SignatureDigest = extractSignatureDigest(signatureOutput)

	provenanceOutput, err := runCosign(ref, verifier, "verify-attestation", "--type", "slsaprovenance1")
	if err != nil {
		return record, fmt.Errorf("verify SLSA provenance attestation for %s: %w", uri, err)
	}
	record.AttestationVerified = true
	record.AttestationURI = uri + "#attestation:slsaprovenance1"

	sbomOutput, err := runCosign(ref, verifier, "verify-attestation", "--type", "cyclonedx")
	if err != nil {
		return record, fmt.Errorf("verify CycloneDX SBOM attestation for %s: %w", uri, err)
	}
	record.SBOMVerified = true
	record.SBOMDigest = sha256Hex(sbomOutput)
	if record.SignatureDigest == "" {
		record.SignatureDigest = sha256Hex(provenanceOutput)
	}
	record.VerifiedAt = time.Now().UTC().Format(time.RFC3339)

	return record, nil
}

func runCosign(ref string, verifier verifierConfig, verb string, extraArgs ...string) ([]byte, error) {
	args := []string{verb}
	if verifier.LocalKeyPath != "" {
		args = append(args, "--key", verifier.LocalKeyPath)
	} else {
		args = append(args,
			"--certificate-identity", verifier.CertificateIdentity,
			"--certificate-oidc-issuer", verifier.CertificateIssuer,
		)
	}
	args = append(args, extraArgs...)
	args = append(args, ref)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, cosignBinaryPath, args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("cosign %s timed out: %w", verb, ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func extractSignatureDigest(output []byte) string {
	type verifyPayload struct {
		Critical struct {
			Image struct {
				DockerManifestDigest string `json:"docker-manifest-digest"`
			} `json:"image"`
		} `json:"critical"`
	}

	var payloads []verifyPayload
	if err := json.Unmarshal(output, &payloads); err == nil && len(payloads) > 0 {
		if digest := payloads[0].Critical.Image.DockerManifestDigest; digest != "" {
			return digest
		}
	}

	return sha256Hex(output)
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func writeRuntimeVerificationRecord(record provenance.RuntimeVerificationRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal runtime verification record: %w", err)
	}
	return os.WriteFile(terminationLogPath, append(data, '\n'), 0o600)
}

func writeCacheRuntimeVerificationRecord(destPath string, record provenance.RuntimeVerificationRecord) error {
	stateDir := filepath.Join(destPath, verificationStateDir)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, verificationStateFile), append(data, '\n'), 0o600)
}

func loadCachedRuntimeVerificationRecord(destPath string) (*provenance.RuntimeVerificationRecord, error) {
	data, err := os.ReadFile(filepath.Join(destPath, verificationStateDir, verificationStateFile))
	if err != nil {
		return nil, err
	}
	return provenance.ParseRuntimeVerificationRecord(string(data))
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
