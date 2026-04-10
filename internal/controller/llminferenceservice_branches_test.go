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
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/cleanup"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/status"
)

// ---- buildStorageInitializer -----------------------------------------------

// TestBuildStorageInitializer_EmptyURI_ReturnsNil returns nil when URI is empty.
func TestBuildStorageInitializer_EmptyURI_ReturnsNil(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Spec.Model.URI = ""

	c := r.buildStorageInitializer(context.Background(), llmSvc, nil)
	assert.Nil(t, c, "empty URI should return nil init container")
}

// TestBuildStorageInitializer_ModelpackURI_ReturnsNil returns nil for modelpack:// scheme.
func TestBuildStorageInitializer_ModelpackURI_ReturnsNil(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Spec.Model.URI = "modelpack://org/llama3:latest"

	c := r.buildStorageInitializer(context.Background(), llmSvc, nil)
	assert.Nil(t, c, "modelpack:// URI should return nil (CSI-native)")
}

// TestBuildStorageInitializer_HFMountURI_ReturnsNil returns nil for hf-mount:// scheme.
func TestBuildStorageInitializer_HFMountURI_ReturnsNil(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Spec.Model.URI = "hf-mount://mistral-community/Mistral-7B-v0.2"

	c := r.buildStorageInitializer(context.Background(), llmSvc, nil)
	assert.Nil(t, c, "hf-mount:// URI should return nil (CSI-native)")
}

// TestBuildStorageInitializer_HFUri_ReturnsContainer returns a container for hf:// URIs.
func TestBuildStorageInitializer_HFUri_ReturnsContainer(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Spec.Model.URI = "hf://meta-llama/Llama-3-8B"

	c := r.buildStorageInitializer(context.Background(), llmSvc, nil)
	require.NotNil(t, c, "hf:// URI should produce an init container")
	assert.Equal(t, "storage-initializer", c.Name)
}

// TestBuildStorageInitializer_ZeroCopyReadyLMC_ReturnsNil bypasses initializer when LMC ready.
func TestBuildStorageInitializer_ZeroCopyReadyLMC_ReturnsNil(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Spec.Model.URI = "hf://meta-llama/Llama-3-8B"

	// Active LMC with a Ready node — Zero-Copy path.
	lmc := &servingv1alpha2.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
		Spec:       servingv1alpha2.LocalModelCacheSpec{SourceModelURI: "hf://meta-llama/Llama-3-8B"},
		Status: servingv1alpha2.LocalModelCacheStatus{
			NodeStatuses: []servingv1alpha2.NodeCacheStatus{
				{NodeName: "node-1", Phase: "Ready"},
			},
		},
	}

	c := r.buildStorageInitializer(context.Background(), llmSvc, lmc)
	assert.Nil(t, c, "zero-copy ready LMC should bypass init container")
}

// TestBuildStorageInitializer_WithSecretRef injects envFrom when SecretRef is set.
func TestBuildStorageInitializer_WithSecretRef(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Spec.Model.URI = "hf://meta-llama/Llama-3-8B"
	llmSvc.Spec.Model.Storage = &servingv1alpha2.StorageSpec{
		SecretRef: &corev1.LocalObjectReference{Name: "my-secret"},
	}

	c := r.buildStorageInitializer(context.Background(), llmSvc, nil)
	require.NotNil(t, c)
	require.Len(t, c.EnvFrom, 1)
	assert.Equal(t, "my-secret", c.EnvFrom[0].SecretRef.Name)
}

// TestBuildStorageInitializer_WithVaultRef injects VAULT_PATH env when VaultRef is set.
func TestBuildStorageInitializer_WithVaultRef(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Spec.Model.URI = "hf://meta-llama/Llama-3-8B"
	llmSvc.Spec.Model.Storage = &servingv1alpha2.StorageSpec{
		VaultRef:  "secret/data/hf-token",
		VaultAddr: "https://vault.example.com",
	}

	c := r.buildStorageInitializer(context.Background(), llmSvc, nil)
	require.NotNil(t, c)

	envNames := make(map[string]string)
	for _, e := range c.Env {
		envNames[e.Name] = e.Value
	}
	assert.Equal(t, "secret/data/hf-token", envNames["VAULT_PATH"])
	assert.Equal(t, "https://vault.example.com", envNames["VAULT_ADDR"])
}

// ---- reconcilePDB ----------------------------------------------------------

// TestReconcilePDB_UpdatesExisting exercises the spec-update path.
func TestReconcilePDB_UpdatesExisting(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.UID = k8stypes.UID("test-uid-my-llm") // needed for SetControllerReference

	// Pre-create a PDB with a different selector so the update path is hit.
	maxUnavailable := intstr.FromInt32(2)
	existingPDB := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-llm",
			Namespace: "default",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable, // different spec → triggers update
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "old"},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc, existingPDB).Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	err := r.PDBReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	// Verify the PDB was updated (MinAvailable should now be set).
	var updated policyv1.PodDisruptionBudget
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &updated))
	assert.NotNil(t, updated.Spec.MinAvailable)
}

// TestReconcilePDB_NoUpdateWhenSpecIdentical skips update when spec unchanged.
func TestReconcilePDB_NoUpdateWhenSpecIdentical(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.UID = k8stypes.UID("test-uid-my-llm")

	labels := map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   "my-llm",
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
	}
	minAvailable := intstr.FromInt32(1)
	existingPDB := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector:     &metav1.LabelSelector{MatchLabels: labels},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc, existingPDB).Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	err := r.PDBReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err) // idempotent — no error, no update needed
}

// ---- LLMInferenceService.Reconcile additional branches --------------------

// TestLLMInferenceService_Reconcile_WithLocalModelCache exercises the zero-copy
// affinity-injection path when an active LocalModelCache exists.
func TestLLMInferenceService_Reconcile_WithLocalModelCache(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Spec.Model.URI = "hf://meta-llama/Llama-3-8B"

	// LocalModelCache with a Ready node triggers the zero-copy path in buildDeployment.
	lmc := &servingv1alpha2.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "llama-cache",
			Namespace: "default",
			UID:       k8stypes.UID("lmc-uid"),
		},
		Spec: servingv1alpha2.LocalModelCacheSpec{SourceModelURI: "hf://meta-llama/Llama-3-8B"},
		Status: servingv1alpha2.LocalModelCacheStatus{
			NodeStatuses: []servingv1alpha2.NodeCacheStatus{
				{NodeName: "gpu-node-1", Phase: "Ready"},
			},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc, lmc).
		WithStatusSubresource(llmSvc).
		Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: s,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-llm", Namespace: "default"},
	})
	require.NoError(t, err)
}
