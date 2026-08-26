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
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileService_CreatesNew(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.ServiceReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	var svc corev1.Service
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &svc))
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
}

// TestReconcileService_GRPCPortAdded verifies gRPC port is added when enabled.
func TestReconcileService_GRPCPortAdded(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)
	r.ServiceReconciler.EnableGRPC = true

	err := r.ServiceReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	var svc corev1.Service
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &svc))
	assert.Len(t, svc.Spec.Ports, 2)
	assert.Equal(t, "grpc-inference", svc.Spec.Ports[1].Name)
}

// TestReconcilePDB_CreatesNew verifies PodDisruptionBudget creation.
func TestReconcilePDB_CreatesNew(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.PDBReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	var pdb policyv1.PodDisruptionBudget
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &pdb))
	assert.NotNil(t, pdb.Spec.MinAvailable)
}

// TestUpdateStatus_NoDeployment sets replicas=0 and ready=false when deployment missing.
func TestUpdateStatus_NoDeployment(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)

	err := r.StatusReconciler.Update(context.Background(), llmSvc, llmSvc.DeepCopy(), false, nil)
	require.NoError(t, err)
	assert.Equal(t, int32(0), llmSvc.Status.Replicas)
	assert.False(t, llmSvc.Status.ModelReady)
}

// TestUpdateStatus_WithReadyDeployment sets replicas and ready from deployment status.
func TestUpdateStatus_WithReadyDeployment(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	readyReplicas := int32(2)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: readyReplicas},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc, deploy).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)

	err := r.StatusReconciler.Update(context.Background(), llmSvc, llmSvc.DeepCopy(), false, nil)
	require.NoError(t, err)
	assert.Equal(t, readyReplicas, llmSvc.Status.Replicas)
	assert.True(t, llmSvc.Status.ModelReady)
}

// TestCleanupResources_NoSPIRE does not error when SPIRE not configured.
func TestReconcileService_UpdatesExisting(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "old-port", Port: 9999},
			},
			Selector: map[string]string{"old-label": "val"},
			Type:     corev1.ServiceTypeNodePort,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc, existingSvc).Build()
	r := setupReconciler(cl, s)

	err := r.ServiceReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	var svc corev1.Service
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &svc))
	// Port should have been updated to the managed port.
	assert.Equal(t, "http-inference", svc.Spec.Ports[0].Name)
}

// TestReconcile_OCI_PullFailureStatus verifies Chaos path for OCI failures.
