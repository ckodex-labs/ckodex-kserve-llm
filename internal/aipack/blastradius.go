package aipack

// BlastRadiusDeclaration is the AIPACK-SPEC v0.1.1 §12 artifact declaring the dependency
// inversion index and downstream artifact set.
// Backed by attestation urn:aipack:blast-radius-declaration:v1.
type BlastRadiusDeclaration struct {
	// ArtifactRef is the root artifact this declaration is about.
	ArtifactRef string `json:"artifactRef"`

	// DownstreamCount is the count of known downstream artifacts.
	DownstreamCount int `json:"downstreamCount"`

	// DownstreamRefs lists OCI digest references of downstream artifacts.
	DownstreamRefs []string `json:"downstreamRefs"`

	// InversionIndex is the computed dependency inversion score (higher = broader blast radius).
	InversionIndex float64 `json:"inversionIndex"`
}

// ComputeBlastRadius computes the downstream impact for a given artifact digest.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §12 — query dependency graph
func ComputeBlastRadius(_ string) (*BlastRadiusDeclaration, error) {
	return nil, newErr(ErrBlastRadiusExceeded, "blast radius computation not yet implemented", "")
}

// EmitQuarantineCascade emits a quarantine cascade signal for the given artifact and its
// downstream dependents.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §12 — emit urn:aipack:quarantine-cascade:v1
func EmitQuarantineCascade(_ *BlastRadiusDeclaration) error {
	return newErr(ErrBlastRadiusExceeded, "quarantine cascade emission not yet implemented", "")
}
