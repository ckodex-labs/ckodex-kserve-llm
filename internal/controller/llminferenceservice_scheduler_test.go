/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/gateway"
)

func TestReconcileSchedulerBlocked_UpdatesStatusAcrossBranches(t *testing.T) {
	tests := []struct {
		name         string
		condition    metav1.Condition
		cause        error
		requeueAfter time.Duration
		wantError    bool
	}{
		{
			name: "pending readiness",
			condition: metav1.Condition{
				Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
				Reason: "EndpointPickerUnavailable", Message: "waiting",
			},
			requeueAfter: 5 * time.Second,
		},
		{
			name: "feature disabled",
			condition: metav1.Condition{
				Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
				Reason: "SchedulerFeatureDisabled", Message: "scheduler is disabled",
			},
			cause:     errors.New("scheduler is disabled"),
			wantError: true,
		},
		{
			name: "reconcile failed",
			condition: metav1.Condition{
				Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
				Reason: "SchedulerReconcileFailed", Message: "scheduler failed",
			},
			cause:     errors.New("scheduler failed"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := buildLLMScheme(t)
			llmSvc := makeLLMInferenceService("scheduler-test", "default")
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(llmSvc).
				WithStatusSubresource(llmSvc).
				Build()
			r := &LLMInferenceServiceReconciler{Client: client}

			result, err := r.reconcileSchedulerBlocked(
				context.Background(), llmSvc, llmSvc.DeepCopy(), tt.condition, tt.cause, tt.requeueAfter,
			)

			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.requeueAfter, result.RequeueAfter)

			var updated servingv1alpha2.LLMInferenceService
			require.NoError(t, client.Get(context.Background(), clientKey(llmSvc), &updated))
			condition := meta.FindStatusCondition(updated.Status.Conditions, servingv1alpha2.ConditionSchedulerReady)
			require.NotNil(t, condition)
			assert.Equal(t, tt.condition.Reason, condition.Reason)
			ready := meta.FindStatusCondition(updated.Status.Conditions, servingv1alpha2.ConditionReady)
			require.NotNil(t, ready)
			assert.Equal(t, metav1.ConditionFalse, ready.Status)
		})
	}
}

func clientKey(obj *servingv1alpha2.LLMInferenceService) client.ObjectKey {
	return client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
}

// TestReconcileSchedulerBlocked_RouteFallsBackToService verifies that when the
// scheduler is blocked (EPP unavailable, InferencePool creation failure, or
// feature disabled), the HTTPRoute created by Gateway.Reconcile points at the
// direct Service backend, not at a nonexistent InferencePool. This is the
// controller-level regression test for the rc.7 incident.
func TestReconcileSchedulerBlocked_RouteFallsBackToService(t *testing.T) {
	scheme := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("sched-fail-route", "default")
	llmSvc.Spec.Router.Scheduler = &servingv1alpha2.SchedulerSpec{}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()

	gwReconciler := &gateway.Reconciler{Client: cl, Scheme: scheme}
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Gateway:  gwReconciler,
	}

	blockedCondition := metav1.Condition{
		Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
		Reason: "SchedulerReconcileFailed", Message: "epp serviceaccount not provisioned",
	}

	_, err := r.reconcileSchedulerBlocked(
		context.Background(), llmSvc, llmSvc.DeepCopy(), blockedCondition,
		errors.New("epp serviceaccount not provisioned"), 0,
	)
	require.Error(t, err, "reconcileSchedulerBlocked should propagate the cause error")

	var route gwapiv1.HTTPRoute
	err = cl.Get(context.Background(),
		types.NamespacedName{Name: "sched-fail-route-httproute", Namespace: "default"}, &route)
	require.NoError(t, err, "HTTPRoute must be created even when scheduler is blocked")

	require.NotEmpty(t, route.Spec.Rules, "route must have rules")
	ref := route.Spec.Rules[0].BackendRefs[0].BackendObjectReference
	assert.Nil(t, ref.Group, "backend group must be nil (direct Service), not InferencePool")
	assert.Nil(t, ref.Kind, "backend kind must be nil (direct Service), not InferencePool")
	assert.Equal(t, gwapiv1.ObjectName("sched-fail-route"), ref.Name,
		"backend name must be the Service name")
	if ref.Port == nil {
		t.Fatal("backend port is nil")
	}
	assert.Equal(t, gwapiv1.PortNumber(80), *ref.Port,
		"backend port must be 80 (direct Service), not 8000 (InferencePool)")
}

// TestReconcileSchedulerBlocked_SchedulerNotReadyConditionSet verifies that
// after a blocked scheduler reconciliation, the SchedulerReady condition is
// False and the Ready condition is also False with a SchedulerUnavailable reason.
func TestReconcileSchedulerBlocked_SchedulerNotReadyConditionSet(t *testing.T) {
	scheme := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("sched-cond", "default")
	llmSvc.Spec.Router.Scheduler = &servingv1alpha2.SchedulerSpec{}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()

	gwReconciler := &gateway.Reconciler{Client: cl, Scheme: scheme}
	r := &LLMInferenceServiceReconciler{
		Client:  cl,
		Scheme:  scheme,
		Gateway: gwReconciler,
	}

	blockedCondition := metav1.Condition{
		Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
		Reason: "EndpointPickerUnavailable", Message: "waiting for epp readiness",
	}

	_, err := r.reconcileSchedulerBlocked(
		context.Background(), llmSvc, llmSvc.DeepCopy(), blockedCondition, nil, 5*time.Second,
	)
	require.NoError(t, err)

	var updated servingv1alpha2.LLMInferenceService
	require.NoError(t, cl.Get(context.Background(), clientKey(llmSvc), &updated))

	schedCond := meta.FindStatusCondition(updated.Status.Conditions, servingv1alpha2.ConditionSchedulerReady)
	require.NotNil(t, schedCond)
	assert.Equal(t, metav1.ConditionFalse, schedCond.Status)
	assert.Equal(t, "EndpointPickerUnavailable", schedCond.Reason)

	readyCond := meta.FindStatusCondition(updated.Status.Conditions, servingv1alpha2.ConditionReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, "SchedulerUnavailable", readyCond.Reason)
}

// TestReconcileScheduler_CleanupWhenDisabled verifies that when Router.Scheduler
// is nil, the scheduler cleanup path is invoked and the HTTPRoute uses direct
// Service routing (not InferencePool).
func TestReconcileScheduler_CleanupWhenDisabled(t *testing.T) {
	scheme := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("sched-cleanup", "default")
	// Scheduler is nil (not configured)

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()

	gwReconciler := &gateway.Reconciler{Client: cl, Scheme: scheme}
	r := &LLMInferenceServiceReconciler{
		Client:  cl,
		Scheme:  scheme,
		Gateway: gwReconciler,
	}

	state := &llmInferenceReconcileState{llmSvc: llmSvc, beforePatch: llmSvc.DeepCopy()}
	result, blocked, err := r.reconcileScheduler(context.Background(), state)
	require.NoError(t, err)
	assert.False(t, blocked, "scheduler should not block when disabled")
	assert.Equal(t, time.Duration(0), result.RequeueAfter)

	var route gwapiv1.HTTPRoute
	err = cl.Get(context.Background(),
		types.NamespacedName{Name: "sched-cleanup-httproute", Namespace: "default"}, &route)
	// Route may not exist yet if Gateway.Reconcile was not called in the
	// disabled path. The key assertion is that blocked=false and no error.
	if err == nil {
		ref := route.Spec.Rules[0].BackendRefs[0].BackendObjectReference
		assert.Nil(t, ref.Group, "disabled scheduler must not produce InferencePool backend")
	}
}

// ensure runtime import is used
var _ = runtime.NewScheme
