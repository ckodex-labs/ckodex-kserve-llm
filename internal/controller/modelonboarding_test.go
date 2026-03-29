/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ---- mockMetricsQuerier --------------------------------------------------

type mockMetricsQuerier struct {
	successRate float64
	successErr  error
	p99MS       int64
	p99Err      error
}

func (m *mockMetricsQuerier) QuerySuccessRate(_ context.Context, _, _ string) (float64, error) {
	return m.successRate, m.successErr
}

func (m *mockMetricsQuerier) QueryP99LatencyMS(_ context.Context, _, _ string) (int64, error) {
	return m.p99MS, m.p99Err
}

// ---- checkGateCriteria ---------------------------------------------------

func readyLLMSvcWithReplicas(name string, replicas int32) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{Name: name, URI: "hf://test/test"},
		},
		Status: servingv1alpha2.LLMInferenceServiceStatus{
			ModelReady: true,
			Replicas:   replicas,
		},
	}
}

func ptr64(v int64) *int64 { return &v }

func TestCheckGateCriteria_NoReplicas_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 0)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90}
	err := r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ready replicas")
}

func TestCheckGateCriteria_ModelNotReady_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	llmSvc.Status.ModelReady = false
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90}
	err := r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model not ready")
}

func TestCheckGateCriteria_MetricsError_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: &mockMetricsQuerier{successErr: errors.New("prometheus unreachable")},
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90}
	err := r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metrics unavailable")
}

func TestCheckGateCriteria_LowSuccessRate_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: &mockMetricsQuerier{successRate: 85.0},
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 2)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90}
	err := r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "success rate")
	assert.Contains(t, err.Error(), "85")
}

func TestCheckGateCriteria_ExactSuccessRate_Passes(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: &mockMetricsQuerier{successRate: 90.0},
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90}
	assert.NoError(t, r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate))
}

func TestCheckGateCriteria_HighSuccessRate_Passes(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: &mockMetricsQuerier{successRate: 99.5},
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 3)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 95}
	assert.NoError(t, r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate))
}

func TestCheckGateCriteria_P99LatencyExceeded_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: &mockMetricsQuerier{successRate: 99.0, p99MS: 600},
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	maxP99 := int64(500)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90, MaxLatencyP99: &maxP99}
	err := r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "P99 latency 600ms > max allowed 500ms")
}

func TestCheckGateCriteria_P99LatencyWithinBound_Passes(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: &mockMetricsQuerier{successRate: 99.0, p99MS: 450},
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	maxP99 := int64(500)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90, MaxLatencyP99: &maxP99}
	assert.NoError(t, r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate))
}

func TestCheckGateCriteria_P99MetricsError_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: &mockMetricsQuerier{successRate: 99.0, p99Err: errors.New("timeout")},
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	maxP99 := int64(500)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90, MaxLatencyP99: &maxP99}
	err := r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "P99 latency metrics unavailable")
}

func TestCheckGateCriteria_NilMaxLatency_SkipsP99Check(t *testing.T) {
	// When MaxLatencyP99 is nil, P99 check is skipped entirely.
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
		// p99Err would fire if P99 check ran.
		Metrics: &mockMetricsQuerier{successRate: 99.0, p99Err: errors.New("should not be called")},
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90, MaxLatencyP99: nil}
	assert.NoError(t, r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate))
}

func TestCheckGateCriteria_ZeroMaxLatency_SkipsP99Check(t *testing.T) {
	// MaxLatencyP99 == 0 means "not set" — skip check.
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: &mockMetricsQuerier{successRate: 99.0, p99Err: errors.New("should not be called")},
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 90, MaxLatencyP99: ptr64(0)}
	assert.NoError(t, r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate))
}

// ---- noopMetricsQuerier (used when Metrics == nil) ----------------------

func TestCheckGateCriteria_NilMetrics_UsesNoop(t *testing.T) {
	scheme := newControllerScheme(t)
	// Metrics is nil → noopMetricsQuerier returns 100%/0ms → gate always passes.
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: nil,
	}
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	gate := &servingv1alpha2.GateCriteria{MinSuccessRate: 99}
	assert.NoError(t, r.checkGateCriteria(context.Background(), &servingv1alpha2.ModelOnboarding{}, llmSvc, gate))
}

// ---- nextStageIndex ------------------------------------------------------

func makeOnboarding(stages []string, currentStage, phase string) *servingv1alpha2.ModelOnboarding {
	spec := make([]servingv1alpha2.OnboardingStage, 0, len(stages))
	for _, s := range stages {
		spec = append(spec, servingv1alpha2.OnboardingStage{Name: s, Type: stageTypeValidation})
	}
	return &servingv1alpha2.ModelOnboarding{
		Spec:   servingv1alpha2.ModelOnboardingSpec{Stages: spec},
		Status: servingv1alpha2.ModelOnboardingStatus{CurrentStage: currentStage, Phase: phase},
	}
}

func TestNextStageIndex_EmptyStages(t *testing.T) {
	r := &ModelOnboardingReconciler{}
	ob := makeOnboarding(nil, "", "")
	assert.Equal(t, 0, r.nextStageIndex(ob))
}

func TestNextStageIndex_PendingPhase_AlwaysZero(t *testing.T) {
	r := &ModelOnboardingReconciler{}
	ob := makeOnboarding([]string{"validate", "canary"}, "validate", phasePending)
	assert.Equal(t, 0, r.nextStageIndex(ob))
}

func TestNextStageIndex_FirstStageNotYetComplete(t *testing.T) {
	r := &ModelOnboardingReconciler{}
	ob := makeOnboarding([]string{"validate", "canary", "promote"}, "", phaseInProgress)
	assert.Equal(t, 0, r.nextStageIndex(ob))
}

func TestNextStageIndex_AfterFirstStage(t *testing.T) {
	r := &ModelOnboardingReconciler{}
	ob := makeOnboarding([]string{"validate", "canary", "promote"}, "validate", phaseInProgress)
	assert.Equal(t, 1, r.nextStageIndex(ob))
}

func TestNextStageIndex_AfterSecondStage(t *testing.T) {
	r := &ModelOnboardingReconciler{}
	ob := makeOnboarding([]string{"validate", "canary", "promote"}, "canary", phaseInProgress)
	assert.Equal(t, 2, r.nextStageIndex(ob))
}

func TestNextStageIndex_AllStagesDone_ReturnsLen(t *testing.T) {
	r := &ModelOnboardingReconciler{}
	stages := []string{"validate", "canary", "promote"}
	ob := makeOnboarding(stages, "promote", phaseInProgress)
	// "promote" is the last stage — next index is 3 (== len(stages))
	assert.Equal(t, 3, r.nextStageIndex(ob))
}

// ---- progressingConditionStatus -----------------------------------------

func TestProgressingConditionStatus(t *testing.T) {
	assert.Equal(t, metav1.ConditionTrue, progressingConditionStatus(phaseInProgress))
	assert.Equal(t, metav1.ConditionTrue, progressingConditionStatus(phasePending))
	assert.Equal(t, metav1.ConditionFalse, progressingConditionStatus(phaseCompleted))
	assert.Equal(t, metav1.ConditionFalse, progressingConditionStatus(phaseFailed))
	assert.Equal(t, metav1.ConditionFalse, progressingConditionStatus(phaseRolledBack))
}

// ---- ModelOnboardingReconciler.Reconcile --------------------------------

func makeModelOnboarding(name, modelRef string, stages ...servingv1alpha2.OnboardingStage) *servingv1alpha2.ModelOnboarding {
	return &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.ModelOnboardingSpec{
			ModelRef:          modelRef,
			Stages:            stages,
			RollbackOnFailure: true,
		},
	}
}

func TestModelOnboardingReconcile_NotFound_NoError(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "gone", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestModelOnboardingReconcile_AddsFinalizer(t *testing.T) {
	scheme := newControllerScheme(t)
	ob := makeModelOnboarding("pipeline", "llama3")

	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ob).WithStatusSubresource(ob).Build(),
		Scheme: scheme,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "pipeline", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.ModelOnboarding
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "pipeline", Namespace: "default"}, &updated))
	assert.Contains(t, updated.Finalizers, FinalizerName)
}

func TestModelOnboardingReconcile_NoStages_Completes(t *testing.T) {
	scheme := newControllerScheme(t)
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	ob := makeModelOnboarding("pipeline", "llama3")
	// Set phase to Pending so it skips the init transition and goes to advancePipeline.
	ob.Status.Phase = phasePending

	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(llmSvc, ob).WithStatusSubresource(ob).Build(),
		Scheme: scheme,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "pipeline", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.ModelOnboarding
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "pipeline", Namespace: "default"}, &updated))
	assert.Equal(t, phaseCompleted, updated.Status.Phase)
}

func TestModelOnboardingReconcile_ModelNotFound_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	ob := makeModelOnboarding("pipeline", "nonexistent-model")
	ob.Status.Phase = phasePending

	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ob).WithStatusSubresource(ob).Build(),
		Scheme: scheme,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "pipeline", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.ModelOnboarding
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "pipeline", Namespace: "default"}, &updated))
	assert.Equal(t, phaseFailed, updated.Status.Phase)
}

func TestModelOnboardingReconcile_CompletedPhase_NoOp(t *testing.T) {
	scheme := newControllerScheme(t)
	ob := makeModelOnboarding("pipeline", "llama3")
	ob.Status.Phase = phaseCompleted

	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ob).WithStatusSubresource(ob).Build(),
		Scheme: scheme,
	}
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "pipeline", Namespace: "default"},
	})
	require.NoError(t, err)
	// Terminal phase: no requeue
	assert.Equal(t, ctrl.Result{}, result)
}

func TestModelOnboardingReconcile_ValidationStage_ModelNotReady_RollsBack(t *testing.T) {
	scheme := newControllerScheme(t)
	llmSvc := readyLLMSvcWithReplicas("llama3", 1)
	llmSvc.Status.ModelReady = false

	stage := servingv1alpha2.OnboardingStage{Name: "validate", Type: stageTypeValidation}
	ob := makeModelOnboarding("pipeline", "llama3", stage)
	ob.Status.Phase = phasePending

	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(llmSvc, ob).WithStatusSubresource(ob).Build(),
		Scheme: scheme,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "pipeline", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.ModelOnboarding
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "pipeline", Namespace: "default"}, &updated))
	assert.Equal(t, phaseRolledBack, updated.Status.Phase)
}
