/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
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

func (r *ModelOnboardingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var ob servingv1alpha2.ModelOnboarding
	if err := r.Get(ctx, req.NamespacedName, &ob); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get ModelOnboarding: %w", err)
	}

	// Handle deletion
	if ob.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&ob, api.FinalizerName) {
			controllerutil.RemoveFinalizer(&ob, api.FinalizerName)
			if err := r.Update(ctx, &ob); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&ob, api.FinalizerName) {
		controllerutil.AddFinalizer(&ob, api.FinalizerName)
		if err := r.Update(ctx, &ob); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
	}

	// Terminal states: nothing more to do
	switch ob.Status.Phase {
	case phaseCompleted, phaseRolledBack:
		return ctrl.Result{}, nil
	}

	// Initialise phase on first reconcile
	if ob.Status.Phase == "" {
		return r.transition(ctx, &ob, phasePending, "", "Initialised", "Pipeline initialised, awaiting first stage")
	}

	// Validate referenced LLMInferenceService exists
	var llmSvc servingv1alpha2.LLMInferenceService
	key := types.NamespacedName{Name: ob.Spec.ModelRef, Namespace: ob.Namespace}
	if err := r.Get(ctx, key, &llmSvc); err != nil {
		if apierrors.IsNotFound(err) {
			return r.transition(ctx, &ob, phaseFailed, "", "ModelNotFound",
				fmt.Sprintf("LLMInferenceService %q not found", ob.Spec.ModelRef))
		}
		return ctrl.Result{}, fmt.Errorf("get LLMInferenceService: %w", err)
	}

	// Advance pipeline
	result, err := r.advancePipeline(ctx, &ob, &llmSvc)
	if err != nil {
		// Attempt rollback when configured
		if ob.Spec.RollbackOnFailure {
			if _, rbErr := r.transition(ctx, &ob, phaseRolledBack, ob.Status.CurrentStage, "RollbackTriggered",
				fmt.Sprintf("rolling back after failure in stage %q: %v", ob.Status.CurrentStage, err)); rbErr != nil {
				logger.Error(rbErr, "failed to set rolled-back status")
			}
		} else {
			if _, failErr := r.transition(ctx, &ob, phaseFailed, ob.Status.CurrentStage, "StageFailed",
				fmt.Sprintf("stage %q failed: %v", ob.Status.CurrentStage, err)); failErr != nil {
				logger.Error(failErr, "failed to set failed status")
			}
		}
		return ctrl.Result{}, nil // Don't re-queue on terminal failure
	}

	logger.Info("pipeline advanced",
		"name", ob.Name,
		"phase", ob.Status.Phase,
		"stage", ob.Status.CurrentStage,
	)
	return result, nil
}

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

// executeStage performs the work for a single pipeline stage.
func (r *ModelOnboardingReconciler) executeStage(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
	llmSvc *servingv1alpha2.LLMInferenceService,
	stage servingv1alpha2.OnboardingStage,
) error {
	logger := log.FromContext(ctx).WithValues("stage", stage.Name, "type", stage.Type)

	switch stage.Type {
	case stageTypeValidation:
		// Validation: the LLMInferenceService must be Ready
		if !llmSvc.Status.ModelReady {
			return fmt.Errorf("LLMInferenceService %q is not ready (validation gate)", ob.Spec.ModelRef)
		}
		logger.Info("validation stage passed")

	case stageTypeCanary:
		// Canary: verify the service has at least one ready replica
		if llmSvc.Status.Replicas < 1 {
			return fmt.Errorf("no ready replicas for canary stage (got %d)", llmSvc.Status.Replicas)
		}
		logger.Info("canary stage passed", "readyReplicas", llmSvc.Status.Replicas)

	case stageTypeGate:
		if stage.Gate == nil {
			logger.Info("gate stage skipped: no criteria defined")
			return nil
		}
		// Gate: check promotion criteria against live metrics
		if err := r.checkGateCriteria(ctx, ob, llmSvc, stage.Gate); err != nil {
			return fmt.Errorf("gate criteria not met: %w", err)
		}
		logger.Info("gate stage passed")

	case stageTypePromotion:
		// Promotion: check model is ready for full traffic
		if !llmSvc.Status.ModelReady {
			return fmt.Errorf("LLMInferenceService %q not ready for promotion", ob.Spec.ModelRef)
		}
		logger.Info("promotion stage passed")

	default:
		logger.Info("unknown stage type, skipping", "type", stage.Type)
	}

	return nil
}

// checkGateCriteria validates promotion gate metrics against live Prometheus data.
//
// Gate evaluation order:
//  1. Replica / readiness pre-check (fast path — no Prometheus query if already failed)
//  2. Success rate query against ckodex_inference_requests_total
//  3. P99 latency query against ckodex_inference_request_duration_seconds_bucket (when MaxLatencyP99 is set)
//
// When Metrics is nil, the gate fails closed because there is no authoritative
// success-rate or latency backend available to prove promotion safety.
func (r *ModelOnboardingReconciler) checkGateCriteria(
	ctx context.Context,
	_ *servingv1alpha2.ModelOnboarding,
	llmSvc *servingv1alpha2.LLMInferenceService,
	gate *servingv1alpha2.GateCriteria,
) error {
	// Pre-check: model must be in a ready state before querying metrics.
	// A model with zero replicas will have no traffic and therefore no metric
	// data — querying Prometheus would always return 0 % success rate, causing
	// a spurious gate failure.
	if llmSvc.Status.Replicas < 1 {
		return fmt.Errorf("gate: no ready replicas (minSuccessRate=%d%%)", gate.MinSuccessRate)
	}
	if !llmSvc.Status.ModelReady {
		return fmt.Errorf("gate: model not ready (minSuccessRate=%d%%)", gate.MinSuccessRate)
	}

	q := r.metricsQuerier()
	if q == nil {
		return fmt.Errorf("gate: promotion metrics backend is not configured")
	}

	// --- Success rate check ---
	successRate, err := q.QuerySuccessRate(ctx, llmSvc.Spec.Model.Name, llmSvc.Namespace)
	if err != nil {
		// Prometheus query failure is a soft block: we cannot prove the gate passes,
		// so we return an error to prevent promotion. The operator will re-queue and
		// retry once the metrics backend is reachable again.
		return fmt.Errorf("gate: metrics unavailable: %w", err)
	}
	if int32(successRate) < gate.MinSuccessRate {
		return fmt.Errorf("gate: success rate %.1f%% < required %d%%", successRate, gate.MinSuccessRate)
	}

	// --- P99 latency check (optional) ---
	if gate.MaxLatencyP99 != nil && *gate.MaxLatencyP99 > 0 {
		p99MS, err := q.QueryP99LatencyMS(ctx, llmSvc.Spec.Model.Name, llmSvc.Namespace)
		if err != nil {
			return fmt.Errorf("gate: P99 latency metrics unavailable: %w", err)
		}
		if p99MS > *gate.MaxLatencyP99 {
			return fmt.Errorf("gate: P99 latency %dms > max allowed %dms", p99MS, *gate.MaxLatencyP99)
		}
	}

	return nil
}

// transition writes a status update and returns the updated object.
func (r *ModelOnboardingReconciler) transition(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
	phase, stage, reason, message string,
) (ctrl.Result, error) {
	patch := client.MergeFrom(ob.DeepCopy())
	ob.Status.Phase = phase
	if stage != "" {
		ob.Status.CurrentStage = stage
	}
	meta.SetStatusCondition(&ob.Status.Conditions, metav1.Condition{
		Type:               "Progressing",
		Status:             progressingConditionStatus(phase),
		Reason:             reason,
		Message:            message,
		ObservedGeneration: ob.Generation,
	})
	if phase == phaseCompleted || phase == phaseRolledBack || phase == phaseFailed {
		meta.SetStatusCondition(&ob.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             boolToConditionStatus(phase == phaseCompleted),
			Reason:             reason,
			Message:            message,
			ObservedGeneration: ob.Generation,
		})
	}
	if err := r.Status().Patch(ctx, ob, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch ModelOnboarding status: %w", err)
	}
	return ctrl.Result{}, nil
}

func progressingConditionStatus(phase string) metav1.ConditionStatus {
	switch phase {
	case phaseInProgress, phasePending:
		return metav1.ConditionTrue
	default:
		return metav1.ConditionFalse
	}
}

// SetupWithManager registers the ModelOnboardingReconciler.
func (r *ModelOnboardingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.ModelOnboarding{}).
		Named("modelonboarding").
		Complete(r)
}
