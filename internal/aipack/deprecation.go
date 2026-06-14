package aipack

// DeprecationPhase is the lifecycle phase of an artifact per AIPACK-SPEC v0.1.1 §16.
type DeprecationPhase string

const (
	DeprecationPhaseActive     DeprecationPhase = "active"
	DeprecationPhaseDeprecated DeprecationPhase = "deprecated"
	DeprecationPhaseSunset     DeprecationPhase = "sunset"
	DeprecationPhaseRevoked    DeprecationPhase = "revoked"
)

// DeprecationNotice is the §16 deprecation declaration for an artifact.
// Backed by attestation urn:aipack:deprecation:v1.
type DeprecationNotice struct {
	// ArtifactRef is the OCI digest reference of the deprecated artifact.
	ArtifactRef string `json:"artifactRef"`

	// Phase is the deprecation lifecycle phase.
	Phase DeprecationPhase `json:"phase"`

	// SunsetDate is the ISO 8601 date after which the artifact is blocked (sunset phase).
	// Required when Phase = "sunset".
	SunsetDate string `json:"sunsetDate,omitempty"`

	// SuccessorRef is the OCI digest reference to the recommended successor artifact.
	SuccessorRef string `json:"successorRef,omitempty"`

	// Reason is a human-readable deprecation reason.
	Reason string `json:"reason,omitempty"`
}

// IssueDeprecationNotice issues a deprecation notice for the given artifact.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §16 — emit urn:aipack:deprecation:v1
func IssueDeprecationNotice(_ *DeprecationNotice) error {
	return newErr(ErrDeprecationBlocked, "deprecation notice issuance not yet implemented", "")
}

// RevokeDeprecation revokes a deprecation notice, returning the artifact to active phase.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §16 — emit urn:aipack:deprecation-revocation:v1
func RevokeDeprecation(_ string) error {
	return newErr(ErrDeprecationBlocked, "deprecation revocation not yet implemented", "")
}

// IsSunsetExpired reports whether a sunset date has passed relative to the current time.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §16 — parse ISO 8601 date + compare to now
func IsSunsetExpired(_ string) bool {
	return false
}
