/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestHFCSIPVUsesCanonicalHuggingFaceTokenKey(t *testing.T) {
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "gpt2", Namespace: "inference"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{Model: servingv1alpha2.ModelSpec{
			URI:     "hf-mount://openai-community/gpt2",
			Storage: &servingv1alpha2.StorageSpec{SecretRef: &corev1.LocalObjectReference{Name: "hf-credentials"}},
		}},
	}

	pv := (&HFCSIReconciler{}).buildPV(svc, HFPVName(svc), "openai-community/gpt2", "")
	require.NotNil(t, pv.Spec.CSI)
	require.NotNil(t, pv.Spec.CSI.NodePublishSecretRef)
	assert.Equal(t, "HF_TOKEN", pv.Spec.CSI.VolumeAttributes["tokenKey"])
	assert.Equal(t, "hf-credentials", pv.Spec.CSI.NodePublishSecretRef.Name)
	assert.Equal(t, "inference", pv.Spec.CSI.NodePublishSecretRef.Namespace)
}

func TestParseHFMountURIWithRevision(t *testing.T) {
	repo, revision := parseHFMountURI("hf-mount://openai-community/gpt2@refs/pr/1")
	assert.Equal(t, "openai-community/gpt2", repo)
	assert.Equal(t, "refs/pr/1", revision)
}
