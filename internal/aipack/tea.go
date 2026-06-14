package aipack

import v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"

// TEAWellKnownPath is the well-known endpoint path for the Transparency & Evidence API (§15).
const TEAWellKnownPath = "/.well-known/aipack/tea/v1"

// TEAQueryParams holds query extension parameters for a TEA inventory request.
type TEAQueryParams struct {
	// FamilyFilter restricts results to a specific artifact family.
	FamilyFilter v1alpha2.ArtifactFamily `json:"family,omitempty"`

	// KindFilter restricts results to a specific artifact kind.
	KindFilter v1alpha2.ArtifactKind `json:"kind,omitempty"`

	// DeprecatedOnly restricts results to deprecated artifacts.
	DeprecatedOnly bool `json:"deprecatedOnly,omitempty"`

	// IncludeAttestations requests full attestation bundles in the response.
	IncludeAttestations bool `json:"includeAttestations,omitempty"`
}

// TEAInventoryResponse is the response body from a TEA well-known query.
type TEAInventoryResponse struct {
	// Artifacts is the list of artifact summaries.
	Artifacts []TEAArtifactSummary `json:"artifacts"`

	// TotalCount is the total matching artifact count.
	TotalCount int `json:"totalCount"`
}

// TEAArtifactSummary is a compact artifact record in a TEA response.
type TEAArtifactSummary struct {
	// Ref is the OCI digest reference.
	Ref string `json:"ref"`

	// Kind is the artifact kind.
	Kind v1alpha2.ArtifactKind `json:"kind"`

	// Family is the artifact family.
	Family v1alpha2.ArtifactFamily `json:"family"`

	// RiskBand is the risk valence band.
	RiskBand v1alpha2.RVBand `json:"riskBand,omitempty"`

	// DeprecationPhase is the deprecation lifecycle phase, if applicable.
	DeprecationPhase string `json:"deprecationPhase,omitempty"`
}

// QueryTEA issues a TEA inventory query against the given endpoint URL.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §15 — HTTP GET + JSON decode
func QueryTEA(_ string, _ TEAQueryParams) (*TEAInventoryResponse, error) {
	return nil, newErr(ErrTEAEndpointUnreachable, "TEA query not yet implemented", "")
}
