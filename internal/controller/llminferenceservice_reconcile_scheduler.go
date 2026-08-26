/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (r *LLMInferenceServiceReconciler) reconcileScheduler(ctx context.Context, state *llmInferenceReconcileState) (ctrl.Result, bool, error) {
	llmSvc := state.llmSvc
	if llmSvc.Spec.Router.Scheduler == nil {
		meta.RemoveStatusCondition(&llmSvc.Status.Conditions, servingv1alpha2.ConditionSchedulerReady)
		if r.Scheduler != nil {
			if err := r.Scheduler.Cleanup(ctx, llmSvc); err != nil {
				return ctrl.Result{}, true, fmt.Errorf("cleanup disabled scheduler: %w", err)
			}
		}
		return ctrl.Result{}, false, nil
	}
	if r.Scheduler == nil {
		return r.schedulerFeatureDisabled(ctx, state)
	}
	return r.reconcileEnabledScheduler(ctx, state)
}

func (r *LLMInferenceServiceReconciler) schedulerFeatureDisabled(ctx context.Context, state *llmInferenceReconcileState) (ctrl.Result, bool, error) {
	err := fmt.Errorf("scheduler is requested but the scheduler feature is disabled")
	condition := schedulerCondition(state.llmSvc, metav1.ConditionFalse, "SchedulerFeatureDisabled", err.Error())
	meta.SetStatusCondition(&state.llmSvc.Status.Conditions, condition)
	setSchedulerGateReadyCondition(state.llmSvc, condition.Message)
	if r.Gateway != nil {
		err = errors.Join(err, r.Gateway.Reconcile(ctx, state.llmSvc))
	}
	err = errors.Join(err, r.Status().Patch(ctx, state.llmSvc, client.MergeFrom(state.beforePatch)))
	return ctrl.Result{}, true, err
}

func (r *LLMInferenceServiceReconciler) reconcileEnabledScheduler(ctx context.Context, state *llmInferenceReconcileState) (ctrl.Result, bool, error) {
	ready, err := r.Scheduler.Reconcile(ctx, state.llmSvc)
	if err != nil {
		return r.schedulerReconcileFailed(ctx, state, err)
	}
	if !ready {
		return r.schedulerNotReady(ctx, state)
	}
	condition := schedulerCondition(state.llmSvc, metav1.ConditionTrue, "EndpointPickerReady", "GA InferencePool and EPP are ready")
	meta.SetStatusCondition(&state.llmSvc.Status.Conditions, condition)
	return ctrl.Result{}, false, nil
}

func (r *LLMInferenceServiceReconciler) schedulerReconcileFailed(ctx context.Context, state *llmInferenceReconcileState, reconcileErr error) (ctrl.Result, bool, error) {
	condition := schedulerCondition(state.llmSvc, metav1.ConditionFalse, "SchedulerReconcileFailed", reconcileErr.Error())
	meta.SetStatusCondition(&state.llmSvc.Status.Conditions, condition)
	setSchedulerGateReadyCondition(state.llmSvc, condition.Message)
	if r.Gateway != nil {
		reconcileErr = errors.Join(reconcileErr, r.Gateway.Reconcile(ctx, state.llmSvc))
	}
	reconcileErr = errors.Join(reconcileErr, r.Status().Patch(ctx, state.llmSvc, client.MergeFrom(state.beforePatch)))
	return ctrl.Result{}, true, fmt.Errorf("reconcile scheduler: %w", reconcileErr)
}

func (r *LLMInferenceServiceReconciler) schedulerNotReady(ctx context.Context, state *llmInferenceReconcileState) (ctrl.Result, bool, error) {
	condition := schedulerCondition(state.llmSvc, metav1.ConditionFalse, "EndpointPickerUnavailable", "Waiting for the GA InferencePool and EPP readiness")
	meta.SetStatusCondition(&state.llmSvc.Status.Conditions, condition)
	setSchedulerGateReadyCondition(state.llmSvc, condition.Message)
	if r.Gateway != nil {
		if err := r.Gateway.Reconcile(ctx, state.llmSvc); err != nil {
			return ctrl.Result{}, true, fmt.Errorf("route scheduler fail-closed backend: %w", err)
		}
	}
	if err := r.Status().Patch(ctx, state.llmSvc, client.MergeFrom(state.beforePatch)); err != nil {
		return ctrl.Result{}, true, fmt.Errorf("update scheduler readiness: %w", err)
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, true, nil
}

func schedulerCondition(llmSvc *servingv1alpha2.LLMInferenceService, status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{Type: servingv1alpha2.ConditionSchedulerReady, Status: status, Reason: reason, Message: message, ObservedGeneration: llmSvc.Generation}
}

func setSchedulerGateReadyCondition(llmSvc *servingv1alpha2.LLMInferenceService, message string) {
	meta.SetStatusCondition(&llmSvc.Status.Conditions, metav1.Condition{
		Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionFalse,
		Reason: "SchedulerUnavailable", Message: message, ObservedGeneration: llmSvc.Generation,
	})
}
