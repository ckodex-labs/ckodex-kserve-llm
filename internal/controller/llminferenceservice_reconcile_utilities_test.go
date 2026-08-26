/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCleanupResources_NoSPIRE(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := setupReconciler(cl, s)

	llmSvc := makeLLMInferenceService("my-llm", "default")
	err := r.cleanupResources(context.Background(), llmSvc)
	require.NoError(t, err)
}

// TestVolumesEqual_SameMounts returns true for identical volume mounts.
func TestVolumesEqual_SameMounts(t *testing.T) {
	mounts := []corev1.VolumeMount{
		{Name: "model-store", MountPath: "/mnt/models"},
	}
	assert.True(t, reconciler.VolumeMountsEqual(mounts, mounts))
}

// TestVolumesEqual_DifferentMounts returns false for different volume mounts.
func TestVolumesEqual_DifferentMounts(t *testing.T) {
	mounts1 := []corev1.VolumeMount{{Name: "vol1", MountPath: "/a"}}
	mounts2 := []corev1.VolumeMount{{Name: "vol1", MountPath: "/b"}}
	assert.False(t, reconciler.VolumeMountsEqual(mounts1, mounts2))
}

// TestContainersEqual_SameContainers returns true.
func TestContainersEqual_SameContainers(t *testing.T) {
	containers := []corev1.Container{{Name: "vllm", Image: "vllm:latest"}}
	assert.True(t, reconciler.ContainersEqual(containers, containers))
}

// TestContainersEqual_DifferentImages returns false.
func TestContainersEqual_DifferentImages(t *testing.T) {
	c1 := []corev1.Container{{Name: "vllm", Image: "vllm:v1"}}
	c2 := []corev1.Container{{Name: "vllm", Image: "vllm:v2"}}
	assert.False(t, reconciler.ContainersEqual(c1, c2))
}

// TestPtrToHostPath returns a valid HostPathType pointer.
func TestPtrToHostPath(t *testing.T) {
	hp := deployment.PtrToHostPath(corev1.HostPathDirectory)
	require.NotNil(t, hp)
	assert.Equal(t, corev1.HostPathDirectory, *hp)
}

// TestMapLocalModelCacheToInferenceServices_MatchingModel returns requests for
// LLMInferenceServices that use the same model URI as the cache.
func TestMapLocalModelCacheToInferenceServices_MatchingModel(t *testing.T) {
	s := buildLLMScheme(t)

	llmSvc1 := makeLLMInferenceService("svc1", "default")
	llmSvc1.Spec.Model.URI = "hf://org/model-a"

	llmSvc2 := makeLLMInferenceService("svc2", "default")
	llmSvc2.Spec.Model.URI = "hf://org/model-b" // different model

	lmc := &servingv1alpha2.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-a", Namespace: "default"},
		Spec:       servingv1alpha2.LocalModelCacheSpec{SourceModelURI: "hf://org/model-a"},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc1, llmSvc2, lmc).Build()
	r := setupReconciler(cl, s)

	requests := r.mapLocalModelCacheToInferenceServices(context.Background(), lmc)
	require.Len(t, requests, 1)
	assert.Equal(t, "svc1", requests[0].Name)
}

// TestReconcileService_UpdatesExisting exercises service update path.
