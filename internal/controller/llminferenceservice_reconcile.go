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

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/gateway"
	domainvalidation "github.com/ckodex-labs/kserve-llm-operator/internal/validation"
)

type llmInferenceReconcileState struct {
	llmSvc      *servingv1alpha2.LLMInferenceService
	beforePatch *servingv1alpha2.LLMInferenceService
	activeLoras []servingv1alpha2.LLMLoraAdapter
	activePacks []servingv1alpha2.AIPack
	multiNode   bool
}

// AnnotationPauseReconciliation marks an LLMInferenceService as managed
// out-of-band: the operator skips every child-resource reconciliation for it
// (Deployment, Gateway, HTTPRoute, scheduler) so a live workload tuned via the
// CR template is never reverted or rolled by a reconcile. Removing the
// annotation resumes ordinary reconciliation.
const AnnotationPauseReconciliation = "ckodex.com/pause-reconciliation"

// reconciliationPaused reports whether the pause annotation is set to "true".
func reconciliationPaused(llmSvc *servingv1alpha2.LLMInferenceService) bool {
	if llmSvc == nil {
		return false
	}
	return llmSvc.Annotations[AnnotationPauseReconciliation] == "true"
}

func newLLMInferenceReconcileState(llmSvc *servingv1alpha2.LLMInferenceService) *llmInferenceReconcileState {
	return &llmInferenceReconcileState{llmSvc: llmSvc, beforePatch: llmSvc.DeepCopy()}
}

func schedulerNeedsEarlyFence(service *servingv1alpha2.LLMInferenceService, gatewayEnabled, schedulerEnabled bool) bool {
	return service.Spec.Router.Scheduler != nil &&
		(!gatewayEnabled || !schedulerEnabled || !gateway.SchedulerReady(service) || service.Spec.Canary != nil)
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
	// Out-of-band management escape hatch: a paused service is left entirely
	// alone. This must run before any child-resource reconcile — rendering the
	// Deployment for a paused CR would roll a live workload with re-rendered
	// args (observed live 2026-09-01: a paused Qwen service was rolled by the
	// well-known preset merge and crashed).
	if reconciliationPaused(llmSvc) {
		logger.Info("reconciliation paused by annotation, skipping",
			"name", llmSvc.Name, "namespace", llmSvc.Namespace,
			"annotation", AnnotationPauseReconciliation)
		return ctrl.Result{}, nil
	}
	if llmSvc.Spec.Router.Scheduler != nil {
		defer func() {
			if retErr != nil {
				fenceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				retErr = errors.Join(retErr, r.fenceSchedulerRoutes(fenceCtx, llmSvc))
			}
		}()
		if schedulerNeedsEarlyFence(llmSvc, r.Gateway != nil, r.Scheduler != nil) {
			fenceErr := r.fenceSchedulerRoutes(ctx, llmSvc)
			if r.Gateway == nil {
				return r.rejectSchedulerConfiguration(ctx, llmSvc, "GatewayFeatureDisabled", "scheduler is requested but the gateway feature is disabled", fenceErr)
			}
			if llmSvc.Spec.Canary != nil {
				return r.rejectSchedulerConfiguration(ctx, llmSvc, "SchedulerCanaryUnsupported", "spec.router.scheduler cannot be combined with spec.canary until both InferencePools are explicitly modeled", fenceErr)
			}
			if err := fenceErr; err != nil {
				return ctrl.Result{}, fmt.Errorf("fence scheduler routes before reconciliation: %w", err)
			}
		}
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

func (r *LLMInferenceServiceReconciler) fenceSchedulerRoutes(ctx context.Context, service *servingv1alpha2.LLMInferenceService) error {
	if err := gateway.FenceOwnedRoutes(ctx, r.APIReader, r.Client, service); err != nil {
		return fmt.Errorf("fence scheduler routes: %w", err)
	}
	return nil
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
