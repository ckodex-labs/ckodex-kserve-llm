package aipack

// LineageEnvelope is the AIPACK-SPEC v0.1.1 §11 lineage record that captures
// per-component attribution for an artifact assembly.
// Backed by attestation urn:aipack:lineage-envelope:v1.
type LineageEnvelope struct {
	// ArtifactRef is the OCI digest reference of the artifact this envelope describes.
	ArtifactRef string `json:"artifactRef"`

	// Components lists per-component attribution records.
	Components []ComponentAttribution `json:"components"`

	// AssemblyHash is the sha256 of the canonical lineage JSON.
	AssemblyHash string `json:"assemblyHash"`
}

// ComponentAttribution records the contribution of a single component to an assembly.
type ComponentAttribution struct {
	// Ref is the OCI digest reference of the contributing artifact.
	Ref string `json:"ref"`

	// Role is the slot name in the composition (e.g. "baseModel", "skills[0]").
	Role string `json:"role"`

	// Digest is the sha256 digest of the component at attribution time.
	Digest string `json:"digest"`
}

// BuildLineageEnvelope constructs a LineageEnvelope for the given artifact digest and components.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §11 — compute AssemblyHash via RFC 8785
func BuildLineageEnvelope(_ string, _ []ComponentAttribution) (*LineageEnvelope, error) {
	return nil, newErr(ErrLineageEnvelopeMissing, "lineage envelope construction not yet implemented", "")
}

// VerifyLineageHash verifies the AssemblyHash of a LineageEnvelope.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §11 — verify canonical JSON hash
func VerifyLineageHash(_ *LineageEnvelope) error {
	return newErr(ErrLineageHashMismatch, "lineage hash verification not yet implemented", "")
}
