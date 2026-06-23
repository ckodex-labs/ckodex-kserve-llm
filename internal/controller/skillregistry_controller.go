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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

// SkillRegistryReconciler reconciles SkillRegistry objects.
// It validates skill entries (unique names, non-empty endpoints/schemas),
// updates the entry count in status, and marks the registry Ready.
type SkillRegistryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=skillregistries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=skillregistries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=skillregistries/finalizers,verbs=update

func (r *SkillRegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var reg servingv1alpha2.SkillRegistry
	if err := r.Get(ctx, req.NamespacedName, &reg); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get SkillRegistry: %w", err)
	}

	// Capture original object for diffing and patching
	regBeforePatch := reg.DeepCopy()

	// Handle deletion
	if reg.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&reg, api.FinalizerName) {
			controllerutil.RemoveFinalizer(&reg, api.FinalizerName)
			if err := r.Update(ctx, &reg); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&reg, api.FinalizerName) {
		controllerutil.AddFinalizer(&reg, api.FinalizerName)
		if err := r.Update(ctx, &reg); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Validate entries
	validationErr := r.validateEntries(&reg)

	if validationErr != nil {
		meta.SetStatusCondition(&reg.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ValidationFailed",
			Message:            validationErr.Error(),
			ObservedGeneration: reg.Generation,
		})
		reg.Status.EntryCount = 0
	} else {
		meta.SetStatusCondition(&reg.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "RegistryReady",
			Message:            fmt.Sprintf("%d skills registered", len(reg.Spec.Entries)),
			ObservedGeneration: reg.Generation,
		})
		reg.Status.EntryCount = int32(len(reg.Spec.Entries))
	}

	// Only patch status if it actually changed to avoid infinite reconciliation loops
	if !equality.Semantic.DeepEqual(&regBeforePatch.Status, &reg.Status) {
		if err := r.Status().Patch(ctx, &reg, client.MergeFrom(regBeforePatch)); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch SkillRegistry status: %w", err)
		}
	}

	logger.Info("SkillRegistry reconciled",
		"name", reg.Name,
		"entries", reg.Status.EntryCount,
		"valid", validationErr == nil,
	)
	return ctrl.Result{}, nil
}

// validateEntries checks for duplicate names and required fields.
func (r *SkillRegistryReconciler) validateEntries(reg *servingv1alpha2.SkillRegistry) error {
	seen := make(map[string]struct{}, len(reg.Spec.Entries))
	for i, entry := range reg.Spec.Entries {
		if entry.Name == "" {
			return fmt.Errorf("entry[%d]: name is required", i)
		}
		if entry.Endpoint == "" {
			return fmt.Errorf("entry %q: endpoint is required", entry.Name)
		}
		if entry.Version == "" {
			return fmt.Errorf("entry %q: version is required", entry.Name)
		}
		if _, dup := seen[entry.Name]; dup {
			return fmt.Errorf("duplicate skill name %q in registry", entry.Name)
		}
		seen[entry.Name] = struct{}{}
	}
	return nil
}

// SetupWithManager registers the SkillRegistryReconciler.
func (r *SkillRegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.SkillRegistry{}).
		Named("skillregistry").
		Complete(r)
}
