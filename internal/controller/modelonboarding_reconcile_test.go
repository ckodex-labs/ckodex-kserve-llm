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
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

// ---- advancePipeline -------------------------------------------------------

// TestAdvancePipeline_NoStages_Completes goes directly to Completed when no stages configured.
func TestAdvancePipeline_NoStages_Completes(t *testing.T) {
	scheme := newControllerScheme(t)

	ob := &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-stages",
			Namespace: "default",
		},
		Spec: servingv1alpha2.ModelOnboardingSpec{
			ModelRef: "my-model",
			// Stages deliberately empty
		},
		Status: servingv1alpha2.ModelOnboardingStatus{
			Phase: phaseInProgress,
		},
	}

	llmSvc := stageReadyLLMSvc("my-model")

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ob).
		WithStatusSubresource(ob).
		Build()
	r := &ModelOnboardingReconciler{Client: cl, Scheme: scheme}

	result, err := r.advancePipeline(context.Background(), ob, llmSvc)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, phaseCompleted, ob.Status.Phase)
}

// TestAdvancePipeline_AllStagesPassed_Completes transitions to Completed when all stages done.
func TestAdvancePipeline_AllStagesPassed_Completes(t *testing.T) {
	scheme := newControllerScheme(t)

	ob := &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "all-passed",
			Namespace: "default",
		},
		Spec: servingv1alpha2.ModelOnboardingSpec{
			ModelRef: "my-model",
			Stages: []servingv1alpha2.OnboardingStage{
				{Name: "validate", Type: stageTypeValidation},
			},
		},
		Status: servingv1alpha2.ModelOnboardingStatus{
			Phase:        phaseInProgress,
			CurrentStage: "validate", // last stage already completed
		},
	}

	llmSvc := stageReadyLLMSvc("my-model")

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ob).
		WithStatusSubresource(ob).
		Build()
	r := &ModelOnboardingReconciler{Client: cl, Scheme: scheme}

	result, err := r.advancePipeline(context.Background(), ob, llmSvc)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, phaseCompleted, ob.Status.Phase)
}

// TestAdvancePipeline_StageFails_ErrorReturned returns an error when a stage fails.
func TestAdvancePipeline_StageFails_ErrorReturned(t *testing.T) {
	scheme := newControllerScheme(t)

	ob := &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failing-pipeline",
			Namespace: "default",
		},
		Spec: servingv1alpha2.ModelOnboardingSpec{
			ModelRef: "my-model",
			Stages: []servingv1alpha2.OnboardingStage{
				{Name: "validate", Type: stageTypeValidation},
			},
		},
		Status: servingv1alpha2.ModelOnboardingStatus{
			Phase: phaseInProgress,
		},
	}

	// Not-ready LLMSvc causes validation stage to fail.
	llmSvc := stageNotReadyLLMSvc("my-model")

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ob).
		WithStatusSubresource(ob).
		Build()
	r := &ModelOnboardingReconciler{Client: cl, Scheme: scheme}

	_, err := r.advancePipeline(context.Background(), ob, llmSvc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute stage")
}

// TestAdvancePipeline_StageAdvances_Requeues re-queues after a stage completes.
func TestAdvancePipeline_StageAdvances_Requeues(t *testing.T) {
	scheme := newControllerScheme(t)

	ob := &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "advancing",
			Namespace: "default",
		},
		Spec: servingv1alpha2.ModelOnboardingSpec{
			ModelRef: "my-model",
			Stages: []servingv1alpha2.OnboardingStage{
				{Name: "validate", Type: stageTypeValidation},
				{Name: "promote", Type: stageTypePromotion},
			},
		},
		Status: servingv1alpha2.ModelOnboardingStatus{
			Phase: phaseInProgress,
		},
	}

	llmSvc := stageReadyLLMSvc("my-model")

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ob).
		WithStatusSubresource(ob).
		Build()
	r := &ModelOnboardingReconciler{Client: cl, Scheme: scheme}

	result, err := r.advancePipeline(context.Background(), ob, llmSvc)
	require.NoError(t, err)
	// Must re-queue after advancing one stage.
	assert.Greater(t, int64(result.RequeueAfter), int64(0))
}

// ---- ModelOnboarding.Reconcile integration ---------------------------------

// TestModelOnboarding_Reconcile_NotFound returns nil when CR is missing.
func TestModelOnboarding_Reconcile_NotFound(t *testing.T) {
	scheme := newControllerScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ModelOnboardingReconciler{Client: cl, Scheme: scheme}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "missing", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestModelOnboarding_Reconcile_TerminalCompleted returns immediately for Completed phase.
func TestModelOnboarding_Reconcile_TerminalCompleted(t *testing.T) {
	scheme := newControllerScheme(t)

	ob := &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "done",
			Namespace:  "default",
			Finalizers: []string{api.FinalizerName},
		},
		Spec: servingv1alpha2.ModelOnboardingSpec{ModelRef: "my-model"},
		Status: servingv1alpha2.ModelOnboardingStatus{
			Phase: phaseCompleted,
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ob).
		WithStatusSubresource(ob).
		Build()
	r := &ModelOnboardingReconciler{Client: cl, Scheme: scheme}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "done", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestModelOnboarding_Reconcile_Deletion removes finalizer on deletion.
func TestModelOnboarding_Reconcile_Deletion(t *testing.T) {
	scheme := newControllerScheme(t)

	ob := &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "deleting",
			Namespace:  "default",
			Finalizers: []string{api.FinalizerName},
		},
		Spec: servingv1alpha2.ModelOnboardingSpec{ModelRef: "my-model"},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ob).
		WithStatusSubresource(ob).
		Build()

	// Trigger deletion.
	require.NoError(t, cl.Delete(context.Background(), ob))

	r := &ModelOnboardingReconciler{Client: cl, Scheme: scheme}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "deleting", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestModelOnboarding_Reconcile_ModelNotFound transitions to Failed when LLMSvc missing.
func TestModelOnboarding_Reconcile_ModelNotFound(t *testing.T) {
	scheme := newControllerScheme(t)

	ob := &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "no-model",
			Namespace:  "default",
			Finalizers: []string{api.FinalizerName},
		},
		Spec: servingv1alpha2.ModelOnboardingSpec{ModelRef: "missing-model"},
		Status: servingv1alpha2.ModelOnboardingStatus{
			Phase: phaseInProgress,
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ob).
		WithStatusSubresource(ob).
		Build()
	r := &ModelOnboardingReconciler{Client: cl, Scheme: scheme}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "no-model", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Fetch fresh copy — status patched server-side.
	var updated servingv1alpha2.ModelOnboarding
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "no-model", Namespace: "default",
	}, &updated))
	assert.Equal(t, phaseFailed, updated.Status.Phase)
}

// TestModelOnboarding_Reconcile_RollbackOnFailure triggers rollback when configured.
func TestModelOnboarding_Reconcile_RollbackOnFailure(t *testing.T) {
	scheme := newControllerScheme(t)

	ob := &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "rollback-me",
			Namespace:  "default",
			Finalizers: []string{api.FinalizerName},
		},
		Spec: servingv1alpha2.ModelOnboardingSpec{
			ModelRef:          "my-model",
			RollbackOnFailure: true,
			Stages: []servingv1alpha2.OnboardingStage{
				{Name: "validate", Type: stageTypeValidation}, // will fail — LLMSvc not ready
			},
		},
		Status: servingv1alpha2.ModelOnboardingStatus{
			Phase: phaseInProgress,
		},
	}

	// Not-ready LLMSvc → validation fails → rollback triggered.
	notReadySvc := stageNotReadyLLMSvc("my-model")

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ob, notReadySvc).
		WithStatusSubresource(ob).
		Build()
	r := &ModelOnboardingReconciler{Client: cl, Scheme: scheme}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "rollback-me", Namespace: "default"},
	})
	require.NoError(t, err)
	// Returns ctrl.Result{} (no requeue on terminal failure).
	assert.Equal(t, ctrl.Result{}, result)

	// Fetch fresh copy — status patched server-side.
	var updated servingv1alpha2.ModelOnboarding
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "rollback-me", Namespace: "default",
	}, &updated))
	assert.Equal(t, phaseRolledBack, updated.Status.Phase)
}
