package aipack

import (
	"testing"

	v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/require"
)

func TestValidatePattern_FailsClosedUntilImplemented(t *testing.T) {
	err := ValidatePattern(PatternBaselineAgent, &v1alpha2.AIPackComposition{})
	require.Error(t, err)
	require.True(t, IsCode(err, ErrNotImplemented))
}

func TestValidateVADDeclaration_FailsClosedUntilImplemented(t *testing.T) {
	err := ValidateVADDeclaration(&VADDeclaration{ArtifactRef: "oci.example/model@sha256:deadbeef"})
	require.Error(t, err)
	require.True(t, IsCode(err, ErrNotImplemented))
}

func TestValidateLineageEnvelope_FailsClosedUntilImplemented(t *testing.T) {
	err := ValidateLineageEnvelope(&v1alpha2.AIPackLineageEnvelope{SourceRef: "oci.example/base@sha256:deadbeef"})
	require.Error(t, err)
	require.True(t, IsCode(err, ErrNotImplemented))
}

func TestValidateAirGapBundle_FailsClosedUntilImplemented(t *testing.T) {
	err := ValidateAirGapBundle(&v1alpha2.AIPackAirGapBundle{
		TrustRootRef: "oci.example/trust@sha256:deadbeef",
		TSACertRef:   "oci.example/tsa@sha256:deadbeef",
	})
	require.Error(t, err)
	require.True(t, IsCode(err, ErrNotImplemented))
}

func TestCheckRVBandBlock_RejectsUnknownBand(t *testing.T) {
	err := CheckRVBandBlock(v1alpha2.RVBand("UNSET"), false)
	require.Error(t, err)
	require.True(t, IsCode(err, ErrRVBandUnknown))
}
