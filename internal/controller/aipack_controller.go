/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
)

// AIPackReconciler reconciles AIPack objects.
// It is a pure artifact-catalog controller: it resolves status.family and sets
// the Ready condition. It does NOT own Deployments or Services.
type AIPackReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=aipacks,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=aipacks/status,verbs=get;update;patch

// Reconcile implements the main reconcile loop for AIPack resources.
func (r *AIPackReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pack servingv1alpha2.AIPack
	if err := r.Get(ctx, req.NamespacedName, &pack); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetch AIPack: %w", err)
	}
	packBefore := pack.DeepCopy()

	// Derive family; reject unknown kinds.
	family, ok := aipack.FamilyForKind(pack.Spec.Kind)
	if !ok {
		pack.Status.ObservedGeneration = pack.Generation
		if err := r.setReadyCondition(ctx, &pack, packBefore, metav1.ConditionFalse,
			"InvalidKind",
			fmt.Sprintf("unknown artifact kind %q", pack.Spec.Kind),
		); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("AIPack blocked: unknown kind", "name", pack.Name, "kind", pack.Spec.Kind)
		return ctrl.Result{}, nil
	}

	// Persist resolved family and mark Ready.
	pack.Status.Family = family
	pack.Status.ObservedGeneration = pack.Generation
	meta.SetStatusCondition(&pack.Status.Conditions, metav1.Condition{
		Type:               string(servingv1alpha2.AIPackConditionReady),
		Status:             metav1.ConditionTrue,
		Reason:             "ArtifactRegistered",
		ObservedGeneration: pack.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            "AIPack artifact registered successfully",
	})

	if err := r.patchStatus(ctx, &pack, packBefore); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("AIPack reconciled", "name", pack.Name, "kind", pack.Spec.Kind, "family", family)
	return ctrl.Result{}, nil
}

// setReadyCondition sets the Ready condition and patches status.
func (r *AIPackReconciler) setReadyCondition(
	ctx context.Context,
	pack *servingv1alpha2.AIPack,
	packBefore *servingv1alpha2.AIPack,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	meta.SetStatusCondition(&pack.Status.Conditions, metav1.Condition{
		Type:               string(servingv1alpha2.AIPackConditionReady),
		Status:             status,
		Reason:             reason,
		ObservedGeneration: pack.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            message,
	})
	return r.patchStatus(ctx, pack, packBefore)
}

// patchStatus patches the AIPack status subresource only when it has changed.
func (r *AIPackReconciler) patchStatus(
	ctx context.Context,
	pack *servingv1alpha2.AIPack,
	packBefore *servingv1alpha2.AIPack,
) error {
	if equality.Semantic.DeepEqual(&packBefore.Status, &pack.Status) {
		return nil
	}
	err := r.Status().Patch(ctx, pack, client.MergeFrom(packBefore))
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
			return nil
		}
		return fmt.Errorf("patch AIPack status: %w", err)
	}
	return nil
}

// SetupWithManager registers the AIPackReconciler with the manager.
// No Owns() calls are needed — AIPack is a pure artifact catalog entry.
func (r *AIPackReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.AIPack{}).
		Complete(r)
}
