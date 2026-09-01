/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/residency"
)

func TestOnboardingResidencyReadinessRequiresObservedPrefill(t *testing.T) {
	service := stageReadyLLMSvc("pd-model")
	service.Generation = 3
	service.Spec.Prefill = &servingv1alpha2.PrefillSpec{}

	status, policy, err := onboardingResidencyReadiness(service)
	require.NoError(t, err)
	assert.Equal(t, residency.StateReady, status.State)
	assert.Equal(t, "PrefillUnavailable", status.Reason)
	assert.False(t, status.Ready)
	assert.False(t, policy.AllowRoute)

	service.Status.Conditions = []metav1.Condition{{
		Type: servingv1alpha2.ConditionPrefillReady, Status: metav1.ConditionTrue,
		ObservedGeneration: service.Generation,
	}}
	status, policy, err = onboardingResidencyReadiness(service)
	require.NoError(t, err)
	assert.True(t, status.Ready)
	assert.True(t, policy.AllowRoute)
}

func TestOnboardingResidencyReadinessRejectsStalePrefillEvidence(t *testing.T) {
	service := stageReadyLLMSvc("pd-model")
	service.Generation = 4
	service.Spec.Prefill = &servingv1alpha2.PrefillSpec{}
	service.Status.Conditions = []metav1.Condition{{
		Type: servingv1alpha2.ConditionPrefillReady, Status: metav1.ConditionTrue,
		ObservedGeneration: 3,
	}}

	err := ensureOnboardingResidencyReady(service)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PrefillUnavailable")
}

func TestOnboardingResidencyReadinessRejectsOrphanGateway(t *testing.T) {
	service := stageNotReadyLLMSvc("orphan-route")
	service.Generation = 2
	service.Status.Conditions = []metav1.Condition{{
		Type: servingv1alpha2.ConditionGatewayReady, Status: metav1.ConditionTrue,
		ObservedGeneration: service.Generation,
	}}

	err := ensureOnboardingResidencyReady(service)
	require.Error(t, err)
	assert.ErrorContains(t, err, "route attached without a ready runtime")
}

func TestExecuteValidationStageBlocksPDUntilPrefillReady(t *testing.T) {
	service := stageReadyLLMSvc("pd-model")
	service.Generation = 1
	service.Spec.Prefill = &servingv1alpha2.PrefillSpec{}
	stage := servingv1alpha2.OnboardingStage{Name: "validate", Type: stageTypeValidation}
	reconciler := &ModelOnboardingReconciler{}

	err := reconciler.executeValidationStage(t.Context(), simpleModelOnboarding("pd-model"), service, stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PrefillUnavailable")

	service.Status.Conditions = []metav1.Condition{{
		Type: servingv1alpha2.ConditionPrefillReady, Status: metav1.ConditionTrue,
		ObservedGeneration: service.Generation,
	}}
	require.NoError(t, reconciler.executeValidationStage(t.Context(), simpleModelOnboarding("pd-model"), service, stage))
}
