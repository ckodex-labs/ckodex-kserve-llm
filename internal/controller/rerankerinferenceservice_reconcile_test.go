/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func rerankerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return scheme
}

func rerankerRequest(name, namespace string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}
}

func TestRerankerReconcileNotFound(t *testing.T) {
	scheme := rerankerTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &RerankerInferenceServiceReconciler{Client: client, Scheme: scheme}

	result, err := r.Reconcile(context.Background(), rerankerRequest("missing", "default"))

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestRerankerReconcileCreateLifecycle(t *testing.T) {
	scheme := rerankerTestScheme(t)
	service := newRerankerSvc("rerank-create", "default")
	client := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(service).
		WithStatusSubresource(&servingv1alpha2.RerankerInferenceService{}).
		Build()
	r := &RerankerInferenceServiceReconciler{Client: client, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), rerankerRequest(service.Name, service.Namespace))
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), rerankerRequest(service.Name, service.Namespace))
	require.NoError(t, err)

	updated := &servingv1alpha2.RerankerInferenceService{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, updated))
	assert.Contains(t, updated.Finalizers, rerankerFinalizer)
	assert.NotEmpty(t, updated.Status.Conditions)

	deployment := &appsv1.Deployment{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, deployment))
	assert.Equal(t, int32(1), *deployment.Spec.Replicas)
	assert.Equal(t, resource.MustParse("1"), deployment.Spec.Template.Spec.Containers[0].Resources.Requests["nvidia.com/gpu"])

	k8sService := &corev1.Service{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, k8sService))
	assert.Equal(t, int32(80), k8sService.Spec.Ports[0].Port)
}

func TestRerankerReconcileUpdateAndDelete(t *testing.T) {
	scheme := rerankerTestScheme(t)
	service := newRerankerSvc("rerank-update", "default")
	service.Finalizers = []string{rerankerFinalizer}
	service.Spec.Replicas = rerankerPtrInt32(3)
	r := &RerankerInferenceServiceReconciler{Scheme: scheme}
	desiredDeployment := r.buildDeployment(service)
	desiredService := r.buildService(service)
	desiredDeployment.Spec.Replicas = rerankerPtrInt32(1)
	desiredService.Spec.Ports[0].Port = 81
	client := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(service, desiredDeployment, desiredService).
		WithStatusSubresource(&servingv1alpha2.RerankerInferenceService{}).
		Build()
	r.Client = client

	_, err := r.Reconcile(context.Background(), rerankerRequest(service.Name, service.Namespace))
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, deployment))
	assert.Equal(t, int32(3), *deployment.Spec.Replicas)
	k8sService := &corev1.Service{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, k8sService))
	assert.Equal(t, int32(80), k8sService.Spec.Ports[0].Port)

	now := metav1.Now()
	service.DeletionTimestamp = &now
	service.Finalizers = []string{rerankerFinalizer}
	deleteClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(service).Build()
	r.Client = deleteClient
	_, err = r.Reconcile(context.Background(), rerankerRequest(service.Name, service.Namespace))
	require.NoError(t, err)
	deleted := &servingv1alpha2.RerankerInferenceService{}
	getErr := deleteClient.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, deleted)
	if !apierrors.IsNotFound(getErr) {
		require.NoError(t, getErr)
		assert.NotContains(t, deleted.Finalizers, rerankerFinalizer)
	}
}

func rerankerPtrInt32(value int32) *int32 { return &value }
