/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	domainvalidation "github.com/ckodex-labs/kserve-llm-operator/internal/validation"
)

type llmInferenceReconcileState struct {
	llmSvc      *servingv1alpha2.LLMInferenceService
	beforePatch *servingv1alpha2.LLMInferenceService
	activeLoras []servingv1alpha2.LLMLoraAdapter
	activePacks []servingv1alpha2.AIPack
	multiNode   bool
}

func newLLMInferenceReconcileState(llmSvc *servingv1alpha2.LLMInferenceService) *llmInferenceReconcileState {
	return &llmInferenceReconcileState{llmSvc: llmSvc, beforePatch: llmSvc.DeepCopy()}
}

// Reconcile implements the main reconcile loop.
func (r *LLMInferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	logger := log.FromContext(ctx)
	reconcileStart := time.Now()
	modelName := ""
	defer func() {
		if r.Inst != nil && modelName != "" {
			r.Inst.RecordReconcile(ctx, modelName, time.Since(reconcileStart).Seconds(), retErr == nil)
		}
	}()

	llmSvc, found, err := r.fetchLLMInferenceService(ctx, req, logger)
	if err != nil || !found {
		return ctrl.Result{}, err
	}
	if err := domainvalidation.ValidateInferenceEngine(llmSvc.Spec.Engine); err != nil {
		return ctrl.Result{}, err
	}
	if err := domainvalidation.ValidateLLMInferenceServiceSurface(llmSvc).ToAggregate(); err != nil {
		return ctrl.Result{}, fmt.Errorf("validate LLMInferenceService surface: %w", err)
	}
	modelName = llmSvc.Spec.Model.Name
	state := newLLMInferenceReconcileState(llmSvc)
	deleted, err := r.reconcileResourceSetup(ctx, state)
	if err != nil || deleted {
		return ctrl.Result{}, err
	}
	if err := r.reconcileExternalInputs(ctx, state); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.prepareWorkloadInputs(ctx, state); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileWorkload(ctx, state); err != nil {
		return ctrl.Result{}, err
	}
	result, complete, err := r.reconcileScheduler(ctx, state)
	if complete {
		return result, err
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.reconcilePostScheduler(ctx, state, logger)
}

func (r *LLMInferenceServiceReconciler) reconcilePostScheduler(ctx context.Context, state *llmInferenceReconcileState, logger logr.Logger) (ctrl.Result, error) {
	if err := r.reconcileRouting(ctx, state); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileSecurityIsolation(ctx, state); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileGovernance(ctx, state, logger); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileIdentityAndPolicy(ctx, state); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.updateLLMInferenceStatus(ctx, state); err != nil {
		return ctrl.Result{}, err
	}
	r.recordLLMInferenceAudit(ctx, state, logger)
	return r.finishLLMInferenceReconcile(state)
}
