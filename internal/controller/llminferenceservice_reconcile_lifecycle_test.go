/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	kserveintegration "github.com/ckodex-labs/kserve-llm-operator/internal/kserve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLLMInferenceService_ReconcileMultiNodeDelegatesToKServe(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("distributed", "default")
	llmSvc.Spec.Model.URI = "pvc://model-weights"
	llmSvc.Spec.Worker = &servingv1alpha2.WorkerSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "worker"}},
			},
		},
	}
	llmSvc.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Tensor:   ptr32(2),
		Pipeline: ptr32(2),
	}
	modelPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "model-weights", Namespace: llmSvc.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc, modelPVC).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)
	r.KServeMultiNode = &kserveintegration.Reconciler{
		Client: cl,
		Scheme: s,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace},
	})
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, result.RequeueAfter)

	assertDelegatedToKServe(t, cl, llmSvc)
}

func assertDelegatedToKServe(t *testing.T, cl client.Client, llmSvc *servingv1alpha2.LLMInferenceService) {
	t.Helper()
	isvc := kserveintegration.NewInferenceService()
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}, isvc))
	require.Error(t, cl.Get(context.Background(), k8stypes.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}, &appsv1.Deployment{}))
	require.Error(t, cl.Get(context.Background(), k8stypes.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}, &corev1.Service{}))
	require.Error(t, cl.Get(context.Background(), k8stypes.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}, &policyv1.PodDisruptionBudget{}))
}
func TestLLMInferenceService_ReconcileNotFound(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := setupReconciler(cl, s)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "missing", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestLLMInferenceService_ReconcileRejectsUnsupportedEngine(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("unsupported-engine", "default")
	llmSvc.Spec.Engine = "other"
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace},
	})
	require.ErrorContains(t, err, "unsupported inference engine")

	var deployment appsv1.Deployment
	err = cl.Get(context.Background(), k8stypes.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}, &deployment)
	require.Error(t, err)
}

// TestLLMInferenceService_ReconcileCreatesDeploymentServicePDB verifies the main
// reconcile loop creates Deployment, Service, and PDB for a new CR.
func TestLLMInferenceService_ReconcileCreatesDeploymentServicePDB(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-llm", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Deployment should exist.
	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &deploy))

	// Service should exist.
	var svc corev1.Service
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &svc))

	// PDB should exist.
	var pdb policyv1.PodDisruptionBudget
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &pdb))

	// Finalizer should be present on the CR.
	var updated servingv1alpha2.LLMInferenceService
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &updated))
	assert.Contains(t, updated.Finalizers, api.FinalizerName)
}

// TestLLMInferenceService_ReconcileDeletion exercises the finalizer cleanup path.
func TestLLMInferenceService_ReconcileDeletion(t *testing.T) {
	s := buildLLMScheme(t)
	// Create the object first without a deletion timestamp, then patch it.
	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Finalizers = []string{api.FinalizerName}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()

	// Mark for deletion by calling Delete on the fake client (sets DeletionTimestamp).
	require.NoError(t, cl.Delete(context.Background(), llmSvc))

	r := setupReconciler(cl, s)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-llm", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestReconcileDeployment_CreatesNew verifies deployment creation.
func TestReconcile_OCI_PullFailureStatus(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("fail-svc", "default")
	llmSvc.Spec.Model.URI = "oci://broken-registry/broken-model"

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)

	// Simulate reconciliation loop
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "fail-svc", Namespace: "default"},
	})
	require.NoError(t, err)

	// Verify conditions even before successful rollout
	var updated servingv1alpha2.LLMInferenceService
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "fail-svc", Namespace: "default",
	}, &updated))

	// Deployment should be created but not ready
	foundDeploymentReady := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == servingv1alpha2.ConditionDeploymentReady {
			assert.Equal(t, metav1.ConditionFalse, cond.Status)
			foundDeploymentReady = true
		}
	}
	assert.True(t, foundDeploymentReady)
}
