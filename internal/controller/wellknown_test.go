/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

func TestGetWellKnownConfig(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{
			name:     "Gemma-4-E2B HF",
			uri:      "hf://google/gemma-4-E2B-it",
			expected: true,
		},
		{
			name:     "Gemma-4-E2B OCI Registry",
			uri:      "oci://ghcr.io/ckodex-labs/models/gemma-4-e2b-it:latest",
			expected: true,
		},
		{
			name:     "Gemma-4-E2B SeaweedFS",
			uri:      "swfs://seaweedfs-filer:8888/models/google/gemma-4-e2b-it",
			expected: true,
		},
		{
			name:     "Gemma-4-E2B Mixed Case OCI",
			uri:      "OCI://MY-REGISTRY.IO/GEMMA-4-E2B",
			expected: true,
		},
		{
			name:     "Gemma-4-E2B Complex S3 Path",
			uri:      "s3://my-bucket/prefix/experimental/google/gemma-4-E2B-it/weights",
			expected: true,
		},
		{
			name:     "Gemma-4-E2B MLX Community",
			uri:      "hf://mlx-community/gemma-4-e2b-4bit",
			expected: true,
		},
		{
			name:     "Gemma-4-E4B HF",
			uri:      "hf://google/gemma-4-E4B-it",
			expected: true,
		},
		{
			name:     "Gemma-4-26B-A4B HF",
			uri:      "hf://google/gemma-4-26B-A4B-it",
			expected: true,
		},
		{
			name:     "Gemma-4-31B HF",
			uri:      "hf://google/gemma-4-31B-it",
			expected: true,
		},
		{
			name:     "Unknown Model",
			uri:      "hf://meta-llama/Llama-2",
			expected: false,
		},
		{
			name:     "Mistral in OCI (Legacy)",
			uri:      "oci://registry/mistral-7b",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GetWellKnownConfig(tt.uri)
			if tt.expected {
				require.NotNil(t, cfg)
				assert.Contains(t, cfg.VLLMDefaults.Args, "--max-model-len")
				assert.Equal(t, api.VLLMGemma4Image, cfg.VLLMDefaults.Image)

				if strings.Contains(tt.uri, "gemma-4-E2B-it") {
					assert.Equal(t, resource.MustParse("32Gi"), cfg.VLLMDefaults.Resources.Requests[corev1.ResourceMemory])
				}
				if strings.Contains(tt.uri, "26B-A4B") {
					assert.True(t, cfg.Parallelism.Expert)
				}
				if strings.Contains(tt.uri, "31B") {
					assert.Equal(t, int32(2), *cfg.Parallelism.Tensor)
				}
			} else {
				assert.Nil(t, cfg)
			}
		})
	}
}

func TestMergePodSpec(t *testing.T) {
	base := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "vllm",
				Image: "vllm:v1",
				Env: []corev1.EnvVar{
					{Name: "EXISTING", Value: "true"},
				},
			},
		},
	}
	override := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name: "vllm",
				Env: []corev1.EnvVar{
					{Name: "NEW", Value: "true"},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("2"),
					},
				},
			},
		},
	}

	mergePodSpec(base, override)

	assert.Equal(t, "vllm:v1", base.Containers[0].Image)
	assert.Len(t, base.Containers[0].Env, 2)
	assert.Equal(t, resource.MustParse("2"), base.Containers[0].Resources.Limits[corev1.ResourceCPU])
}

func TestApplyConfigToSpecWithOverrides(t *testing.T) {
	reconciler := &LLMInferenceServiceReconciler{}
	cfg := GetWellKnownConfig("hf://google/gemma-4-E2B-it")
	require.NotNil(t, cfg)
	require.True(t, cfg.VLLMDefaults.EnableTurboQuant)

	t.Run("TurboQuant Enabled by Default", func(t *testing.T) {
		spec := &servingv1alpha2.LLMInferenceServiceSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "vllm"}},
				},
			},
		}
		reconciler.ApplyConfigToSpec(spec, cfg)

		found := false
		for _, e := range spec.Template.Spec.Containers[0].Env {
			if e.Name == "VLLM_TURBOQUANT" && e.Value == "true" {
				found = true
				break
			}
		}
		assert.True(t, found, "VLLM_TURBOQUANT should be injected by default")
	})

	t.Run("TurboQuant Suppressed by User Override", func(t *testing.T) {
		spec := &servingv1alpha2.LLMInferenceServiceSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "vllm",
							Env: []corev1.EnvVar{
								{Name: "VLLM_TURBOQUANT", Value: "false"},
							},
						},
					},
				},
			},
		}
		reconciler.ApplyConfigToSpec(spec, cfg)

		count := 0
		val := ""
		for _, e := range spec.Template.Spec.Containers[0].Env {
			if e.Name == "VLLM_TURBOQUANT" {
				count++
				val = e.Value
			}
		}
		assert.Equal(t, 1, count, "Should only have one VLLM_TURBOQUANT env var")
		assert.Equal(t, "false", val, "User-provided FALSE should be preserved")
	})
}
