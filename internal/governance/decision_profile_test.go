/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package governance

import (
	"context"
	"errors"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validDecisionProfile() *servingv1alpha2.ModelServingProfile {
	maxLatency := int32(150)
	return &servingv1alpha2.ModelServingProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-decision-profile",
			Namespace: "default",
		},
		Spec: servingv1alpha2.ModelServingProfileSpec{
			WorkloadClass: servingv1alpha2.WorkloadClassDecision,
			DecisionCapabilities: []servingv1alpha2.DecisionCapability{
				servingv1alpha2.DecisionCapabilityPredicate,
				servingv1alpha2.DecisionCapabilityChoice,
				servingv1alpha2.DecisionCapabilityScore,
			},
			Runtime: &servingv1alpha2.RuntimeLinkSpec{
				Engine:   "vllm",
				Endpoint: "http://eval-engine:8000",
			},
			SLA: &servingv1alpha2.ProfileSLA{
				MaxLatencyMillis: &maxLatency,
			},
			Applicability: &servingv1alpha2.ProfileApplicability{
				SupportedRuntimes: []string{"vllm", "triton"},
			},
			Evidence: &servingv1alpha2.ProfileEvidence{
				MinTrustLevel:        "verified",
				RequiredAttestations: []string{"https://ckodex.com/attestation/evaluator/v1"},
				RequireSBOM:          true,
				RequireSignature:     true,
			},
		},
	}
}

func fullySatisfiedTarget() *EvaluatorTarget {
	calScore := 0.98
	return &EvaluatorTarget{
		Name:          "eval-service-1",
		Namespace:     "default",
		WorkloadClass: servingv1alpha2.WorkloadClassDecision,
		Capabilities: []servingv1alpha2.DecisionCapability{
			servingv1alpha2.DecisionCapabilityPredicate,
			servingv1alpha2.DecisionCapabilityChoice,
			servingv1alpha2.DecisionCapabilityScore,
		},
		RuntimeEngine:        "vllm",
		Endpoint:             "http://eval-engine:8000/v1",
		EndpointHealthy:      true,
		EndpointInitializing: false,
		TrustLevel:           "verified",
		Attestations:         []string{"https://ckodex.com/attestation/evaluator/v1"},
		HasSignature:         true,
		HasSBOM:              true,
		ArtifactsResident:    true,
		ArtifactsLoading:     false,
		HasCalibration:       true,
		CalibrationScore:     &calScore,
		ObservedLatencyMs:    95,
		ReadyReplicas:        2,
		DesiredReplicas:      2,
	}
}

func TestValidateModelServingProfile(t *testing.T) {
	t.Run("valid decision profile", func(t *testing.T) {
		profile := validDecisionProfile()
		err := ValidateModelServingProfile(profile)
		require.NoError(t, err)
	})

	t.Run("decision workload class missing capabilities", func(t *testing.T) {
		profile := validDecisionProfile()
		profile.Spec.DecisionCapabilities = nil
		err := ValidateModelServingProfile(profile)
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires at least one decision capability")
	})

	t.Run("invalid workload class", func(t *testing.T) {
		profile := validDecisionProfile()
		profile.Spec.WorkloadClass = "unknown-class"
		err := ValidateModelServingProfile(profile)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid workloadClass")
	})

	t.Run("invalid capability", func(t *testing.T) {
		profile := validDecisionProfile()
		profile.Spec.DecisionCapabilities = []servingv1alpha2.DecisionCapability{"hallucinate"}
		err := ValidateModelServingProfile(profile)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid decisionCapability")
	})
}

func TestEvaluateSubstrateState_AllSatisfied(t *testing.T) {
	profile := validDecisionProfile()
	target := fullySatisfiedTarget()

	report, err := EvaluateSubstrateState(context.Background(), profile, target)
	require.NoError(t, err)
	require.True(t, report.IsOperable)
	require.Empty(t, report.FailureReasons)

	require.Equal(t, servingv1alpha2.VectorStateSatisfied, report.Vector.Ready)
	require.Equal(t, servingv1alpha2.VectorStateSatisfied, report.Vector.Admitted)
	require.Equal(t, servingv1alpha2.VectorStateSatisfied, report.Vector.Calibrated)
	require.Equal(t, servingv1alpha2.VectorStateSatisfied, report.Vector.Compatible)
	require.Equal(t, servingv1alpha2.VectorStateSatisfied, report.Vector.Resident)
	require.Equal(t, servingv1alpha2.VectorStateSatisfied, report.Vector.Available)
}

func TestEvaluateSubstrateState_ReadyDimension(t *testing.T) {
	profile := validDecisionProfile()

	t.Run("initializing endpoint", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.EndpointInitializing = true
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStatePending, report.Vector.Ready)
		require.False(t, report.IsOperable)
	})

	t.Run("unhealthy endpoint", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.EndpointHealthy = false
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Ready)
		require.False(t, report.IsOperable)
	})
}

func TestEvaluateSubstrateState_AdmittedDimension(t *testing.T) {
	profile := validDecisionProfile()

	t.Run("denied trust", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.TrustLevel = "denied"
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Admitted)
	})

	t.Run("insufficient trust", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.TrustLevel = "asserted"
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Admitted)
	})

	t.Run("missing required attestation", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.Attestations = []string{"other-attestation"}
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Admitted)
	})

	t.Run("missing SBOM", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.HasSBOM = false
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Admitted)
	})
}

func TestEvaluateSubstrateState_CalibratedDimension(t *testing.T) {
	profile := validDecisionProfile()

	t.Run("missing calibration on decision workload", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.HasCalibration = false
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Calibrated)
	})

	t.Run("degraded calibration score", func(t *testing.T) {
		target := fullySatisfiedTarget()
		negScore := -0.5
		target.CalibrationScore = &negScore
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateDegraded, report.Vector.Calibrated)
	})

	t.Run("generative workload bypasses calibration", func(t *testing.T) {
		genProfile := validDecisionProfile()
		genProfile.Spec.WorkloadClass = servingv1alpha2.WorkloadClassGenerative
		genProfile.Spec.DecisionCapabilities = nil
		target := fullySatisfiedTarget()
		target.WorkloadClass = servingv1alpha2.WorkloadClassGenerative
		target.HasCalibration = false
		report, err := EvaluateSubstrateState(context.Background(), genProfile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateSatisfied, report.Vector.Calibrated)
	})
}

func TestEvaluateSubstrateState_CompatibleDimension(t *testing.T) {
	profile := validDecisionProfile()

	t.Run("workload class mismatch", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.WorkloadClass = servingv1alpha2.WorkloadClassEmbedding
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Compatible)
	})

	t.Run("missing required decision capability", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.Capabilities = []servingv1alpha2.DecisionCapability{servingv1alpha2.DecisionCapabilityPredicate}
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Compatible)
	})

	t.Run("incompatible runtime engine", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.RuntimeEngine = "custom-unsupported-engine"
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Compatible)
	})
}

func TestEvaluateSubstrateState_ResidentAndAvailableDimensions(t *testing.T) {
	profile := validDecisionProfile()

	t.Run("artifacts loading", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.ArtifactsLoading = true
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStatePending, report.Vector.Resident)
	})

	t.Run("artifacts not resident", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.ArtifactsResident = false
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Resident)
	})

	t.Run("zero ready replicas", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.ReadyReplicas = 0
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateFailed, report.Vector.Available)
	})

	t.Run("latency SLA violated", func(t *testing.T) {
		target := fullySatisfiedTarget()
		target.ObservedLatencyMs = 250 // SLA is 150
		report, err := EvaluateSubstrateState(context.Background(), profile, target)
		require.NoError(t, err)
		require.Equal(t, servingv1alpha2.VectorStateDegraded, report.Vector.Available)
	})
}

func TestExecuteDomainDecision_RefusesSemanticJudgment(t *testing.T) {
	target := fullySatisfiedTarget()
	err := ExecuteDomainDecision(context.Background(), target, map[string]string{"input": "prompt"})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrOperatorNotSemanticAuthority), "must wrap ErrOperatorNotSemanticAuthority")
}

func TestApplySubstrateStatus(t *testing.T) {
	profile := validDecisionProfile()
	target := fullySatisfiedTarget()
	report, err := EvaluateSubstrateState(context.Background(), profile, target)
	require.NoError(t, err)

	status := &servingv1alpha2.ModelServingProfileStatus{}
	ApplySubstrateStatus(status, report)

	require.Equal(t, report.Vector, status.SubstrateVector)
	require.Len(t, status.Conditions, 1)
	require.Equal(t, metav1.ConditionTrue, status.Conditions[0].Status)
	require.Equal(t, "SubstrateOperable", status.Conditions[0].Reason)
}
