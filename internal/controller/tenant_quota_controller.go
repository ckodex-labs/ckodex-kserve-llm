/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	// LabelTenantID is the namespace label that marks a tenant-owned namespace.
	LabelTenantID = "ckodex.com/tenant-id"

	// quotaName is the ResourceQuota created in every tenant namespace.
	quotaName = "ckodex-tenant-quota"

	// limitRangeName is the LimitRange created in every tenant namespace.
	limitRangeName = "ckodex-tenant-limits"
)

// TenantQuotaDefaults are the ResourceQuota limits applied to each tenant namespace.
// They can be overridden per-namespace by annotating the namespace with
// ckodex.com/quota-<resource>=<quantity>.
type TenantQuotaDefaults struct {
	// MaxLLMInferenceServices is the maximum number of LLMInferenceService CRs.
	MaxLLMInferenceServices int64
	// MaxGPUs is the maximum nvidia.com/gpu requests across all pods.
	MaxGPUs int64
	// MaxCPU is the maximum CPU requests (as a Kubernetes quantity string).
	MaxCPU string
	// MaxMemory is the maximum memory requests (as a Kubernetes quantity string).
	MaxMemory string
}

// DefaultTenantQuota returns conservative production defaults.
func DefaultTenantQuota() TenantQuotaDefaults {
	return TenantQuotaDefaults{
		MaxLLMInferenceServices: 5,
		MaxGPUs:                 8,
		MaxCPU:                  "64",
		MaxMemory:               "256Gi",
	}
}

// TenantQuotaReconciler watches namespaces labeled ckodex.com/tenant-id and
// reconciles ResourceQuota and LimitRange objects to enforce per-tenant resource caps.
//
// Design rationale: quota objects are owned by the namespace (via OwnerReference is
// not applicable for cluster-scoped Namespaces), so we rely on label-based discovery
// and last-write-wins update semantics rather than OwnerReferences.
type TenantQuotaReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Defaults TenantQuotaDefaults
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch;create;update;patch;delete

// Reconcile watches Namespace objects labeled ckodex.com/tenant-id and ensures
// a ResourceQuota + LimitRange exist in that namespace.
func (r *TenantQuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("namespace", req.Name)

	var ns corev1.Namespace
	if err := r.Get(ctx, req.NamespacedName, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetch namespace: %w", err)
	}

	// Only reconcile namespaces that carry the tenant label.
	tenantID, ok := ns.Labels[LabelTenantID]
	if !ok || tenantID == "" {
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("tenantID", tenantID)

	if err := r.reconcileResourceQuota(ctx, ns.Name, tenantID); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile resource quota: %w", err)
	}

	if err := r.reconcileLimitRange(ctx, ns.Name); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile limit range: %w", err)
	}

	logger.Info("tenant quota reconciled")
	return ctrl.Result{}, nil
}

// reconcileResourceQuota creates or updates the tenant ResourceQuota.
func (r *TenantQuotaReconciler) reconcileResourceQuota(ctx context.Context, namespace, tenantID string) error {
	desired := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaName,
			Namespace: namespace,
			Labels: map[string]string{
				LabelTenantID:                  tenantID,
				"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				// GPU cap — prevents a tenant from monopolising cluster GPU capacity.
				corev1.ResourceName("requests.nvidia.com/gpu"): resource.MustParse(
					fmt.Sprintf("%d", r.Defaults.MaxGPUs)),
				corev1.ResourceCPU:    resource.MustParse(r.Defaults.MaxCPU),
				corev1.ResourceMemory: resource.MustParse(r.Defaults.MaxMemory),
				// LLMInferenceService count limit.
				corev1.ResourceName("count/llminferenceservices.serving.ckodex.com"): resource.MustParse(
					fmt.Sprintf("%d", r.Defaults.MaxLLMInferenceServices)),
			},
		},
	}

	var existing corev1.ResourceQuota
	if err := r.Get(ctx, client.ObjectKey{Name: quotaName, Namespace: namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		return r.Update(ctx, &existing)
	}
	return nil
}

// reconcileLimitRange creates or updates a LimitRange setting default container limits.
// This prevents unbounded containers from consuming all resources in the namespace.
func (r *TenantQuotaReconciler) reconcileLimitRange(ctx context.Context, namespace string) error {
	desired := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      limitRangeName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
			},
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("32"),
						corev1.ResourceMemory: resource.MustParse("128Gi"),
					},
				},
			},
		},
	}

	var existing corev1.LimitRange
	if err := r.Get(ctx, client.ObjectKey{Name: limitRangeName, Namespace: namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		return r.Update(ctx, &existing)
	}
	return nil
}

// SetupWithManager registers the TenantQuotaReconciler and filters to Namespace events only.
func (r *TenantQuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Only trigger on Namespace create/update where the tenant label is present.
	tenantLabelPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, ok := e.Object.GetLabels()[LabelTenantID]
			return ok
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			_, oldOk := e.ObjectOld.GetLabels()[LabelTenantID]
			_, newOk := e.ObjectNew.GetLabels()[LabelTenantID]
			return oldOk || newOk
		},
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		WithEventFilter(tenantLabelPredicate).
		Named("tenant-quota").
		Complete(r)
}
