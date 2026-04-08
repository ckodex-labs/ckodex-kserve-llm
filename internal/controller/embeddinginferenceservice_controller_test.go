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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// newEmbeddingScheme returns a scheme with all types needed for Embedding controller tests.
func newEmbeddingScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

// newEmbSvc creates a minimal EmbeddingInferenceService for testing.
func newEmbSvc(name, ns string, opts ...func(*servingv1alpha2.EmbeddingInferenceService)) *servingv1alpha2.EmbeddingInferenceService {
	svc := &servingv1alpha2.EmbeddingInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       types.UID("test-uid-" + name),
		},
		Spec: servingv1alpha2.EmbeddingInferenceServiceSpec{
			Model:   servingv1alpha2.ModelSpec{URI: "hf://BAAI/bge-large-en-v1.5", Name: "bge-large-en-v1.5"},
			Runtime: servingv1alpha2.EmbeddingRuntimeInfinity,
		},
	}
	for _, o := range opts {
		o(svc)
	}
	return svc
}

// TestEmbeddingInferenceServiceReconciler_Create verifies finalizer + Deployment + Service
// are created on two reconcile passes.
func TestEmbeddingInferenceServiceReconciler_Create(t *testing.T) {
	scheme := newEmbeddingScheme(t)
	svc := newEmbSvc("bge-test", "ckodex-inference")

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(&servingv1alpha2.EmbeddingInferenceService{}).
		Build()
	r := &EmbeddingInferenceServiceReconciler{Client: cl, Scheme: scheme}

	// Pass 1: adds finalizer and returns.
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "bge-test"},
	})
	require.NoError(t, err)

	// Pass 2: creates Deployment and Service.
	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "bge-test"},
	})
	require.NoError(t, err)

	updated := &servingv1alpha2.EmbeddingInferenceService{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "bge-test"}, updated))
	assert.Contains(t, updated.Finalizers, embeddingFinalizer, "finalizer must be present")

	dep := &appsv1.Deployment{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "bge-test"}, dep),
		"Deployment must be created")

	svcObj := &corev1.Service{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "bge-test"}, svcObj),
		"Service must be created")
}

// TestEmbeddingInferenceServiceReconciler_Delete verifies the finalizer is removed on deletion.
func TestEmbeddingInferenceServiceReconciler_Delete(t *testing.T) {
	scheme := newEmbeddingScheme(t)
	now := metav1.Now()
	svc := newEmbSvc("emb-del", "ckodex-inference")
	svc.Finalizers = []string{embeddingFinalizer}
	svc.DeletionTimestamp = &now

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(&servingv1alpha2.EmbeddingInferenceService{}).
		Build()
	r := &EmbeddingInferenceServiceReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "emb-del"},
	})
	require.NoError(t, err)

	updated := &servingv1alpha2.EmbeddingInferenceService{}
	getErr := cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "emb-del"}, updated)
	if apierrors.IsNotFound(getErr) {
		// Object fully removed — finalizer was cleared and the fake client GC deleted it.
		return
	}
	require.NoError(t, getErr)
	assert.NotContains(t, updated.Finalizers, embeddingFinalizer, "finalizer must be removed")
}

// TestEmbeddingInferenceServiceReconciler_NotFound verifies graceful handling of missing CR.
func TestEmbeddingInferenceServiceReconciler_NotFound(t *testing.T) {
	scheme := newEmbeddingScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &EmbeddingInferenceServiceReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "gone"},
	})
	assert.NoError(t, err, "missing CR must not return an error")
}

// TestBuildEmbeddingContainer_InfinityDefaults verifies default image and args for infinity.
func TestBuildEmbeddingContainer_InfinityDefaults(t *testing.T) {
	r := &EmbeddingInferenceServiceReconciler{}
	svc := newEmbSvc("bge", "ns")

	c := r.buildEmbeddingContainer(svc)

	assert.Equal(t, embeddingContainerName, c.Name)
	assert.Equal(t, "michaelfeil/infinity-emb:latest-cpu", c.Image,
		"default infinity image must be used when runtimeImage is empty")
	assert.Equal(t, int32(servingv1alpha2.EmbeddingServerPort), c.Ports[0].ContainerPort)

	// Verify model ID is passed as --model-name-or-path arg.
	assert.Contains(t, c.Args, "BAAI/bge-large-en-v1.5", "hf:// prefix must be stripped")
	assert.Contains(t, c.Args, "--model-name-or-path")
	assert.Contains(t, c.Args, "v2", "infinity v2 subcommand must be present")
}

// TestBuildEmbeddingContainer_TEIRuntime verifies TEI runtime uses correct args.
func TestBuildEmbeddingContainer_TEIRuntime(t *testing.T) {
	r := &EmbeddingInferenceServiceReconciler{}
	svc := newEmbSvc("tei", "ns", func(s *servingv1alpha2.EmbeddingInferenceService) {
		s.Spec.Runtime = servingv1alpha2.EmbeddingRuntimeTextEmbeddingsInference
		s.Spec.Model = servingv1alpha2.ModelSpec{
			URI:  "hf://BAAI/bge-reranker-v2-m3",
			Name: "bge-reranker-v2-m3",
		}
	})

	c := r.buildEmbeddingContainer(svc)

	assert.Equal(t, "ghcr.io/huggingface/text-embeddings-inference:cpu-latest", c.Image)
	assert.Contains(t, c.Args, "--model-id")
	assert.Contains(t, c.Args, "BAAI/bge-reranker-v2-m3")
	// TEI does NOT include infinity v2 subcommand.
	assert.NotContains(t, c.Args, "v2")
}

// TestBuildEmbeddingContainer_OverrideImage verifies spec.runtimeImage overrides the default.
func TestBuildEmbeddingContainer_OverrideImage(t *testing.T) {
	r := &EmbeddingInferenceServiceReconciler{}
	svc := newEmbSvc("bge", "ns", func(s *servingv1alpha2.EmbeddingInferenceService) {
		s.Spec.RuntimeImage = "my-registry.io/custom-emb:v3"
	})

	c := r.buildEmbeddingContainer(svc)
	assert.Equal(t, "my-registry.io/custom-emb:v3", c.Image)
}

// TestBuildEmbeddingDeployment_ReplicasDefault verifies default 1 replica.
func TestBuildEmbeddingDeployment_ReplicasDefault(t *testing.T) {
	r := &EmbeddingInferenceServiceReconciler{}
	svc := newEmbSvc("bge", "ns")

	dep := r.buildEmbeddingDeployment(svc)
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)
}

// TestBuildEmbeddingDeployment_CustomReplicas verifies spec.replicas is propagated.
func TestBuildEmbeddingDeployment_CustomReplicas(t *testing.T) {
	r := &EmbeddingInferenceServiceReconciler{}
	svc := newEmbSvc("bge", "ns", func(s *servingv1alpha2.EmbeddingInferenceService) {
		s.Spec.Replicas = ptr.To(int32(4))
	})

	dep := r.buildEmbeddingDeployment(svc)
	assert.Equal(t, int32(4), *dep.Spec.Replicas)
}

// TestBuildEmbeddingDeployment_SidecarsMerged verifies user containers are appended.
func TestBuildEmbeddingDeployment_SidecarsMerged(t *testing.T) {
	r := &EmbeddingInferenceServiceReconciler{}
	svc := newEmbSvc("bge", "ns", func(s *servingv1alpha2.EmbeddingInferenceService) {
		s.Spec.Template.Spec.Containers = []corev1.Container{
			{Name: "metrics-exporter", Image: "prom/exporter:latest"},
		}
	})

	dep := r.buildEmbeddingDeployment(svc)
	require.Len(t, dep.Spec.Template.Spec.Containers, 2)
	assert.Equal(t, embeddingContainerName, dep.Spec.Template.Spec.Containers[0].Name,
		"embedding-server must be the first container")
	assert.Equal(t, "metrics-exporter", dep.Spec.Template.Spec.Containers[1].Name)
}

// TestBuildEmbeddingService_Spec verifies Service selector, port, and type.
func TestBuildEmbeddingService_Spec(t *testing.T) {
	r := &EmbeddingInferenceServiceReconciler{}
	svc := newEmbSvc("emb-svc", "ckodex-inference")

	s := r.buildEmbeddingService(svc)
	assert.Equal(t, corev1.ServiceTypeClusterIP, s.Spec.Type)
	assert.Equal(t, int32(servingv1alpha2.EmbeddingServerPort), s.Spec.Ports[0].Port)
	assert.Equal(t, "embeddinginferenceservice", s.Spec.Selector["app.kubernetes.io/name"])
	assert.Equal(t, "emb-svc", s.Spec.Selector["app.kubernetes.io/instance"])
}

// TestEmbeddingModelID verifies hf:// prefix stripping.
func TestEmbeddingModelID(t *testing.T) {
	assert.Equal(t, "BAAI/bge-large-en-v1.5", embeddingModelID("hf://BAAI/bge-large-en-v1.5"))
	assert.Equal(t, "s3://my-bucket/model", embeddingModelID("s3://my-bucket/model"),
		"non-hf:// URIs must be returned as-is")
}

// TestDefaultEmbeddingRuntimeImage verifies correct defaults per runtime.
func TestDefaultEmbeddingRuntimeImage(t *testing.T) {
	assert.Equal(t,
		"michaelfeil/infinity-emb:latest-cpu",
		servingv1alpha2.DefaultEmbeddingRuntimeImage(servingv1alpha2.EmbeddingRuntimeInfinity),
	)
	assert.Equal(t,
		"ghcr.io/huggingface/text-embeddings-inference:cpu-latest",
		servingv1alpha2.DefaultEmbeddingRuntimeImage(servingv1alpha2.EmbeddingRuntimeTextEmbeddingsInference),
	)
}

// TestEmbeddingReconcile_UpdateExistingDeployment verifies the deployment update path.
func TestEmbeddingReconcile_UpdateExistingDeployment(t *testing.T) {
	scheme := newEmbeddingScheme(t)
	svc := newEmbSvc("update-emb", "ns")
	svc.Finalizers = []string{embeddingFinalizer}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "update-emb", Namespace: "ns"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: embeddingLabels("update-emb")},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: embeddingLabels("update-emb")},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "old-server", Image: "old-image"}}},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc, dep).
		WithStatusSubresource(&servingv1alpha2.EmbeddingInferenceService{}).
		Build()
	r := &EmbeddingInferenceServiceReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "update-emb"},
	})
	require.NoError(t, err)

	updatedDep := &appsv1.Deployment{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "update-emb"}, updatedDep))
	assert.Equal(t, "michaelfeil/infinity-emb:latest-cpu", updatedDep.Spec.Template.Spec.Containers[0].Image)
}

// TestBuildEmbeddingContainer_CustomBatchSize verifies --batch-size propagation.
func TestBuildEmbeddingContainer_CustomBatchSize(t *testing.T) {
	r := &EmbeddingInferenceServiceReconciler{}
	svc := newEmbSvc("batch", "ns", func(s *servingv1alpha2.EmbeddingInferenceService) {
		s.Spec.BatchSize = ptr.To(int32(64))
	})

	c := r.buildEmbeddingContainer(svc)
	assert.Contains(t, c.Args, "--batch-size")
	assert.Contains(t, c.Args, "64")
}

// TestBuildEmbeddingContainer_StorageSecret verifies HUGGING_FACE_HUB_TOKEN injection.
func TestBuildEmbeddingContainer_StorageSecret(t *testing.T) {
	r := &EmbeddingInferenceServiceReconciler{}
	svc := newEmbSvc("secret", "ns", func(s *servingv1alpha2.EmbeddingInferenceService) {
		s.Spec.Model.Storage = &servingv1alpha2.StorageSpec{
			SecretRef: &corev1.LocalObjectReference{Name: "my-hf-secret"},
		}
	})

	c := r.buildEmbeddingContainer(svc)
	found := false
	for _, env := range c.Env {
		if env.Name == "HUGGING_FACE_HUB_TOKEN" {
			assert.Equal(t, "my-hf-secret", env.ValueFrom.SecretKeyRef.Name)
			found = true
			break
		}
	}
	assert.True(t, found, "HUGGING_FACE_HUB_TOKEN must be injected")
}

// TestEmbeddingReconcile_SyncStatus verifies conditions are set correctly.
func TestEmbeddingReconcile_SyncStatus(t *testing.T) {
	scheme := newEmbeddingScheme(t)
	svc := newEmbSvc("sync-status", "ns")
	svc.Finalizers = []string{embeddingFinalizer}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "sync-status", Namespace: "ns"},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc, dep).
		WithStatusSubresource(&servingv1alpha2.EmbeddingInferenceService{}).
		Build()
	r := &EmbeddingInferenceServiceReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "sync-status"},
	})
	require.NoError(t, err)

	updated := &servingv1alpha2.EmbeddingInferenceService{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sync-status"}, updated))

	cond := meta.FindStatusCondition(updated.Status.Conditions, servingv1alpha2.EmbeddingConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}
