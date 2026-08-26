/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const (
	// phaseTerminal returns true when the pipeline has reached a final state.
	phasePending    = "Pending"
	phaseInProgress = "InProgress"
	phaseCompleted  = "Completed"
	phaseFailed     = "Failed"
	phaseRolledBack = "RolledBack"

	// stageTypeValidation validates the model + LLMInferenceService health.
	stageTypeValidation = "validation"
	// stageTypeCanary runs a weighted canary rollout (10%→50%).
	stageTypeCanary = "canary"
	// stageTypePromotion promotes the canary to full traffic.
	stageTypePromotion = "promotion"
	// stageTypeGate is a policy gate checked against live metrics.
	stageTypeGate = "gate"

	// requeueAfterStageTransition is the wait time between stage advances.
	requeueAfterStageTransition = 2 * time.Second
)

// ModelOnboardingReconciler reconciles ModelOnboarding objects.
// It drives a multi-stage model promotion pipeline:
//
//	Pending → validation → canary → promotion/gate → Completed
//	                                     ↓ (failure + rollbackOnFailure)
//	                               RolledBack
//
// The controller is edge-triggered: each reconcile advances the pipeline
// by exactly one stage transition, then re-queues with a short delay so
// the model can stabilise before the next gate check.
type ModelOnboardingReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Metrics is the Prometheus query backend used by checkGateCriteria.
	// When nil, promotion gates fail closed because no authoritative metrics
	// backend is configured. A dev-only fallback must be injected explicitly.
	Metrics MetricsQuerier
}

// metricsQuerier returns the configured querier.
func (r *ModelOnboardingReconciler) metricsQuerier() MetricsQuerier {
	return r.Metrics
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=modelonboardings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=modelonboardings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=modelonboardings/finalizers,verbs=update
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices,verbs=get;list;watch

// advancePipeline drives the pipeline forward by one stage.
func (r *ModelOnboardingReconciler) advancePipeline(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
	llmSvc *servingv1alpha2.LLMInferenceService,
) (ctrl.Result, error) {
	stages := ob.Spec.Stages
	if len(stages) == 0 {
		// No stages defined: go directly to Completed
		_, err := r.transition(ctx, ob, phaseCompleted, "", "NoStages", "Pipeline completed — no stages configured")
		return ctrl.Result{}, err
	}

	// Find the next pending stage
	nextStageIdx := r.nextStageIndex(ob)
	if nextStageIdx >= len(stages) {
		// All stages passed → Completed
		_, err := r.transition(ctx, ob, phaseCompleted, stages[len(stages)-1].Name, "AllStagesPassed",
			fmt.Sprintf("all %d stages completed successfully", len(stages)))
		return ctrl.Result{}, err
	}

	stage := stages[nextStageIdx]
	if err := r.executeStage(ctx, ob, llmSvc, stage); err != nil {
		return ctrl.Result{}, fmt.Errorf("execute stage %q: %w", stage.Name, err)
	}

	// Mark this stage complete in status and re-queue
	_, err := r.transition(ctx, ob, phaseInProgress, stage.Name, "StageComplete",
		fmt.Sprintf("stage %q completed, advancing pipeline", stage.Name))
	if err != nil {
		return ctrl.Result{}, err
	}

	// Re-queue to process the next stage after a stabilisation window
	return ctrl.Result{RequeueAfter: requeueAfterStageTransition}, nil
}

// nextStageIndex returns the index of the first stage not yet completed.
// A stage is "completed" if its name matches the currentStage that was last written.
func (r *ModelOnboardingReconciler) nextStageIndex(ob *servingv1alpha2.ModelOnboarding) int {
	if ob.Status.CurrentStage == "" || ob.Status.Phase == phasePending {
		return 0
	}
	for i, s := range ob.Spec.Stages {
		if s.Name == ob.Status.CurrentStage {
			return i + 1 // Return the one after the last completed
		}
	}
	return 0
}

// SetupWithManager registers the ModelOnboardingReconciler.
func (r *ModelOnboardingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.ModelOnboarding{}).
		Named("modelonboarding").
		Complete(r)
}
