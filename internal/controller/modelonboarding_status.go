package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

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
	setProgressingCondition(ob, phase, reason, message)
	if isTerminalStatusPhase(phase) {
		setReadyCondition(ob, phase, reason, message)
	}
	if err := r.Status().Patch(ctx, ob, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch ModelOnboarding status: %w", err)
	}
	return ctrl.Result{}, nil
}

func setProgressingCondition(
	ob *servingv1alpha2.ModelOnboarding,
	phase, reason, message string,
) {
	meta.SetStatusCondition(&ob.Status.Conditions, metav1.Condition{
		Type: "Progressing", Status: progressingConditionStatus(phase), Reason: reason,
		Message: message, ObservedGeneration: ob.Generation,
	})
}

func setReadyCondition(
	ob *servingv1alpha2.ModelOnboarding,
	phase, reason, message string,
) {
	meta.SetStatusCondition(&ob.Status.Conditions, metav1.Condition{
		Type: "Ready", Status: boolToConditionStatus(phase == phaseCompleted), Reason: reason,
		Message: message, ObservedGeneration: ob.Generation,
	})
}

func isTerminalStatusPhase(phase string) bool {
	return phase == phaseCompleted || phase == phaseRolledBack || phase == phaseFailed
}

func progressingConditionStatus(phase string) metav1.ConditionStatus {
	switch phase {
	case phaseInProgress, phasePending:
		return metav1.ConditionTrue
	default:
		return metav1.ConditionFalse
	}
}
