/*
Copyright 2026 CKodex Authors.
*/

package governance

import (
	"bufio"
	"bytes"
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

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
)

// cosignBinaryPath is configurable for tests.
var cosignBinaryPath = func() string {
	if path := os.Getenv("CKODEX_COSIGN_BINARY_PATH"); path != "" {
		return path
	}
	return "/cosign"
}()

// HasRequiredAIPackAttestations reports whether the attestation block contains
// entries for all predicates required by the artifact's kind. It does not
// perform cryptographic verification — use VerifyAIPackAttestation for that.
func HasRequiredAIPackAttestations(kind servingv1alpha2.ArtifactKind, attestation *servingv1alpha2.AIPackAttestation) bool {
	if len(aipack.RequiredPredicates(kind)) == 0 {
		return false
	}
	if attestation == nil {
		return false
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
// 1. All required predicates for kind are present (AIPACK-ATTEST-001/002)
// 2. Predicate entries have non-empty URIs (AIPACK-ATTEST-003)
// 3. signature and per-predicate attestation verification succeeds via cosign.
func VerifyAIPackAttestation(ctx context.Context, kind servingv1alpha2.ArtifactKind, ref string, attestation *servingv1alpha2.AIPackAttestation) (*AIPackVerificationResult, error) {
	if ref == "" {
		return missingArtifactReferenceResult(), nil
	}
	if len(aipack.RequiredPredicates(kind)) == 0 {
		return unsupportedArtifactKindResult(kind, ref), nil
	}
	if attestation == nil {
		return verifyMissingAttestation(kind, ref), nil
	}

	present := predicateURIs(attestation)
	emptyURI := emptyPredicateEntries(attestation)
	if len(emptyURI) > 0 {
		return emptyPredicateResult(ref, emptyURI), nil
	}

	missing := aipack.MissingPredicates(kind, present)
	if len(missing) > 0 {
		return missingPredicatesResult(ref, missing), nil
	}

	requiredPredicates := aipack.RequiredPredicates(kind)
	entries := make(map[string]servingv1alpha2.PredicateEntry, len(requiredPredicates))
	invalidEntries := make([]string, 0)
	for _, requiredPredicate := range requiredPredicates {
		entry, err := requiredPredicateEntry(attestation, requiredPredicate)
		if err != nil {
			invalidEntries = append(invalidEntries, requiredPredicate)
			continue
		}
		if entry.RekorLogID != "" {
			invalidEntries = append(invalidEntries, requiredPredicate)
			continue
		}
		if entry.Digest != "" && (!strings.HasPrefix(entry.Digest, "sha256:") || len(entry.Digest) != len("sha256:")+sha256.Size*2) {
			invalidEntries = append(invalidEntries, requiredPredicate)
			continue
		}
		entries[requiredPredicate] = entry
	}
	if len(invalidEntries) > 0 {
		return &AIPackVerificationResult{
			Verified:         false,
			VerifiedAt:       time.Now().UTC(),
			FailedPredicates: invalidEntries,
			Message:          fmt.Sprintf("artifact %q has unbound or invalid predicate metadata: %v", ref, invalidEntries),
		}, nil
	}

	trimmedRef, err := trimArtifactRef(ref)
	if err != nil {
		return nil, err
	}

	cfg, err := resolveCosignConfig(attestation)
	if err != nil {
		return &AIPackVerificationResult{
			Verified:         false,
			VerifiedAt:       time.Now().UTC(),
			FailedPredicates: []string{"cosign-signature"},
			Message:          fmt.Sprintf("artifact %q verification config is invalid: %v", ref, err),
		}, nil
	}

	failed := []string{}
	verifyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := verifyArtifactSignature(verifyCtx, trimmedRef, cfg); err != nil {
		failed = append(failed, "cosign-signature")
	}

	for _, requiredPredicate := range aipack.RequiredPredicates(kind) {
		entry := entries[requiredPredicate]
		if err := verifyArtifactAttestation(verifyCtx, trimmedRef, cfg, entry); err != nil {
			failed = append(failed, requiredPredicate)
		}
	}

	if len(failed) > 0 {
		return &AIPackVerificationResult{
			Verified:         false,
			VerifiedAt:       time.Now().UTC(),
			FailedPredicates: failed,
			Message:          fmt.Sprintf("artifact %q failed predicate verification: %v", ref, failed),
		}, nil
	}

	return &AIPackVerificationResult{
		Verified:   true,
		VerifiedAt: time.Now().UTC(),
		Message:    fmt.Sprintf("artifact %q verified for kind %q via cosign", ref, kind),
	}, nil
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
	return &AIPackVerificationResult{
		Verified:         false,
		VerifiedAt:       time.Now().UTC(),
		FailedPredicates: required,
		Message:          fmt.Sprintf("artifact %q (kind %s) has no attestation block but requires %d predicate(s)", ref, kind, len(required)),
	}
}

func unsupportedArtifactKindResult(kind servingv1alpha2.ArtifactKind, ref string) *AIPackVerificationResult {
	return &AIPackVerificationResult{
		Verified:         false,
		VerifiedAt:       time.Now().UTC(),
		FailedPredicates: []string{"artifact-kind"},
		Message:          fmt.Sprintf("artifact %q has unsupported kind %q", ref, kind),
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

type cosignVerifyConfig struct {
	keyPath  string
	identity string
	issuer   string
	rekorURL string
}

func resolveCosignConfig(attestation *servingv1alpha2.AIPackAttestation) (*cosignVerifyConfig, error) {
	cfg := &cosignVerifyConfig{}
	cfg.rekorURL = strings.TrimSpace(os.Getenv("CKODEX_COSIGN_REKOR_URL"))

	trustedKeyRef := strings.TrimSpace(os.Getenv("CKODEX_COSIGN_TRUSTED_KEY_REF"))
	trustedKeyPath := strings.TrimSpace(os.Getenv("CKODEX_COSIGN_TRUSTED_KEY_PATH"))
	if attestation != nil && strings.TrimSpace(attestation.CosignKeyRef) != "" {
		if trustedKeyRef == "" || strings.TrimSpace(attestation.CosignKeyRef) != trustedKeyRef {
			return nil, fmt.Errorf("AIPack cosignKeyRef is not in the operator trusted-key configuration")
		}
		if trustedKeyPath == "" {
			return nil, fmt.Errorf("operator trusted-key path is not configured")
		}
		cfg.keyPath = trustedKeyPath
	} else if trustedKeyPath != "" {
		cfg.keyPath = trustedKeyPath
	}

	if cfg.keyPath == "" {
		cfg.identity = strings.TrimSpace(os.Getenv("CKODEX_COSIGN_CERT_IDENTITY"))
		cfg.issuer = strings.TrimSpace(os.Getenv("CKODEX_COSIGN_CERT_OIDC_ISSUER"))
	}
	if cfg.keyPath == "" && (cfg.identity == "" || cfg.issuer == "") {
		return nil, fmt.Errorf("missing cosign verifier config: set the operator trusted key path or both CKODEX_COSIGN_CERT_IDENTITY and CKODEX_COSIGN_CERT_OIDC_ISSUER")
	}
	return cfg, nil
}

func verifyArtifactSignature(ctx context.Context, ref string, cfg *cosignVerifyConfig) error {
	args := []string{"verify"}
	args = append(args, cfg.cosignAuthArgs()...)
	args = append(args, ref)
	_, err := runCosignCommand(ctx, args...)
	return err
}

func requiredPredicateEntry(attestation *servingv1alpha2.AIPackAttestation, predicateURI string) (servingv1alpha2.PredicateEntry, error) {
	var found *servingv1alpha2.PredicateEntry
	for i := range attestation.Predicates {
		entry := &attestation.Predicates[i]
		if entry.PredicateURI != predicateURI {
			continue
		}
		if found != nil {
			return servingv1alpha2.PredicateEntry{}, fmt.Errorf("duplicate entry for required predicate %q", predicateURI)
		}
		found = entry
	}
	if found == nil {
		return servingv1alpha2.PredicateEntry{}, fmt.Errorf("required predicate %q is absent", predicateURI)
	}
	return *found, nil
}

type verifiedInTotoStatement struct {
	PredicateType string          `json:"predicateType"`
	Predicate     json.RawMessage `json:"predicate"`
}

func verifyArtifactAttestation(ctx context.Context, ref string, cfg *cosignVerifyConfig, entry servingv1alpha2.PredicateEntry) error {
	args := []string{"verify-attestation"}
	args = append(args, cfg.cosignAuthArgs()...)
	args = append(args, "--type", entry.PredicateURI, "--output", "text")
	args = append(args, ref)
	output, err := runCosignCommand(ctx, args...)
	if err != nil {
		return err
	}
	return verifyPredicatePayload(output, entry)
}

func verifyPredicatePayload(output []byte, entry servingv1alpha2.PredicateEntry) error {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var matched bool
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var statement verifiedInTotoStatement
		if err := json.Unmarshal(line, &statement); err != nil {
			continue
		}
		if statement.PredicateType != entry.PredicateURI || len(statement.Predicate) == 0 || bytes.Equal(statement.Predicate, []byte("null")) {
			continue
		}
		matched = true
		if entry.Digest == "" {
			return nil
		}
		if !strings.HasPrefix(entry.Digest, "sha256:") || len(entry.Digest) != len("sha256:")+sha256.Size*2 {
			return fmt.Errorf("predicate %q has an invalid SHA-256 digest", entry.PredicateURI)
		}
		digest := sha256.Sum256(statement.Predicate)
		if entry.Digest == "sha256:"+hex.EncodeToString(digest[:]) {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read verified predicate output: %w", err)
	}
	if !matched {
		return fmt.Errorf("cosign returned no verified in-toto statement for predicate %q", entry.PredicateURI)
	}
	return fmt.Errorf("verified predicate %q does not match the declared digest", entry.PredicateURI)
}

func (c *cosignVerifyConfig) cosignAuthArgs() []string {
	args := make([]string, 0, 6)
	if c.keyPath != "" {
		args = append(args, "--key", c.keyPath)
	} else {
		if c.identity != "" {
			args = append(args, "--certificate-identity", c.identity)
		}
		if c.issuer != "" {
			args = append(args, "--certificate-oidc-issuer", c.issuer)
		}
	}
	if c.rekorURL != "" {
		args = append(args, "--rekor-url", c.rekorURL)
	}
	return args
}

func runCosignCommand(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("cosign command requires arguments")
	}
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if !filepath.IsAbs(cosignBinaryPath) {
		return nil, fmt.Errorf("cosign binary path must be absolute: %q", cosignBinaryPath)
	}

	// #nosec G204 -- cosignBinaryPath is validated as an absolute path and intended command execution target is controlled.
	cmd := exec.CommandContext(cmdCtx, cosignBinaryPath, args...)

	output, err := cmd.CombinedOutput()
	if cmdCtx.Err() != nil {
		return nil, fmt.Errorf("cosign command timed out: %w", cmdCtx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func trimArtifactRef(ref string) (string, error) {
	if strings.HasPrefix(ref, "oci://") {
		return strings.TrimPrefix(ref, "oci://"), nil
	}
	if strings.HasPrefix(ref, "ocis://") {
		return strings.TrimPrefix(ref, "ocis://"), nil
	}
	if strings.Contains(ref, "://") {
		parts := strings.SplitN(ref, "://", 2)
		return "", fmt.Errorf("unsupported artifact scheme %q", parts[0])
	}
	return ref, nil
}
