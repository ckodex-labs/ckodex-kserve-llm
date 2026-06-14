package aipack

// AirGapValidityDaysRegulated is the maximum bundle validity window for regulated environments
// per AIPACK-SPEC v0.1.1 §17.3.
const AirGapValidityDaysRegulated = 30

// AirGapBundle is the §17 offline distribution bundle containing an AIPACK artifact
// with embedded trust roots and offline TSA tokens.
// Backed by attestation urn:aipack:airgap-bundle:v1.
type AirGapBundle struct {
	// ArtifactRef is the OCI digest reference of the bundled artifact.
	ArtifactRef string `json:"artifactRef"`

	// TrustRoots contains PEM-encoded trusted CA certificates for offline verification.
	TrustRoots []string `json:"trustRoots"`

	// TSAToken is the RFC 3161 timestamp token issued at bundle creation time.
	TSAToken string `json:"tsaToken"`

	// ValidUntil is the ISO 8601 expiry timestamp for this bundle.
	ValidUntil string `json:"validUntil"`

	// Profile declares the regulatory profile (e.g. "regulated", "standard").
	Profile string `json:"profile,omitempty"`
}

// AssembleAirGapBundle creates an offline distribution bundle for the given artifact.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §17 — embed trust roots + TSA token
func AssembleAirGapBundle(_ string, _ []string) (*AirGapBundle, error) {
	return nil, newErr(ErrAirGapTrustRootMissing, "air-gap bundle assembly not yet implemented", "")
}

// ValidateInternalAirGapBundle verifies that an internal AirGapBundle is within its validity
// window and has the required trust roots and TSA token.
// For v1alpha2.AIPackAirGapBundle validation see ValidateAirGapBundle in validators_operational.go.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §17 — check expiry + TSA verification
func ValidateInternalAirGapBundle(_ *AirGapBundle) error {
	return newErr(ErrAirGapBundleExpired, "air-gap bundle validation not yet implemented", "")
}
