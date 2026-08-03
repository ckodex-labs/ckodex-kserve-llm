/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// Reconciler owns the scheduler configuration, EPP, Service, and InferencePool.
type Reconciler struct {
	Config *ConfigReconciler
	EPP    *EPPManager
	Pool   *InferencePoolManager
}

// Cleanup removes scheduler resources after scheduler opt-out. The ordinary
// model Service is intentionally retained for direct routing.
func (r *Reconciler) Cleanup(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	if r.EPP == nil || r.Config == nil || r.Pool == nil {
		return nil
	}
	objects := []client.Object{
		&appsv1.Deployment{}, &corev1.Service{}, &corev1.ConfigMap{},
	}
	names := []string{llmSvc.Name + "-epp", llmSvc.Name + "-epp", llmSvc.Name + "-scheduler-config"}
	for i, object := range objects {
		object.SetName(names[i])
		object.SetNamespace(llmSvc.Namespace)
		if err := r.EPP.Delete(ctx, object); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	pool := &unstructured.Unstructured{}
	pool.SetGroupVersionKind(inferencePoolGVK)
	pool.SetName(llmSvc.Name)
	pool.SetNamespace(llmSvc.Namespace)
	if err := r.Pool.Delete(ctx, pool); err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return err
	}
	return nil
}

// Reconcile returns true only after the EPP Deployment has an available replica.
func (r *Reconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) (bool, error) {
	if r.Config == nil || r.EPP == nil || r.Pool == nil {
		return false, fmt.Errorf("scheduler reconcilers are not fully configured")
	}
	if err := r.Config.Reconcile(ctx, llmSvc); err != nil {
		return false, err
	}
	if err := r.EPP.Reconcile(ctx, llmSvc); err != nil {
		return false, err
	}
	if err := r.Pool.Reconcile(ctx, llmSvc); err != nil {
		return false, err
	}
	var deployment appsv1.Deployment
	if err := r.EPP.Get(ctx, types.NamespacedName{Name: llmSvc.Name + "-epp", Namespace: llmSvc.Namespace}, &deployment); err != nil {
		return false, err
	}
	return deployment.Status.AvailableReplicas > 0, nil
}
