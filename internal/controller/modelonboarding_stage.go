package controller

import (
	"context"
	"fmt"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// executeStage performs the work for a single pipeline stage.
func (r *ModelOnboardingReconciler) executeStage(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
	llmSvc *servingv1alpha2.LLMInferenceService,
	stage servingv1alpha2.OnboardingStage,
) error {
	switch stage.Type {
	case stageTypeValidation:
		return r.executeValidationStage(ctx, ob, llmSvc, stage)
	case stageTypeCanary:
		return r.executeCanaryStage(ctx, llmSvc, stage)
	case stageTypeGate:
		return r.executeGateStage(ctx, ob, llmSvc, stage)
	case stageTypePromotion:
		return r.executePromotionStage(ctx, ob, llmSvc, stage)
	default:
		return fmt.Errorf("unsupported onboarding stage type %q", stage.Type)
	}
}

func (r *ModelOnboardingReconciler) executeValidationStage(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
	llmSvc *servingv1alpha2.LLMInferenceService,
	stage servingv1alpha2.OnboardingStage,
) error {
	if err := ensureOnboardingResidencyReady(llmSvc); err != nil {
		return fmt.Errorf("LLMInferenceService %q is not ready (validation gate): %w", ob.Spec.ModelRef, err)
	}
	log.FromContext(ctx).Info("validation stage passed", "stage", stage.Name, "type", stage.Type)
	return nil
}

func (r *ModelOnboardingReconciler) executeCanaryStage(
	ctx context.Context,
	llmSvc *servingv1alpha2.LLMInferenceService,
	stage servingv1alpha2.OnboardingStage,
) error {
	if llmSvc.Status.Replicas < 1 {
		return fmt.Errorf("no ready replicas for canary stage (got %d)", llmSvc.Status.Replicas)
	}
	if err := ensureOnboardingResidencyReady(llmSvc); err != nil {
		return fmt.Errorf("model is not resident for canary stage: %w", err)
	}
	log.FromContext(ctx).Info("canary stage passed", "stage", stage.Name, "type", stage.Type,
		"readyReplicas", llmSvc.Status.Replicas)
	return nil
}

func (r *ModelOnboardingReconciler) executeGateStage(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
	llmSvc *servingv1alpha2.LLMInferenceService,
	stage servingv1alpha2.OnboardingStage,
) error {
	if stage.Gate == nil {
		log.FromContext(ctx).Info("gate stage skipped: no criteria defined", "stage", stage.Name)
		return nil
	}
	if err := r.checkGateCriteria(ctx, ob, llmSvc, stage.Gate); err != nil {
		return fmt.Errorf("gate criteria not met: %w", err)
	}
	log.FromContext(ctx).Info("gate stage passed", "stage", stage.Name, "type", stage.Type)
	return nil
}

func (r *ModelOnboardingReconciler) executePromotionStage(
	ctx context.Context,
	ob *servingv1alpha2.ModelOnboarding,
	llmSvc *servingv1alpha2.LLMInferenceService,
	stage servingv1alpha2.OnboardingStage,
) error {
	if err := ensureOnboardingResidencyReady(llmSvc); err != nil {
		return fmt.Errorf("LLMInferenceService %q not ready for promotion: %w", ob.Spec.ModelRef, err)
	}
	log.FromContext(ctx).Info("promotion stage passed", "stage", stage.Name, "type", stage.Type)
	return nil
}
