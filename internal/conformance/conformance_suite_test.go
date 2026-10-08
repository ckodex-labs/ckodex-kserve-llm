/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package conformance

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandardSuite12ClassCoverage(t *testing.T) {
	suite, err := Standard12ClassSuite()
	require.NoError(t, err)
	require.NotNil(t, suite)

	for _, expectedClass := range AllConformanceClasses {
		vectors := suite.VectorsByClass(expectedClass)
		assert.NotEmpty(t, vectors, "Missing vectors for Conformance Class %s", expectedClass)
	}

	summary, err := suite.Run(context.Background())
	require.NoError(t, err)
	require.NotNil(t, summary)

	assert.Equal(t, len(suite.Vectors()), summary.TotalVectors)
	for _, expectedClass := range AllConformanceClasses {
		count := summary.ClassCoverage[expectedClass]
		assert.Greater(t, count, 0, "Class %s had 0 executions in summary", expectedClass)
	}
}

func registerPassingVectors(t *testing.T, suite *ConformanceSuite, count int) {
	t.Helper()
	for i := 1; i <= count; i++ {
		vecID := fmt.Sprintf("vec-pass-%03d", i)
		err := suite.Register(ConformanceVector{
			ID:          vecID,
			Name:        fmt.Sprintf("Standard Passing Vector %d", i),
			Class:       ClassPositive,
			Severity:    SeverityStandard,
			Target:      "GenericTarget",
			Description: "Simulated standard passing test",
			Execute: func(_ context.Context) (*VectorResult, error) {
				return &VectorResult{
					VectorID: vecID,
					Class:    ClassPositive,
					Severity: SeverityStandard,
					Verdict:  VerdictSatisfied,
					ObservedState: StatePlaneVector{
						Lifecycle: "active",
						Trust:     "verified",
						Integrity: "verified",
					},
					Message: "Pass",
				}, nil
			},
		})
		require.NoError(t, err)
	}
}

func TestHardAntiInvariantDominanceOver99PercentPassRate(t *testing.T) {
	suite := NewConformanceSuite()
	registerPassingVectors(t, suite, 99)

	// Register 1 failing Hard Anti-Invariant vector
	antiID := "vec-anti-critical-breach"
	err := suite.Register(ConformanceVector{
		ID:          antiID,
		Name:        "Adversarial Escape Vector",
		Class:       ClassAnti,
		Severity:    SeverityHardAnti,
		Target:      "SecurityKernel",
		Description: "Simulated hard anti-invariant violation",
		Execute: func(_ context.Context) (*VectorResult, error) {
			return &VectorResult{
				VectorID: antiID,
				Class:    ClassAnti,
				Severity: SeverityHardAnti,
				Verdict:  VerdictHaltViolation,
				ObservedState: StatePlaneVector{
					Lifecycle: "halted",
					Trust:     "denied",
					Integrity: "compromised",
				},
				Message: "Critical adversarial escape detected",
			}, nil
		},
	})
	require.NoError(t, err)

	summary, err := suite.Run(context.Background())
	require.NoError(t, err)
	require.NotNil(t, summary)

	// In arithmetic terms, pass rate is 99% (99 / 100).
	// Under CKODEX Constitution, 99% pass rate NEVER masks a hard anti-invariant failure.
	assert.Equal(t, 100, summary.TotalVectors)
	assert.Equal(t, 99, summary.PassedCount)
	assert.Equal(t, 1, summary.HardViolationCount)

	// Hard anti-invariant strictly dominates the categorical state:
	assert.Equal(t, VerdictHaltViolation, summary.CategoricalVerdict)
	assert.True(t, summary.DominanceInvoked, "Dominance must be explicitly flagged")
	assert.Equal(t, "denied", summary.CompositeTrust)
	assert.False(t, summary.IsViable(), "System with hard anti-violation is non-viable")
}

func TestGLM53EXL3TensorFoldVectors(t *testing.T) {
	suite, err := Standard12ClassSuite()
	require.NoError(t, err)

	glmVectors := suite.VectorsByTarget(TargetGLM53EXL3TensorFold)
	require.NotEmpty(t, glmVectors)

	classesFound := make(map[ConformanceClass]bool)
	for _, v := range glmVectors {
		classesFound[v.Class] = true
		res, err := v.Execute(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, res.EvidenceDigest)

		switch v.ID {
		case "vec-pos-glm53-tensorfold":
			assert.Equal(t, VerdictSatisfied, res.Verdict)
			assert.Equal(t, "bound-4x-rtx-6000", res.ObservedState.Binding)
		case "vec-anti-glm53-tamper":
			assert.Equal(t, VerdictHaltViolation, res.Verdict)
			assert.Equal(t, SeverityHardAnti, res.Severity)
			assert.Equal(t, "denied", res.ObservedState.Trust)
		case "vec-vbw-glm53-5fold-on-4gpu":
			assert.Equal(t, VerdictRejected, res.Verdict)
		case "vec-sat-glm53-queue-overflow":
			assert.Equal(t, VerdictSatisfied, res.Verdict)
			assert.Equal(t, "backpressure-active", res.ObservedState.Binding)
		case "vec-rec-tensorfold-pcie-reset":
			assert.Equal(t, VerdictSatisfied, res.Verdict)
		}
	}

	assert.True(t, classesFound[ClassPositive])
	assert.True(t, classesFound[ClassAnti])
	assert.True(t, classesFound[ClassValidButWrong])
	assert.True(t, classesFound[ClassSaturation])
	assert.True(t, classesFound[ClassRecovery])
}

func TestVLLMNVFP4FlashMLAVectors(t *testing.T) {
	suite, err := Standard12ClassSuite()
	require.NoError(t, err)

	vllmVectors := suite.VectorsByTarget(TargetVLLMNVFP4FlashMLA)
	require.NotEmpty(t, vllmVectors)

	for _, v := range vllmVectors {
		res, err := v.Execute(context.Background())
		require.NoError(t, err)

		switch v.ID {
		case "vec-pos-vllm-nvfp4-flashmla":
			assert.Equal(t, VerdictSatisfied, res.Verdict)
			assert.Equal(t, "vllm-0.31.0-nvfp4-flashmla", res.ObservedState.Binding)
		case "vec-neg-vllm-nvfp4-arch":
			assert.Equal(t, VerdictRejected, res.Verdict)
		case "vec-anti-vllm-nvfp4-bypass":
			assert.Equal(t, VerdictHaltViolation, res.Verdict)
			assert.Equal(t, SeverityHardAnti, res.Severity)
		case "vec-deg-flashmla-kv-fragmentation":
			assert.Equal(t, VerdictDegraded, res.Verdict)
			assert.Equal(t, "degraded", res.ObservedState.Lifecycle)
		case "vec-race-flashmla-kv-alloc":
			assert.Equal(t, VerdictSatisfied, res.Verdict)
		}
	}
}

func TestDecisionEvaluatorVectors(t *testing.T) {
	suite, err := Standard12ClassSuite()
	require.NoError(t, err)

	evalVectors := suite.VectorsByTarget(TargetDecisionEvaluator)
	require.NotEmpty(t, evalVectors)

	for _, v := range evalVectors {
		res, err := v.Execute(context.Background())
		require.NoError(t, err)

		switch v.ID {
		case "vec-pos-evaluator":
			assert.Equal(t, VerdictSatisfied, res.Verdict)
		case "vec-neg-evaluator-missing-sbom":
			assert.Equal(t, VerdictRejected, res.Verdict)
		case "vec-anti-evaluator-forbidden-tuple":
			assert.Equal(t, VerdictHaltViolation, res.Verdict)
			assert.Equal(t, SeverityHardAnti, res.Severity)
		case "vec-temp-evaluator-lease-expired":
			assert.Equal(t, VerdictRejected, res.Verdict)
		case "vec-meta-evaluator-whitespace":
			assert.Equal(t, VerdictSatisfied, res.Verdict)
		case "vec-silence-missing-inference-receipt":
			assert.Equal(t, VerdictRejected, res.Verdict)
		case "vec-cfr-evaluator-threshold-replay":
			assert.Equal(t, VerdictSatisfied, res.Verdict)
		}
	}
}

func TestCategoricalLatticeResolution(t *testing.T) {
	suite := NewConformanceSuite()

	// Only passing tests -> Satisfied & verified
	summary1 := suite.EvaluateCategoricalState([]VectorResult{
		{Verdict: VerdictSatisfied, Severity: SeverityStandard},
	})
	assert.Equal(t, VerdictSatisfied, summary1.CategoricalVerdict)
	assert.Equal(t, "verified", summary1.CompositeTrust)
	assert.False(t, summary1.DominanceInvoked)

	// Degraded present -> Degraded & asserted
	summary2 := suite.EvaluateCategoricalState([]VectorResult{
		{Verdict: VerdictSatisfied, Severity: SeverityStandard},
		{Verdict: VerdictDegraded, Severity: SeverityDegradable},
	})
	assert.Equal(t, VerdictDegraded, summary2.CategoricalVerdict)
	assert.Equal(t, "asserted", summary2.CompositeTrust)

	// Rejected present -> Rejected & untrusted
	summary3 := suite.EvaluateCategoricalState([]VectorResult{
		{Verdict: VerdictSatisfied, Severity: SeverityStandard},
		{Verdict: VerdictDegraded, Severity: SeverityDegradable},
		{Verdict: VerdictRejected, Severity: SeverityStandard},
	})
	assert.Equal(t, VerdictRejected, summary3.CategoricalVerdict)
	assert.Equal(t, "untrusted", summary3.CompositeTrust)

	// Quarantined present -> Quarantined
	summary4 := suite.EvaluateCategoricalState([]VectorResult{
		{Verdict: VerdictSatisfied, Severity: SeverityStandard},
		{Verdict: VerdictQuarantined, Severity: SeverityStandard},
	})
	assert.Equal(t, VerdictQuarantined, summary4.CategoricalVerdict)
	assert.Equal(t, "quarantined", summary4.CompositeTrust)
}
