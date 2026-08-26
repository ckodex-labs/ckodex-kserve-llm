package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/scheduler"
)

func TestLLMInferenceCoverage_SchedulerFailClosedBranches(t *testing.T) {
	s := buildLLMScheme(t)
	svc := makeLLMInferenceService("scheduler", "default")
	state := newLLMInferenceReconcileState(svc)
	r := setupReconciler(fake.NewClientBuilder().WithScheme(s).WithObjects(svc).WithStatusSubresource(svc).Build(), s)

	result, complete, err := r.reconcileScheduler(context.Background(), state)
	require.False(t, complete)
	require.NoError(t, err)
	require.Zero(t, result)

	svc.Spec.Router.Scheduler = &servingv1alpha2.SchedulerSpec{}
	result, complete, err = r.reconcileScheduler(context.Background(), newLLMInferenceReconcileState(svc))
	require.True(t, complete)
	require.ErrorContains(t, err, "scheduler is requested")
	require.Zero(t, result)

	// The concrete scheduler with no configured sub-reconcilers fails closed.
	svc.Spec.Router.Scheduler = &servingv1alpha2.SchedulerSpec{}
	r.Scheduler = &scheduler.Reconciler{}
	result, complete, err = r.reconcileScheduler(context.Background(), newLLMInferenceReconcileState(svc))
	require.True(t, complete)
	require.ErrorContains(t, err, "reconcile scheduler")
	require.Zero(t, result)

	state = newLLMInferenceReconcileState(svc)
	result, complete, err = r.schedulerNotReady(context.Background(), state)
	require.True(t, complete)
	require.NoError(t, err)
	require.Equal(t, 5*time.Second, result.RequeueAfter)

	state = newLLMInferenceReconcileState(svc)
	result, complete, err = r.schedulerReconcileFailed(context.Background(), state, errors.New("epp unavailable"))
	require.True(t, complete)
	require.ErrorContains(t, err, "epp unavailable")
	require.Zero(t, result)

	condition := schedulerCondition(svc, metav1.ConditionTrue, "ready", "ok")
	require.Equal(t, int64(0), condition.ObservedGeneration)
	setSchedulerGateReadyCondition(svc, "blocked")
}

func TestLLMInferenceCoverage_PrepareFetchAndLoras(t *testing.T) {
	s := buildLLMScheme(t)
	svc := makeLLMInferenceService("svc", "default")
	base := fake.NewClientBuilder().WithScheme(s).WithObjects(svc,
		&servingv1alpha2.LLMLoraAdapter{ObjectMeta: metav1.ObjectMeta{Name: "active", Namespace: "default"}, Spec: servingv1alpha2.LLMLoraAdapterSpec{TargetService: "svc"}},
		&servingv1alpha2.LLMLoraAdapter{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default"}, Spec: servingv1alpha2.LLMLoraAdapterSpec{TargetService: "other"}},
	).Build()
	r := setupReconciler(base, s)
	found, ok, err := r.fetchLLMInferenceService(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "svc", Namespace: "default"}}, ctrl.LoggerFrom(context.Background()))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, svc.Name, found.Name)
	_, ok, err = r.fetchLLMInferenceService(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"}}, ctrl.LoggerFrom(context.Background()))
	require.NoError(t, err)
	require.False(t, ok)

	state := newLLMInferenceReconcileState(svc)
	require.NoError(t, r.prepareWorkloadInputs(context.Background(), state))
	require.Len(t, state.activeLoras, 1)

	r.Client = &coverageClient{Client: base, listErr: errors.New("list failed")}
	require.Empty(t, r.listActiveLoras(context.Background(), svc))
	r.Client = &coverageClient{Client: base, getErr: errors.New("fetch failed")}
	_, ok, err = r.fetchLLMInferenceService(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "svc", Namespace: "default"}}, ctrl.LoggerFrom(context.Background()))
	require.False(t, ok)
	require.ErrorContains(t, err, "fetch LLMInferenceService")
}
