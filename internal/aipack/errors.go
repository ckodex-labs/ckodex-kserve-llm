// Package aipack implements the AIPACK-SPEC v0.1.1 runtime for artifact packaging,
// composition validation, attestation verification, and operational conformance.
package aipack

import "fmt"

// ErrorCode is a typed error code from AIPACK-SPEC v0.1.1 Appendix A.
type ErrorCode string

// Error codes from AIPACK-SPEC v0.1.1 Appendix A (39 total).
const (
	// Kind/family validation errors
	ErrKindUnknown      ErrorCode = "AIPACK-KIND-000"
	ErrFamilyMismatch   ErrorCode = "AIPACK-KIND-001"

	// Composition errors
	ErrTagOnlyRef       ErrorCode = "AIPACK-COMP-001"
	ErrCyclicDAG        ErrorCode = "AIPACK-COMP-002"
	ErrDAGDepthExceeded ErrorCode = "AIPACK-COMP-003"
	ErrSlotTypeMismatch ErrorCode = "AIPACK-COMP-004"

	// Attestation errors
	ErrMissingPredicate   ErrorCode = "AIPACK-ATTEST-001"
	ErrInvalidSignature   ErrorCode = "AIPACK-ATTEST-002"
	ErrExpiredPredicate   ErrorCode = "AIPACK-ATTEST-003"
	ErrUnresolvableRef    ErrorCode = "AIPACK-ATTEST-004"

	// Compatibility errors
	ErrLoRABaseRefMissing       ErrorCode = "AIPACK-COMPAT-001"
	ErrRetrievalEmbedMismatch   ErrorCode = "AIPACK-COMPAT-002"

	// Runtime errors
	ErrMediaTypeMismatch   ErrorCode = "AIPACK-RUNTIME-001"
	ErrDigestVerifyFailed  ErrorCode = "AIPACK-RUNTIME-002"

	// Lineage errors (§11)
	ErrLineageEnvelopeMissing ErrorCode = "AIPACK-LIN-001"
	ErrLineageHashMismatch    ErrorCode = "AIPACK-LIN-002"

	// Blast radius errors (§12)
	ErrBlastRadiusExceeded    ErrorCode = "AIPACK-BLAST-001"
	ErrDependencyIndexStale   ErrorCode = "AIPACK-BLAST-002"

	// Risk valence errors (§13)
	ErrRVWeightsSumInvalid    ErrorCode = "AIPACK-RV-001"
	ErrRVRedBandBlocked       ErrorCode = "AIPACK-RV-002"

	// Deprecation errors (§16)
	ErrDeprecationBlocked     ErrorCode = "AIPACK-DEP-001"
	ErrSunsetExpired          ErrorCode = "AIPACK-DEP-002"

	// TEA errors (§15)
	ErrTEAEndpointUnreachable ErrorCode = "AIPACK-TEA-001"
	ErrTEAQueryInvalid        ErrorCode = "AIPACK-TEA-002"

	// Outlier errors (§14)
	ErrOutlierUnacknowledged  ErrorCode = "AIPACK-OUTLIER-001"

	// Air-gap errors (§17)
	ErrAirGapBundleExpired    ErrorCode = "AIPACK-AIRGAP-001"
	ErrAirGapTrustRootMissing ErrorCode = "AIPACK-AIRGAP-002"
	ErrAirGapTSAMissing       ErrorCode = "AIPACK-AIRGAP-003"

	// Pattern errors (§18)
	ErrPatternViolation         ErrorCode = "AIPACK-PATTERN-001"
	ErrManifoldDistanceExceeded ErrorCode = "AIPACK-PATTERN-002"

	// Profile/policy errors (§19)
	ErrProfileFamilyDenied    ErrorCode = "AIPACK-PROFILE-001"
	ErrProfilePredicateDenied ErrorCode = "AIPACK-PROFILE-002"

	// Quarantine trigger errors (§21)
	ErrQuarantineTriggerFired   ErrorCode = "AIPACK-TRIGGER-001"
	ErrQuarantineEscalationFail ErrorCode = "AIPACK-TRIGGER-002"

	// VAD errors (§22)
	ErrVADConsensusFailed  ErrorCode = "AIPACK-VAD-001"
	ErrVADClassUnknown     ErrorCode = "AIPACK-VAD-002"
	ErrVADPerturbationFail ErrorCode = "AIPACK-VAD-003"
)

// AIPackError is the canonical error type for AIPACK-SPEC violations.
type AIPackError struct {
	Code    ErrorCode
	Message string
	// Detail carries additional context (e.g. the offending field or ref).
	Detail string
}

func (e *AIPackError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// newErr constructs an AIPackError with the given code, message, and optional detail.
func newErr(code ErrorCode, msg, detail string) *AIPackError {
	return &AIPackError{Code: code, Message: msg, Detail: detail}
}

// IsCode reports whether err has the given AIPackError code.
func IsCode(err error, code ErrorCode) bool {
	if e, ok := err.(*AIPackError); ok {
		return e.Code == code
	}
	return false
}
