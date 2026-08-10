/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestReconcilerCleanupDeletesOwnedSchedulerResources(t *testing.T) {
	scheme := schedulerScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	svc := minimalLLMSvc("llama", "inference")
	svc.UID = types.UID("llama-uid")

	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: eppName(svc.Name), Namespace: svc.Namespace}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: eppName(svc.Name), Namespace: svc.Namespace}}
	config := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: schedulerConfigName(svc.Name), Namespace: svc.Namespace}}
	for _, object := range []client.Object{deployment, service, config} {
		require.NoError(t, controllerutil.SetControllerReference(svc, object, scheme))
	}
	pool := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "inference.networking.k8s.io/v1",
		"kind":       "InferencePool",
		"metadata": map[string]interface{}{
			"name": svc.Name, "namespace": svc.Namespace,
		},
	}}
	pool.SetGroupVersionKind(InferencePoolGVK)
	require.NoError(t, controllerutil.SetControllerReference(svc, pool, scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(deployment, service, config, pool).
		Build()
	r := &Reconciler{Client: fakeClient}

	require.NoError(t, r.Cleanup(context.Background(), svc))
	for _, object := range []client.Object{deployment, service, config} {
		err := fakeClient.Get(context.Background(), types.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, object)
		assert.True(t, apierrors.IsNotFound(err), "resource %s should be deleted", object.GetName())
	}
	remainingPool := &unstructured.Unstructured{}
	remainingPool.SetGroupVersionKind(InferencePoolGVK)
	err := fakeClient.Get(context.Background(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, remainingPool)
	assert.True(t, apierrors.IsNotFound(err), "inference pool should be deleted")
}

func TestReconcilerCleanupRefusesForeignSchedulerResource(t *testing.T) {
	scheme := schedulerScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	foreign := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: schedulerConfigName("llama"), Namespace: "inference",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "apps/v1", Kind: "Deployment", Name: "other", UID: types.UID("other"),
			Controller: boolPtr(true), BlockOwnerDeletion: boolPtr(true),
		}},
	}}
	svc := minimalLLMSvc("llama", "inference")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(foreign).Build()
	r := &Reconciler{Client: fakeClient}

	err := r.Cleanup(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to delete unowned scheduler resource")
	assert.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{Name: foreign.Name, Namespace: foreign.Namespace}, foreign))
}
