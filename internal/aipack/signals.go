package aipack

// AIPACK observability signal namespace per AIPACK-SPEC v0.1.1 §20.
// All OTel attributes and metric names must use this prefix.
const SignalNamespace = "ai.aipack"

// OTel span names per §20.
const (
	SpanCompositionValidate = "ai.aipack.composition.validate"
	SpanAttestationVerify   = "ai.aipack.attestation.verify"
	SpanPolicyEvaluate      = "ai.aipack.policy.evaluate"
	SpanRiskValenceCompute  = "ai.aipack.riskvalence.compute"
	SpanQuarantineCheck     = "ai.aipack.quarantine.check"
)

// OTel metric names per §20.
const (
	MetricArtifactsTotal         = "ai.aipack.artifacts.total"
	MetricAttestationVerifyTotal = "ai.aipack.attestation.verify.total"
	MetricRiskScore              = "ai.aipack.risk.score"
	MetricCompositionErrors      = "ai.aipack.composition.errors.total"
	MetricQuarantineTotal        = "ai.aipack.quarantine.total"
)

// OTel attribute keys per §20.
const (
	AttrKind             = "ai.aipack.kind"
	AttrFamily           = "ai.aipack.family"
	AttrRiskBand         = "ai.aipack.risk.band"
	AttrErrorCode        = "ai.aipack.error.code"
	AttrArtifactRef      = "ai.aipack.artifact.ref"
	AttrDeprecationPhase = "ai.aipack.deprecation.phase"
)
