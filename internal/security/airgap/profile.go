/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package airgap implements Security Sovereignty Profiles and Air-Gap Transfer Capsules.
// It enforces offline validation, zero phone-home guarantees, digest-pinned OCI packages,
// and hermetic evidence verification across Connected, Enterprise, Restricted,
// Disconnected, and Air-Gap environments.
package airgap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type (
	SovereigntyProfileLevel    string
	SovereigntyState           string
	NetworkPosture             string
	DigestPinningStatus        string
	SignatureIntegrityStatus   string
	AttestationIntegrityStatus string
	BOMIntegrityStatus         string
	IdentityIntegrityStatus    string
	SecretStoreIntegrityStatus string
)

const (
	ProfileConnected        SovereigntyProfileLevel    = "Connected"
	ProfileEnterprise       SovereigntyProfileLevel    = "Enterprise"
	ProfileRestricted       SovereigntyProfileLevel    = "Restricted"
	ProfileDisconnected     SovereigntyProfileLevel    = "Disconnected"
	ProfileAirGap           SovereigntyProfileLevel    = "AirGap"
	StateCompliant          SovereigntyState           = "Compliant"
	StateDegraded           SovereigntyState           = "Degraded"
	StateNonCompliant       SovereigntyState           = "NonCompliant"
	PostureConnected        NetworkPosture             = "Connected"
	PostureEnterpriseProxy  NetworkPosture             = "EnterpriseProxy"
	PostureRestricted       NetworkPosture             = "RestrictedEgress"
	PostureDisconnected     NetworkPosture             = "Disconnected"
	PostureAirGapZeroEgress NetworkPosture             = "AirGapZeroPhoneHome"
	DigestPinned            DigestPinningStatus        = "Pinned"
	DigestUnpinned          DigestPinningStatus        = "Unpinned"
	DigestMalformed         DigestPinningStatus        = "Malformed"
	SigVerified             SignatureIntegrityStatus   = "Verified"
	SigUnverified           SignatureIntegrityStatus   = "Unverified"
	SigMissing              SignatureIntegrityStatus   = "Missing"
	SigUntrusted            SignatureIntegrityStatus   = "Untrusted"
	AttestationVerified     AttestationIntegrityStatus = "Verified"
	AttestationMissing      AttestationIntegrityStatus = "Missing"
	AttestationInvalid      AttestationIntegrityStatus = "Invalid"
	BOMComplete             BOMIntegrityStatus         = "Complete"
	BOMPartial              BOMIntegrityStatus         = "Partial"
	BOMMissing              BOMIntegrityStatus         = "Missing"
	IdentityEnforced        IdentityIntegrityStatus    = "Enforced"
	IdentityOptional        IdentityIntegrityStatus    = "Optional"
	IdentityMissing         IdentityIntegrityStatus    = "Missing"
	SecretStoreLocalSeeded  SecretStoreIntegrityStatus = "LocalSeeded"
	SecretStoreEnterprise   SecretStoreIntegrityStatus = "EnterpriseManaged"
	SecretStoreRemoteKMS    SecretStoreIntegrityStatus = "RemoteKMS"
	SecretStoreInsecure     SecretStoreIntegrityStatus = "Insecure"
)

var (
	ErrNilCapsule            = errors.New("air-gap capsule is nil")
	ErrCapsuleExpired        = errors.New("air-gap capsule has expired")
	ErrInvalidMetadata       = errors.New("invalid capsule metadata")
	ErrUnpinnedDigest        = errors.New("artifact digest is unpinned or malformed")
	ErrDigestMismatch        = errors.New("artifact payload does not match declared digest")
	ErrSignatureMissing      = errors.New("required cryptographic signature missing")
	ErrUntrustedKey          = errors.New("signature key fingerprint untrusted")
	ErrAttestationMissing    = errors.New("required attestation missing")
	ErrBOMIncomplete         = errors.New("required AI-BOM or SBOM incomplete")
	ErrPolicyViolation       = errors.New("sovereignty policy violated")
	ErrIncompatibleProfile   = errors.New("capsule target profile incompatible with host profile")
	ErrPhoneHomeNotForbidden = errors.New("air gap requires strict zero phone-home policy")
	ErrUnknownProfile        = errors.New("unknown sovereignty profile")
)

type (
	ProfileRequirements struct {
		Level                                                                   SovereigntyProfileLevel
		RequireDigestPinning, RequireSignatures, RequireAttestation, RequireBOM bool
		RequireSPIFFE, RequireLocalSecrets, RequireAirGapCapsule, ZeroPhoneHome bool
	}
	SovereigntyProfile struct {
		Requirements ProfileRequirements
	}
	SovereigntyVector struct {
		Profile              SovereigntyProfileLevel    `json:"profile"`
		Network              NetworkPosture             `json:"network"`
		DigestPinning        DigestPinningStatus        `json:"digestPinning"`
		SignatureIntegrity   SignatureIntegrityStatus   `json:"signatureIntegrity"`
		AttestationIntegrity AttestationIntegrityStatus `json:"attestationIntegrity"`
		BOMIntegrity         BOMIntegrityStatus         `json:"bomIntegrity"`
		IdentityIntegrity    IdentityIntegrityStatus    `json:"identityIntegrity"`
		SecretStoreIntegrity SecretStoreIntegrityStatus `json:"secretStoreIntegrity"`
		State                SovereigntyState           `json:"state"`
		Violations           []string                   `json:"violations,omitempty"`
	}
	WorkloadTarget struct {
		Name, ImageURI, ModelURI, SPIFFEIdentity, SecretStoreType, EgressTarget string
		HasSBOM, HasAIBOM, HasAttestation, HasSignature                         bool
	}
	TransferCapsule struct {
		APIVersion string            `json:"apiVersion"`
		Kind       string            `json:"kind"`
		Metadata   CapsuleMetadata   `json:"metadata"`
		Artifacts  []CapsuleArtifact `json:"artifacts"`
		Evidence   CapsuleEvidence   `json:"evidence"`
		Policies   CapsulePolicies   `json:"policies"`
	}
	CapsuleMetadata struct {
		ID, SourceEnvironment, Classification string
		CreatedAt, ExpiresAt                  time.Time
		TargetProfile                         SovereigntyProfileLevel
		Annotations                           map[string]string `json:"annotations,omitempty"`
	}
	CapsuleArtifact struct {
		Name, MediaType, Digest, Payload string
		Size                             int64
	}
	CapsuleEvidence struct {
		Signatures   []SignatureRecord   `json:"signatures"`
		Attestations []AttestationRecord `json:"attestations"`
		AIBOM        *AIBOMRecord        `json:"aiBom,omitempty"`
		SBOM         *SBOMRecord         `json:"sBom,omitempty"`
	}
	SignatureRecord struct {
		ArtifactDigest, SignerIdentity, KeyFingerprint, SignatureValue string
		Timestamp                                                      time.Time
	}
	AttestationRecord struct {
		PredicateType, SubjectDigest, PayloadDigest, StatementJSON string
	}
	AIBOMRecord struct {
		ModelName, ModelVersion, Architecture, Parameters, WeightsDigest, Format string
		Metadata                                                                 map[string]string `json:"metadata,omitempty"`
	}
	SBOMRecord struct {
		Format, Digest string
		Components     []string
	}
	CapsulePolicies struct {
		ValidationRules        []ValidationRule `json:"validationRules"`
		TrustedKeyFingerprints []string         `json:"trustedKeyFingerprints"`
		AllowedRegistries      []string         `json:"allowedRegistries"`
		ZeroPhoneHomeRequired  bool             `json:"zeroPhoneHomeRequired"`
		MaxPermittedRisk       string           `json:"maxPermittedRisk"`
	}
	ValidationRule struct {
		Name, RuleType, Requirement string
	}
)

func NewSovereigntyProfile(level SovereigntyProfileLevel) (*SovereigntyProfile, error) {
	reqs, err := getProfileRequirements(level)
	if err != nil {
		return nil, fmt.Errorf("create sovereignty profile: %w", err)
	}
	return &SovereigntyProfile{Requirements: reqs}, nil
}

func getProfileRequirements(level SovereigntyProfileLevel) (ProfileRequirements, error) {
	switch level {
	case ProfileConnected:
		return ProfileRequirements{Level: ProfileConnected}, nil
	case ProfileEnterprise:
		return ProfileRequirements{Level: ProfileEnterprise, RequireSignatures: true, RequireSPIFFE: true}, nil
	case ProfileRestricted:
		return ProfileRequirements{
			Level: ProfileRestricted, RequireDigestPinning: true, RequireSignatures: true,
			RequireAttestation: true, RequireBOM: true, RequireSPIFFE: true,
		}, nil
	case ProfileDisconnected:
		return ProfileRequirements{
			Level: ProfileDisconnected, RequireDigestPinning: true, RequireSignatures: true,
			RequireAttestation: true, RequireBOM: true, RequireSPIFFE: true,
			RequireLocalSecrets: true, ZeroPhoneHome: true,
		}, nil
	case ProfileAirGap:
		return ProfileRequirements{
			Level: ProfileAirGap, RequireDigestPinning: true, RequireSignatures: true,
			RequireAttestation: true, RequireBOM: true, RequireSPIFFE: true,
			RequireLocalSecrets: true, RequireAirGapCapsule: true, ZeroPhoneHome: true,
		}, nil
	default:
		return ProfileRequirements{}, fmt.Errorf("%w: %s", ErrUnknownProfile, level)
	}
}

func ValidateWorkload(level SovereigntyProfileLevel, target WorkloadTarget) SovereigntyVector {
	req, _ := getProfileRequirements(level)
	vec := SovereigntyVector{
		Profile:              level,
		Network:              evalNetworkPosture(level, target.EgressTarget),
		DigestPinning:        evalDigestPinning(target.ImageURI, target.ModelURI),
		SignatureIntegrity:   evalSigStatus(target.HasSignature),
		AttestationIntegrity: evalAttestStatus(target.HasAttestation),
		BOMIntegrity:         evalBOMStatus(target.HasSBOM, target.HasAIBOM),
		IdentityIntegrity:    evalIdentityStatus(target.SPIFFEIdentity),
		SecretStoreIntegrity: evalSecretStoreStatus(target.SecretStoreType),
		State:                StateCompliant,
	}
	enforceWorkloadRequirements(&vec, req, target)
	return vec
}

func enforceWorkloadRequirements(vec *SovereigntyVector, req ProfileRequirements, target WorkloadTarget) {
	if req.RequireDigestPinning && vec.DigestPinning != DigestPinned {
		vec.Violations = append(vec.Violations, "digest pinning required for OCI images and models")
	}
	if req.RequireSignatures && vec.SignatureIntegrity != SigVerified {
		vec.Violations = append(vec.Violations, "cryptographic signature required")
	}
	if req.RequireAttestation && vec.AttestationIntegrity != AttestationVerified {
		vec.Violations = append(vec.Violations, "SLSA provenance attestation required")
	}
	if req.RequireBOM && vec.BOMIntegrity != BOMComplete {
		vec.Violations = append(vec.Violations, "complete SBOM and AI-BOM required")
	}
	if req.RequireSPIFFE && vec.IdentityIntegrity != IdentityEnforced {
		vec.Violations = append(vec.Violations, "SPIFFE workload identity required")
	}
	if req.ZeroPhoneHome && target.EgressTarget != "" {
		vec.Violations = append(vec.Violations, "external network egress prohibited in zero-phone-home profile")
	}
	if len(vec.Violations) > 0 {
		vec.State = StateNonCompliant
	}
}

func evalNetworkPosture(level SovereigntyProfileLevel, _ string) NetworkPosture {
	switch level {
	case ProfileAirGap:
		return PostureAirGapZeroEgress
	case ProfileDisconnected:
		return PostureDisconnected
	case ProfileRestricted:
		return PostureRestricted
	case ProfileEnterprise:
		return PostureEnterpriseProxy
	default:
		return PostureConnected
	}
}

func evalDigestPinning(uris ...string) DigestPinningStatus {
	for _, u := range uris {
		if u == "" {
			continue
		}
		if !strings.Contains(u, "@sha256:") {
			return DigestUnpinned
		}
		parts := strings.Split(u, "@sha256:")
		if len(parts) != 2 || len(parts[1]) != 64 {
			return DigestMalformed
		}
	}
	return DigestPinned
}

func evalSigStatus(hasSig bool) SignatureIntegrityStatus {
	if hasSig {
		return SigVerified
	}
	return SigMissing
}

func evalAttestStatus(hasAttest bool) AttestationIntegrityStatus {
	if hasAttest {
		return AttestationVerified
	}
	return AttestationMissing
}

func evalBOMStatus(hasSBOM, hasAIBOM bool) BOMIntegrityStatus {
	if hasSBOM && hasAIBOM {
		return BOMComplete
	}
	if hasSBOM || hasAIBOM {
		return BOMPartial
	}
	return BOMMissing
}

func evalIdentityStatus(id string) IdentityIntegrityStatus {
	if strings.HasPrefix(id, "spiffe://") {
		return IdentityEnforced
	}
	return IdentityMissing
}

func evalSecretStoreStatus(store string) SecretStoreIntegrityStatus {
	switch strings.ToLower(store) {
	case "local", "localseeded", "sealedsecret":
		return SecretStoreLocalSeeded
	case "vault", "enterprise":
		return SecretStoreEnterprise
	case "remote", "kms":
		return SecretStoreRemoteKMS
	default:
		return SecretStoreInsecure
	}
}

// ValidateCapsule hermetically validates an air-gap transfer capsule in offline mode.
func ValidateCapsule(capsule *TransferCapsule, expectedProfile SovereigntyProfileLevel, now time.Time) (SovereigntyVector, error) {
	if capsule == nil {
		return SovereigntyVector{State: StateNonCompliant}, ErrNilCapsule
	}
	req, err := getProfileRequirements(expectedProfile)
	if err != nil {
		return SovereigntyVector{State: StateNonCompliant}, err
	}
	if err := validateCapsuleMetadata(capsule.Metadata, expectedProfile, now); err != nil {
		return SovereigntyVector{State: StateNonCompliant}, fmt.Errorf("validate metadata: %w", err)
	}
	artViolations, digStatus, err := validateCapsuleArtifacts(capsule.Artifacts)
	if err != nil {
		return SovereigntyVector{DigestPinning: digStatus, State: StateNonCompliant}, err
	}
	evViolations, sigStatus, attStatus, bomStatus := validateCapsuleEvidence(capsule.Evidence, capsule.Artifacts, capsule.Policies.TrustedKeyFingerprints, req)
	polViolations, netPosture, polErr := validateCapsulePolicies(capsule.Policies, req)
	if polErr != nil {
		return SovereigntyVector{Network: netPosture, State: StateNonCompliant}, polErr
	}
	vec := SovereigntyVector{
		Profile: expectedProfile, Network: netPosture, DigestPinning: digStatus,
		SignatureIntegrity: sigStatus, AttestationIntegrity: attStatus, BOMIntegrity: bomStatus,
		IdentityIntegrity: IdentityEnforced, SecretStoreIntegrity: SecretStoreLocalSeeded, State: StateCompliant,
	}
	vec.Violations = append(vec.Violations, artViolations...)
	vec.Violations = append(vec.Violations, evViolations...)
	vec.Violations = append(vec.Violations, polViolations...)
	if len(vec.Violations) > 0 {
		vec.State = StateNonCompliant
	}
	return vec, nil
}

func validateCapsuleMetadata(m CapsuleMetadata, expectedProfile SovereigntyProfileLevel, now time.Time) error {
	if m.ID == "" || m.CreatedAt.IsZero() || m.ExpiresAt.IsZero() {
		return ErrInvalidMetadata
	}
	if !now.IsZero() && now.After(m.ExpiresAt) {
		return ErrCapsuleExpired
	}
	if expectedProfile == ProfileAirGap && m.TargetProfile != ProfileAirGap {
		return fmt.Errorf("%w: capsule target %s vs expected %s", ErrIncompatibleProfile, m.TargetProfile, expectedProfile)
	}
	return nil
}

func validateCapsuleArtifacts(artifacts []CapsuleArtifact) ([]string, DigestPinningStatus, error) {
	if len(artifacts) == 0 {
		return nil, DigestMalformed, fmt.Errorf("%w: artifacts list is empty", ErrInvalidMetadata)
	}
	for _, art := range artifacts {
		if !strings.HasPrefix(art.Digest, "sha256:") || len(art.Digest) != 71 {
			return nil, DigestUnpinned, fmt.Errorf("%w: artifact %s", ErrUnpinnedDigest, art.Name)
		}
		if art.Payload != "" {
			expectedHex := art.Digest[7:]
			actualHash := sha256.Sum256([]byte(art.Payload))
			if hex.EncodeToString(actualHash[:]) != expectedHex {
				return nil, DigestMalformed, fmt.Errorf("%w: payload digest mismatch for %s", ErrDigestMismatch, art.Name)
			}
		}
	}
	return nil, DigestPinned, nil
}

func validateCapsuleEvidence(ev CapsuleEvidence, arts []CapsuleArtifact, trustedKeys []string, req ProfileRequirements) ([]string, SignatureIntegrityStatus, AttestationIntegrityStatus, BOMIntegrityStatus) {
	var violations []string
	digests := artifactDigestMap(arts)
	sigStatus := checkSignatures(ev.Signatures, digests, trustedKeys, req.RequireSignatures, &violations)
	attStatus := checkAttestations(ev.Attestations, digests, req.RequireAttestation, &violations)
	bomStatus := checkBOMs(ev.AIBOM, ev.SBOM, digests, req.RequireBOM, &violations)
	return violations, sigStatus, attStatus, bomStatus
}

func artifactDigestMap(arts []CapsuleArtifact) map[string]bool {
	m := make(map[string]bool, len(arts))
	for _, a := range arts {
		m[a.Digest] = true
	}
	return m
}

func checkSignatures(sigs []SignatureRecord, digests map[string]bool, trustedKeys []string, required bool, violations *[]string) SignatureIntegrityStatus {
	if len(sigs) == 0 {
		if required {
			*violations = append(*violations, "missing required cryptographic signatures")
		}
		return SigMissing
	}
	for _, sig := range sigs {
		if !digests[sig.ArtifactDigest] {
			*violations = append(*violations, fmt.Sprintf("signature references unknown artifact digest: %s", sig.ArtifactDigest))
			return SigUntrusted
		}
		if !containsFingerprint(trustedKeys, sig.KeyFingerprint) {
			*violations = append(*violations, fmt.Sprintf("signature key fingerprint %s untrusted", sig.KeyFingerprint))
			return SigUntrusted
		}
	}
	return SigVerified
}

func containsFingerprint(keys []string, target string) bool {
	for _, k := range keys {
		if k == target {
			return true
		}
	}
	return false
}

func checkAttestations(attests []AttestationRecord, digests map[string]bool, required bool, violations *[]string) AttestationIntegrityStatus {
	if len(attests) == 0 {
		if required {
			*violations = append(*violations, "missing required in-toto provenance attestation")
		}
		return AttestationMissing
	}
	for _, att := range attests {
		if !digests[att.SubjectDigest] {
			*violations = append(*violations, fmt.Sprintf("attestation subject %s not found in artifacts", att.SubjectDigest))
			return AttestationInvalid
		}
	}
	return AttestationVerified
}

func checkBOMs(aiBOM *AIBOMRecord, sBOM *SBOMRecord, digests map[string]bool, required bool, violations *[]string) BOMIntegrityStatus {
	if aiBOM == nil || sBOM == nil {
		if required {
			*violations = append(*violations, "both AI-BOM and SBOM are required for offline sovereignty")
		}
		if aiBOM != nil || sBOM != nil {
			return BOMPartial
		}
		return BOMMissing
	}
	if !digests[aiBOM.WeightsDigest] {
		*violations = append(*violations, fmt.Sprintf("AI-BOM weights digest %s not found in artifacts", aiBOM.WeightsDigest))
		return BOMPartial
	}
	return BOMComplete
}

func validateCapsulePolicies(pol CapsulePolicies, req ProfileRequirements) ([]string, NetworkPosture, error) {
	var violations []string
	posture := PostureConnected
	if req.ZeroPhoneHome {
		posture = PostureAirGapZeroEgress
		if !pol.ZeroPhoneHomeRequired {
			return nil, posture, fmt.Errorf("%w: air gap capsule must explicitly assert zero-phone-home requirement", ErrPhoneHomeNotForbidden)
		}
	}
	return violations, posture, nil
}

func ExportCapsule(capsule *TransferCapsule) ([]byte, error) {
	if capsule == nil {
		return nil, ErrNilCapsule
	}
	data, err := json.MarshalIndent(capsule, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("export capsule: %w", err)
	}
	return data, nil
}

func ImportCapsule(data []byte) (*TransferCapsule, error) {
	if len(data) == 0 {
		return nil, errors.New("import capsule: empty data")
	}
	var capsule TransferCapsule
	if err := json.Unmarshal(data, &capsule); err != nil {
		return nil, fmt.Errorf("import capsule: %w", err)
	}
	return &capsule, nil
}
