/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

// --- hf-mount CSI volume tests ---

func TestBuildStorageInitializer_HFMountReturnsNil(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	llmSvc := baseLLMInferenceService()
	llmSvc.Spec.Model.URI = "hf-mount://Qwen/Qwen2.5-0.5B-Instruct"

	result := r.buildStorageInitializer(ctx, llmSvc, nil)
	if result != nil {
		t.Error("buildStorageInitializer should return nil for hf-mount:// URIs")
	}
}

// TestBuildDeployment_HFMountCSIVolume verifies that hf-mount:// URIs produce a
// PersistentVolumeClaim reference in the pod spec (not an inline CSI volume).
// The actual PV+PVC are provisioned by HFCSIReconciler before pod scheduling.
func TestBuildDeployment_HFMountCSIVolume(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	llmSvc := baseLLMInferenceService()
	llmSvc.Spec.Model.URI = "hf-mount://Qwen/Qwen2.5-0.5B-Instruct"

	deploy := r.buildDeployment(ctx, llmSvc, 1)
	podSpec := deploy.Spec.Template.Spec

	// Should have no init containers (hf-mount skips storage initializer)
	for _, ic := range podSpec.InitContainers {
		if ic.Name == "storage-initializer" {
			t.Error("hf-mount should not have a storage-initializer init container")
		}
	}

	// Should reference a PVC (provisioned by HFCSIReconciler), not an inline CSI volume.
	var pvcVol *corev1.PersistentVolumeClaimVolumeSource
	for _, v := range podSpec.Volumes {
		if v.Name == api.ModelVolumeName && v.PersistentVolumeClaim != nil {
			pvcVol = v.PersistentVolumeClaim
			break
		}
	}
	if pvcVol == nil {
		t.Fatal("expected PVC volume for hf-mount:// URI — HFCSIReconciler provisions PV+PVC before pod scheduling")
	}
	expectedPVC := "hf-model-" + llmSvc.Namespace + "-" + llmSvc.Name
	if pvcVol.ClaimName != expectedPVC {
		t.Errorf("PVC ClaimName = %q, want %q", pvcVol.ClaimName, expectedPVC)
	}
	if !pvcVol.ReadOnly {
		t.Error("hf-mount PVC volume should be read-only")
	}
}

// TestBuildDeployment_HFMountWithRevision checks that the PVC name is deterministic
// even when the URI includes a revision suffix (@v1.0). Revision is handled in HFCSIReconciler.
func TestBuildDeployment_HFMountWithRevision(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	llmSvc := baseLLMInferenceService()
	llmSvc.Spec.Model.URI = "hf-mount://Qwen/Qwen2.5-0.5B-Instruct@v1.0"

	deploy := r.buildDeployment(ctx, llmSvc, 1)

	var pvcVol *corev1.PersistentVolumeClaimVolumeSource
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.Name == api.ModelVolumeName && v.PersistentVolumeClaim != nil {
			pvcVol = v.PersistentVolumeClaim
			break
		}
	}
	if pvcVol == nil {
		t.Fatal("expected PVC volume for hf-mount:// URI")
	}
	// PVC name is based on namespace+name, not the URI — revision is encoded in the PV spec.
	expectedPVC := "hf-model-" + llmSvc.Namespace + "-" + llmSvc.Name
	if pvcVol.ClaimName != expectedPVC {
		t.Errorf("PVC ClaimName = %q, want %q", pvcVol.ClaimName, expectedPVC)
	}
}

// TestBuildDeployment_HFMountWithSecret checks that secret handling does not affect
// the pod-spec PVC reference. Auth is encoded in the PV's nodePublishSecretRef by HFCSIReconciler.
func TestBuildDeployment_HFMountWithSecret(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	llmSvc := baseLLMInferenceService()
	llmSvc.Spec.Model.URI = "hf-mount://myorg/private-model"
	llmSvc.Spec.Model.Storage = &servingv1alpha2.StorageSpec{
		SecretRef: &corev1.LocalObjectReference{Name: "hf-token-secret"},
	}

	deploy := r.buildDeployment(ctx, llmSvc, 1)

	var pvcVol *corev1.PersistentVolumeClaimVolumeSource
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.Name == api.ModelVolumeName && v.PersistentVolumeClaim != nil {
			pvcVol = v.PersistentVolumeClaim
			break
		}
	}
	if pvcVol == nil {
		t.Fatal("expected PVC volume for hf-mount:// URI")
	}
	expectedPVC := "hf-model-" + llmSvc.Namespace + "-" + llmSvc.Name
	if pvcVol.ClaimName != expectedPVC {
		t.Errorf("PVC ClaimName = %q, want %q", pvcVol.ClaimName, expectedPVC)
	}
	// Auth (hf-token-secret) is bound in the PV's nodePublishSecretRef, not the pod spec.
	// HFCSIReconciler_test.go validates that binding.
}
