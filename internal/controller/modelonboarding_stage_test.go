/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// stageReadyLLMSvc returns a ready LLMInferenceService with replicas for stage tests.
func stageReadyLLMSvc(name string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{Name: name, URI: "hf://test/test"},
		},
		Status: servingv1alpha2.LLMInferenceServiceStatus{
			ModelReady: true,
			Replicas:   2,
		},
	}
}

// stageNotReadyLLMSvc returns a not-ready LLMInferenceService with zero replicas.
func stageNotReadyLLMSvc(name string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{Name: name, URI: "hf://test/test"},
		},
		Status: servingv1alpha2.LLMInferenceServiceStatus{
			ModelReady: false,
			Replicas:   0,
		},
	}
}

// simpleModelOnboarding builds a minimal ModelOnboarding for executeStage tests.
func simpleModelOnboarding(name string) *servingv1alpha2.ModelOnboarding {
	return &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       servingv1alpha2.ModelOnboardingSpec{ModelRef: name},
	}
}

// ---- executeStage -----------------------------------------------------------

// TestExecuteStage_Validation_Passes when LLMInferenceService is ready.
func TestExecuteStage_Validation_Passes(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageReadyLLMSvc("my-model")
	stage := servingv1alpha2.OnboardingStage{Name: "validate", Type: stageTypeValidation}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.NoError(t, err)
}

// TestExecuteStage_Validation_Fails when LLMInferenceService not ready.
func TestExecuteStage_Validation_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageNotReadyLLMSvc("my-model")
	stage := servingv1alpha2.OnboardingStage{Name: "validate", Type: stageTypeValidation}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")
}

// TestExecuteStage_Canary_Passes when replicas >= 1.
func TestExecuteStage_Canary_Passes(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageReadyLLMSvc("my-model")
	stage := servingv1alpha2.OnboardingStage{Name: "canary", Type: stageTypeCanary}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.NoError(t, err)
}

// TestExecuteStage_Canary_Fails when no ready replicas.
func TestExecuteStage_Canary_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageNotReadyLLMSvc("my-model")
	stage := servingv1alpha2.OnboardingStage{Name: "canary", Type: stageTypeCanary}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ready replicas")
}

// TestExecuteStage_Gate_NilCriteria_Passes when gate has no criteria defined.
func TestExecuteStage_Gate_NilCriteria_Passes(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageReadyLLMSvc("my-model")
	stage := servingv1alpha2.OnboardingStage{Name: "gate", Type: stageTypeGate, Gate: nil}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.NoError(t, err)
}

// TestExecuteStage_Gate_WithCriteria_PassesViaExplicitInsecureFallback uses the
// opt-in compatibility fallback when no Prometheus backend is present.
func TestExecuteStage_Gate_WithCriteria_PassesViaExplicitInsecureFallback(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: NewInsecurePassMetricsQuerier(),
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageReadyLLMSvc("my-model")

	minSuccess := int32(95)
	stage := servingv1alpha2.OnboardingStage{
		Name: "gate",
		Type: stageTypeGate,
		Gate: &servingv1alpha2.GateCriteria{MinSuccessRate: minSuccess},
	}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.NoError(t, err)
}

// TestExecuteStage_Gate_FailsWhenSuccessRateLow uses a mock that returns low success.
func TestExecuteStage_Gate_FailsWhenSuccessRateLow(t *testing.T) {
	scheme := newControllerScheme(t)
	mock := &mockMetricsQuerier{successRate: 50.0}
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: mock,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageReadyLLMSvc("my-model")

	minSuccess := int32(95)
	stage := servingv1alpha2.OnboardingStage{
		Name: "gate",
		Type: stageTypeGate,
		Gate: &servingv1alpha2.GateCriteria{MinSuccessRate: minSuccess},
	}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "success rate")
}

// TestExecuteStage_Gate_FailsWhenP99TooHigh uses a mock that returns high latency.
func TestExecuteStage_Gate_FailsWhenP99TooHigh(t *testing.T) {
	scheme := newControllerScheme(t)
	maxLatency := int64(200) // 200ms max
	mock := &mockMetricsQuerier{successRate: 99.0, p99MS: 500}
	r := &ModelOnboardingReconciler{
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:  scheme,
		Metrics: mock,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageReadyLLMSvc("my-model")

	stage := servingv1alpha2.OnboardingStage{
		Name: "gate",
		Type: stageTypeGate,
		Gate: &servingv1alpha2.GateCriteria{
			MinSuccessRate: 95,
			MaxLatencyP99:  &maxLatency,
		},
	}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "P99 latency")
}

// TestExecuteStage_Promotion_Passes when LLMInferenceService is ready.
func TestExecuteStage_Promotion_Passes(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageReadyLLMSvc("my-model")
	stage := servingv1alpha2.OnboardingStage{Name: "promote", Type: stageTypePromotion}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.NoError(t, err)
}

// TestExecuteStage_Promotion_Fails when not ready.
func TestExecuteStage_Promotion_Fails(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageNotReadyLLMSvc("my-model")
	stage := servingv1alpha2.OnboardingStage{Name: "promote", Type: stageTypePromotion}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not ready for promotion")
}

// TestExecuteStage_UnknownType_FailsClosed rejects unknown stage types.
func TestExecuteStage_UnknownType_FailsClosed(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &ModelOnboardingReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	ob := simpleModelOnboarding("my-model")
	llmSvc := stageNotReadyLLMSvc("my-model")
	stage := servingv1alpha2.OnboardingStage{Name: "custom", Type: "future-stage-type"}

	err := r.executeStage(context.Background(), ob, llmSvc, stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported onboarding stage type")
}

// ---- insecurePassMetricsQuerier ---------------------------------------------

func TestInsecurePassMetricsQuerier_QuerySuccessRate(t *testing.T) {
	q := insecurePassMetricsQuerier{}
	rate, err := q.QuerySuccessRate(context.Background(), "model", "ns")
	require.NoError(t, err)
	assert.Equal(t, float64(100), rate)
}

func TestInsecurePassMetricsQuerier_QueryP99LatencyMS(t *testing.T) {
	q := insecurePassMetricsQuerier{}
	ms, err := q.QueryP99LatencyMS(context.Background(), "model", "ns")
	require.NoError(t, err)
	assert.Equal(t, int64(0), ms)
}

func TestModelOnboardingReconciler_MetricsQuerier_Nil_ReturnsNil(t *testing.T) {
	r := &ModelOnboardingReconciler{Metrics: nil}
	q := r.metricsQuerier()
	assert.Nil(t, q)
}

// TestModelOnboardingReconciler_MetricsQuerier_Set_ReturnsMock verifies injected querier.
func TestModelOnboardingReconciler_MetricsQuerier_Set_ReturnsMock(t *testing.T) {
	mock := &mockMetricsQuerier{successRate: 99.0}
	r := &ModelOnboardingReconciler{Metrics: mock}
	q := r.metricsQuerier()
	assert.Equal(t, mock, q)
}
