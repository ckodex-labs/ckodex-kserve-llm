package status

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// Reconciler handles LLMInferenceService status updates.
type Reconciler struct {
	Client client.Client
}

// Update updates the LLMInferenceService status based on the underlying deployment and well-known configs.
func (r *Reconciler) Update(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, llmSvcBeforePatch *servingv1alpha2.LLMInferenceService, isOptimized bool) error {
	// 1. Check Deployment Readiness
	var deploy appsv1.Deployment
	err := r.Client.Get(ctx, types.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}, &deploy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			llmSvc.Status.Replicas = 0
			llmSvc.Status.ModelReady = false
		} else {
			return fmt.Errorf("get deployment for status: %w", err)
		}
	} else {
		llmSvc.Status.Replicas = deploy.Status.ReadyReplicas
		llmSvc.Status.ModelReady = deploy.Status.ReadyReplicas > 0
	}

	llmSvc.Status.ObservedGeneration = llmSvc.Generation

	// 2. Ready Condition
	newStatus := metav1.ConditionFalse
	if llmSvc.Status.ModelReady {
		newStatus = metav1.ConditionTrue
	}

	readyCondition := metav1.Condition{
		Type:               servingv1alpha2.ConditionReady,
		Status:             newStatus,
		ObservedGeneration: llmSvc.Generation,
	}

	if newStatus == metav1.ConditionTrue {
		readyCondition.Reason = "Ready"
		readyCondition.Message = "Model is loaded and serving"
	} else {
		readyCondition.Reason = "NotReady"
		readyCondition.Message = "Waiting for model pods to become ready"
	}

	// Update or add condition, preserving LastTransitionTime if status hasn't changed.
	r.setCondition(&llmSvc.Status.Conditions, readyCondition)

	// 3. Set URL
	llmSvc.Status.URL = fmt.Sprintf("http://%s.%s.svc.cluster.local/v2/models/%s",
		llmSvc.Name, llmSvc.Namespace, llmSvc.Spec.Model.Name)

	// 4. Set optimization status
	llmSvc.Status.Optimized = isOptimized
	optCondition := metav1.Condition{
		Type:               servingv1alpha2.ConditionModelOptimized,
		Status:             metav1.ConditionFalse,
		Reason:             "NotOptimized",
		Message:            "Running with generic defaults",
		ObservedGeneration: llmSvc.Generation,
	}
	if isOptimized {
		optCondition.Status = metav1.ConditionTrue
		optCondition.Reason = "Optimized"
		optCondition.Message = "WellKnown optimizations (e.g. TurboQuant) applied"
	}
	r.setCondition(&llmSvc.Status.Conditions, optCondition)

	// 5. Final CAS-compliant update
	if !equality.Semantic.DeepEqual(&llmSvcBeforePatch.Status, &llmSvc.Status) {
		// Standardize on Update with ResourceVersion (CAS) for high-integrity states.
		// controller-runtime handles the ResourceVersion check during Update.
		err := r.Client.Status().Update(ctx, llmSvc)
		if err != nil {
			if apierrors.IsConflict(err) {
				// Return the error to trigger a requeue and Refetch
				return fmt.Errorf("conflict during status CAS update: %w", err)
			}
			return err
		}
	}
	return nil
}

// SetCondition is a generic helper for setting an ad-hoc condition (e.g. GPUCapacity).
func (r *Reconciler) SetCondition(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, condType string, status metav1.ConditionStatus, reason, message string) error {
	patch := client.MergeFrom(llmSvc.DeepCopy())
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: llmSvc.Generation,
	}
	r.setCondition(&llmSvc.Status.Conditions, condition)
	return r.Client.Status().Patch(ctx, llmSvc, patch)
}

func (r *Reconciler) setCondition(conditions *[]metav1.Condition, newCond metav1.Condition) {
	for i, c := range *conditions {
		if c.Type == newCond.Type {
			if c.Status == newCond.Status && c.Reason == newCond.Reason {
				newCond.LastTransitionTime = c.LastTransitionTime
			} else {
				newCond.LastTransitionTime = metav1.Now()
			}
			(*conditions)[i] = newCond
			return
		}
	}
	newCond.LastTransitionTime = metav1.Now()
	*conditions = append(*conditions, newCond)
}
