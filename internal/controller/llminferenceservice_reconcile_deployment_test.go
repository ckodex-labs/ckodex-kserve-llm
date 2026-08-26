/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileDeployment_CreatesNew(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)

	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &deploy))
	assert.Equal(t, "my-llm", deploy.Name)
}

// TestReconcileDeployment_Gemma4WellKnown verifies that Gemma-4 gets optimized defaults.
func TestReconcileDeployment_Gemma4WellKnown(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("gemma-svc", "default")
	llmSvc.Spec.Model.URI = "hf://google/gemma-4-E2B-it"

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)

	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "gemma-svc", Namespace: "default",
	}, &deploy))

	// Verify vLLM args from WellKnown config
	vllmContainer := deploy.Spec.Template.Spec.Containers[0]
	args := strings.Join(vllmContainer.Args, " ")
	assert.Contains(t, args, "--max-model-len 131072")
	assert.Contains(t, args, "--trust-remote-code")
	assert.Contains(t, args, "--model /mnt/models")
	assert.Equal(t, api.VLLMImage, vllmContainer.Image)

	// Verify resources from WellKnown config (requests match our defined defaults)
	assert.Equal(t, resource.MustParse("8"), vllmContainer.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("32Gi"), vllmContainer.Resources.Requests[corev1.ResourceMemory])
}

// TestReconcileDeployment_OCIGemma4Optimization verifies URI-agnostic matching for OCI.
func TestReconcileDeployment_OCIGemma4Optimization(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("oci-gemma", "default")
	llmSvc.Spec.Model.URI = "oci://ghcr.io/ckodex-labs/models/gemma-4-e2b-it:latest"

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)

	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "oci-gemma", Namespace: "default",
	}, &deploy))

	// Verify vLLM args are optimized even for OCI URI
	vllmContainer := deploy.Spec.Template.Spec.Containers[0]
	args := strings.Join(vllmContainer.Args, " ")
	assert.Contains(t, args, "--model /mnt/models")
}

// TestReconcileDeployment_PVCNativeMount verifies that pvc:// URI results in direct volume mount.
func TestReconcileDeployment_PVCNativeMount(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("pvc-svc", "default")
	llmSvc.Spec.Model.URI = "pvc://existing-model-pvc"

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)

	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "pvc-svc", Namespace: "default",
	}, &deploy))

	// Verify no init container
	assert.Empty(t, deploy.Spec.Template.Spec.InitContainers)

	// Verify PVC volume source
	found := false
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.Name == api.ModelVolumeName {
			require.NotNil(t, v.PersistentVolumeClaim)
			assert.Equal(t, "existing-model-pvc", v.PersistentVolumeClaim.ClaimName)
			found = true
			break
		}
	}
	assert.True(t, found)
}

// TestReconcileDeployment_UpdatesExisting exercises the update path.
func TestReconcileDeployment_UpdatesExisting(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	// Pre-create a deployment with different replica count.
	replicas := int32(3)
	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "vllm", Image: "old-image"}},
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc, existingDeploy).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)
}

// TestReconcileService_CreatesNew verifies service creation.
