package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

func (r *ModelOnboardingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ob, err := r.loadOnboarding(ctx, req)
	if err != nil || ob == nil {
		return ctrl.Result{}, err
	}
	if result, done, err := r.handleOnboardingDeletion(ctx, ob); done || err != nil {
		return result, err
	}
	if err := r.ensureOnboardingFinalizer(ctx, ob); err != nil {
		return ctrl.Result{}, err
	}
	if isTerminalOnboardingPhase(ob.Status.Phase) {
		return ctrl.Result{}, nil
	}
	if ob.Status.Phase == "" {
		return r.transition(ctx, ob, phasePending, "", "Initialised", "Pipeline initialised, awaiting first stage")
	}
	return r.reconcileOnboardingPipeline(ctx, ob)
}

func (r *ModelOnboardingReconciler) loadOnboarding(
	ctx context.Context,
	req ctrl.Request,
) (*servingv1alpha2.ModelOnboarding, error) {
	var ob servingv1alpha2.ModelOnboarding
	if err := r.Get(ctx, req.NamespacedName, &ob); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get ModelOnboarding: %w", err)
	}
	return &ob, nil
}

func (r *ModelOnboardingReconciler) handleOnboardingDeletion(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
) (ctrl.Result, bool, error) {
	if ob.DeletionTimestamp == nil {
		return ctrl.Result{}, false, nil
	}
	if controllerutil.ContainsFinalizer(ob, api.FinalizerName) {
		controllerutil.RemoveFinalizer(ob, api.FinalizerName)
		if err := r.Update(ctx, ob); err != nil {
			return ctrl.Result{}, true, fmt.Errorf("remove finalizer: %w", err)
		}
	}
	return ctrl.Result{}, true, nil
}

func (r *ModelOnboardingReconciler) ensureOnboardingFinalizer(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
) error {
	if controllerutil.ContainsFinalizer(ob, api.FinalizerName) {
		return nil
	}
	controllerutil.AddFinalizer(ob, api.FinalizerName)
	if err := r.Update(ctx, ob); err != nil {
		return fmt.Errorf("add finalizer: %w", err)
	}
	return nil
}

func isTerminalOnboardingPhase(phase string) bool {
	return phase == phaseCompleted || phase == phaseRolledBack
}

func (r *ModelOnboardingReconciler) reconcileOnboardingPipeline(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
) (ctrl.Result, error) {
	llmSvc, err := r.loadReferencedInferenceService(ctx, ob)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.transition(ctx, ob, phaseFailed, "", "ModelNotFound",
				fmt.Sprintf("LLMInferenceService %q not found", ob.Spec.ModelRef))
		}
		return ctrl.Result{}, err
	}
	result, err := r.advancePipeline(ctx, ob, llmSvc)
	if err != nil {
		return r.handlePipelineFailure(ctx, ob, err)
	}
	log.FromContext(ctx).Info("pipeline advanced", "name", ob.Name,
		"phase", ob.Status.Phase, "stage", ob.Status.CurrentStage)
	return result, nil
}

func (r *ModelOnboardingReconciler) loadReferencedInferenceService(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
) (*servingv1alpha2.LLMInferenceService, error) {
	var llmSvc servingv1alpha2.LLMInferenceService
	key := types.NamespacedName{Name: ob.Spec.ModelRef, Namespace: ob.Namespace}
	if err := r.Get(ctx, key, &llmSvc); err != nil {
		return nil, fmt.Errorf("get LLMInferenceService: %w", err)
	}
	return &llmSvc, nil
}

func (r *ModelOnboardingReconciler) handlePipelineFailure(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
	pipelineErr error,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	phase, reason := phaseFailed, "StageFailed"
	message := fmt.Sprintf("stage %q failed: %v", ob.Status.CurrentStage, pipelineErr)
	if ob.Spec.RollbackOnFailure {
		phase, reason = phaseRolledBack, "RollbackTriggered"
		message = fmt.Sprintf("rolling back after failure in stage %q: %v", ob.Status.CurrentStage, pipelineErr)
	}
	if _, err := r.transition(ctx, ob, phase, ob.Status.CurrentStage, reason, message); err != nil {
		logger.Error(err, "failed to set terminal failure status", "phase", phase)
	}
	return ctrl.Result{}, nil
}
