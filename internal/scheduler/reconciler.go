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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// Reconciler owns the scheduler configuration, EPP, Service, and InferencePool.
type Reconciler struct {
	Client client.Client
	Config *ConfigReconciler
	EPP    *EPPManager
	Pool   *InferencePoolManager
}

// Cleanup removes scheduler resources after scheduler opt-out. The ordinary
// model Service is intentionally retained for direct routing.
func (r *Reconciler) Cleanup(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	if r.Client == nil {
		return fmt.Errorf("scheduler client is not configured")
	}
	for _, resource := range []struct {
		object client.Object
		name   string
	}{
		{object: &appsv1.Deployment{}, name: eppName(llmSvc.Name)},
		{object: &corev1.Service{}, name: eppName(llmSvc.Name)},
		{object: &corev1.ConfigMap{}, name: schedulerConfigName(llmSvc.Name)},
	} {
		if err := r.deleteOwned(ctx, llmSvc, resource.object, resource.name); err != nil {
			return err
		}
	}
	pool := &unstructured.Unstructured{}
	pool.SetGroupVersionKind(InferencePoolGVK)
	if err := r.deleteOwned(ctx, llmSvc, pool, llmSvc.Name); err != nil && !meta.IsNoMatchError(err) {
		return err
	}
	return nil
}

func (r *Reconciler) deleteOwned(ctx context.Context, owner *servingv1alpha2.LLMInferenceService, object client.Object, name string) error {
	object.SetName(name)
	object.SetNamespace(owner.Namespace)
	if err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: owner.Namespace}, object); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(object, owner) {
		return fmt.Errorf("refusing to delete unowned scheduler resource %s/%s", owner.Namespace, name)
	}
	if err := r.Client.Delete(ctx, object); err != nil && !apierrors.IsNotFound(err) {
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
	if err := r.EPP.Get(ctx, types.NamespacedName{Name: eppName(llmSvc.Name), Namespace: llmSvc.Namespace}, &deployment); err != nil {
		return false, err
	}
	return deployment.Status.AvailableReplicas > 0, nil
}
