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
	"k8s.io/apimachinery/pkg/api/resource"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestGetWellKnownConfig_ModelIntentPresets(t *testing.T) {
	tests := []struct {
		name       string
		uri        string
		wantTensor int32
		wantGPU    string
		wantMemory string
		wantExpert bool
		wantEPLB   bool
		wantArgs   []string
	}{
		{name: "gemma 4 e2b", uri: "hf://google/gemma-4-E2B-it", wantGPU: "1", wantMemory: "32Gi", wantArgs: []string{"--max-num-seqs", "256"}},
		{name: "gemma 4 e4b", uri: "oci://registry/gemma-4-e4b", wantGPU: "1", wantMemory: "64Gi", wantArgs: []string{"--max-num-seqs", "128"}},
		{name: "gemma 4 moe", uri: "swfs://filer/gemma-4-26b-a4b", wantGPU: "1", wantMemory: "128Gi", wantExpert: true},
		{name: "gemma 4 dense multi gpu", uri: "hf://google/gemma-4-31b-it", wantTensor: 2, wantGPU: "2", wantMemory: "256Gi"},
		{name: "llama 8b", uri: "meta-llama/Llama-3.1-8B-it", wantGPU: "1", wantMemory: "32Gi"},
		{name: "llama 70b", uri: "meta-llama/Llama-3.1-70B-it", wantTensor: 4, wantGPU: "4", wantMemory: "256Gi"},
		{name: "mistral 7b", uri: "mistralai/Mistral-7B-v0.3", wantGPU: "1", wantMemory: "32Gi", wantArgs: []string{"--tokenizer-mode", "mistral"}},
		{name: "deepseek v4", uri: "hf://deepseek-ai/DeepSeek-V4", wantTensor: 8, wantGPU: "8", wantMemory: "512Gi", wantExpert: true, wantEPLB: true},
		{name: "deepseek v3.2", uri: "oci://registry/deepseek-v3.2", wantTensor: 8, wantGPU: "8", wantExpert: true, wantEPLB: true},
		{name: "qwen 72b", uri: "hf://Qwen/Qwen3-72B", wantTensor: 4, wantGPU: "4", wantMemory: "256Gi"},
		{name: "qwen 8b", uri: "hf://Qwen/Qwen3-8B", wantGPU: "1", wantMemory: "32Gi"},
		{name: "qwen 7b", uri: "hf://Qwen/Qwen3-7B", wantGPU: "1", wantMemory: "32Gi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GetWellKnownConfig(tt.uri)
			require.NotNil(t, cfg)
			require.NotNil(t, cfg.VLLMDefaults)
			require.NotNil(t, cfg.VLLMDefaults.Resources)
			assert.Equal(t, tt.wantExpert, parallelismExpert(cfg))
			assert.Equal(t, tt.wantEPLB, parallelismEPLB(cfg))
			if tt.wantTensor > 0 {
				require.NotNil(t, cfg.Parallelism)
				require.NotNil(t, cfg.Parallelism.Tensor)
				assert.Equal(t, tt.wantTensor, *cfg.Parallelism.Tensor)
			}
			assert.Equal(t, resource.MustParse(tt.wantGPU), cfg.VLLMDefaults.Resources.Requests["nvidia.com/gpu"])
			if tt.wantMemory != "" {
				assert.Equal(t, resource.MustParse(tt.wantMemory), cfg.VLLMDefaults.Resources.Requests[corev1.ResourceMemory])
			}
			for _, arg := range tt.wantArgs {
				assert.Contains(t, cfg.VLLMDefaults.Args, arg)
			}
		})
	}
}

func TestGetWellKnownConfig_UnknownAndBoundaryIntents(t *testing.T) {
	for _, uri := range []string{"", "hf://google/gemma-3-E2B"} {
		assert.Nil(t, GetWellKnownConfig(uri), uri)
	}
	for _, uri := range []string{"hf://Qwen/Qwen3-72B-extra", "hf://deepseek-ai/deepseek-v3.2-preview"} {
		if uri == "hf://deepseek-ai/deepseek-v3.2-preview" {
			assert.NotNil(t, GetWellKnownConfig(uri), uri)
			continue
		}
		assert.NotNil(t, GetWellKnownConfig(uri), uri)
	}
}

func TestApplyConfigToSpec_DefaultPrecedenceAndOptionalBranches(t *testing.T) {
	tensor, data := int32(4), int32(2)
	min, max := int32(1), int32(5)
	cfg := &servingv1alpha2.LLMInferenceServiceConfigSpec{
		Parallelism: &servingv1alpha2.ParallelismSpec{Tensor: &tensor, Data: &data, Expert: true},
		Scaling:     &servingv1alpha2.ScalingSpec{MinReplicas: &min, MaxReplicas: &max},
		Template: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Image: "default/image", Env: []corev1.EnvVar{{Name: "FROM_DEFAULT", Value: "yes"}},
			Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("8")}},
		}}}},
		VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
			Image: "default/image", Args: []string{"--present", "--enable-turboquant", "--new"},
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4")},
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("16Gi")},
			},
			EnableTurboQuant: true,
		},
	}
	spec := &servingv1alpha2.LLMInferenceServiceSpec{
		Parallelism: &servingv1alpha2.ParallelismSpec{Data: wellKnownPtrInt32(9)},
		Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Image: "user/image", Args: []string{"--present", "--disable-turboquant", "--enable-request-id-headers"},
			Env:       []corev1.EnvVar{{Name: "VLLM_TURBOQUANT", Value: "user"}},
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("9")}},
		}}}},
	}

	(&LLMInferenceServiceReconciler{}).ApplyConfigToSpec(spec, cfg)
	require.NotNil(t, spec.Parallelism)
	assert.Equal(t, int32(9), *spec.Parallelism.Data)
	assert.Equal(t, tensor, *spec.Parallelism.Tensor)
	assert.False(t, spec.Parallelism.Expert)
	assert.Equal(t, min, *spec.Scaling.MinReplicas)
	assert.Equal(t, "default/image", spec.Template.Spec.Containers[0].Image)
	assert.Contains(t, spec.Template.Spec.Containers[0].Args, "--new")
	assert.NotContains(t, spec.Template.Spec.Containers[0].Args, "--enable-turboquant")
	assert.Len(t, spec.Template.Spec.Containers[0].Env, 2)
	assert.Equal(t, resource.MustParse("9"), spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("16Gi"), spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory])

	(&LLMInferenceServiceReconciler{}).ApplyConfigToSpec(&servingv1alpha2.LLMInferenceServiceSpec{}, nil)
	noContainers := &servingv1alpha2.LLMInferenceServiceSpec{}
	(&LLMInferenceServiceReconciler{}).ApplyConfigToSpec(noContainers, &servingv1alpha2.LLMInferenceServiceConfigSpec{VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{Image: "ignored"}})
}

func TestApplyConfigToSpec_NewParallelismAndPodDefaults(t *testing.T) {
	tensor := int32(2)
	spec := &servingv1alpha2.LLMInferenceServiceSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{}}}}}
	cfg := &servingv1alpha2.LLMInferenceServiceConfigSpec{
		Parallelism: &servingv1alpha2.ParallelismSpec{Tensor: &tensor},
		VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
			Image: "default", Args: []string{"--x"}, EnableTurboQuant: true,
			Resources: &corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")}},
		},
	}
	(&LLMInferenceServiceReconciler{}).ApplyConfigToSpec(spec, cfg)
	require.NotNil(t, spec.Parallelism)
	assert.Equal(t, int32(2), *spec.Parallelism.Tensor)
	assert.Equal(t, "default", spec.Template.Spec.Containers[0].Image)
	assert.Contains(t, spec.Template.Spec.Containers[0].Args, "--x")
	assert.Contains(t, spec.Template.Spec.Containers[0].Args, "--enable-request-id-headers")
	assert.Equal(t, "true", spec.Template.Spec.Containers[0].Env[0].Value)
}

func TestMergeConfigs_AllOverrideBranches(t *testing.T) {
	base := &servingv1alpha2.LLMInferenceServiceConfigSpec{VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{Args: []string{"base"}}}
	(&LLMInferenceServiceReconciler{}).MergeConfigs(base, &servingv1alpha2.LLMInferenceServiceConfigSpec{
		Template:     &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "merged"}}}},
		VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{Image: "override", Args: []string{"override"}, EnableTurboQuant: true, TurboQuantMetadataPath: "/metadata", Resources: &corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}}},
	})
	require.NotNil(t, base.Template)
	assert.Equal(t, "merged", base.Template.Spec.Containers[0].Image)
	assert.Equal(t, "override", base.VLLMDefaults.Image)
	assert.Equal(t, []string{"base", "override"}, base.VLLMDefaults.Args)
	assert.True(t, base.VLLMDefaults.EnableTurboQuant)
	assert.Equal(t, "/metadata", base.VLLMDefaults.TurboQuantMetadataPath)
	assert.Equal(t, resource.MustParse("2"), base.VLLMDefaults.Resources.Requests[corev1.ResourceCPU])

	base = &servingv1alpha2.LLMInferenceServiceConfigSpec{Template: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{}}}}, VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{Resources: &corev1.ResourceRequirements{}}}
	(&LLMInferenceServiceReconciler{}).MergeConfigs(base, &servingv1alpha2.LLMInferenceServiceConfigSpec{Template: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Env: []corev1.EnvVar{{Name: "new"}}}}}}, VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{Resources: &corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}}}})
	assert.Len(t, base.Template.Spec.Containers[0].Env, 1)
	assert.Equal(t, resource.MustParse("4Gi"), base.VLLMDefaults.Resources.Limits[corev1.ResourceMemory])
	(&LLMInferenceServiceReconciler{}).MergeConfigs(base, nil)
}

func TestGetRerankerWellKnownConfig_ModelAndUnknownIntent(t *testing.T) {
	preset := GetRerankerWellKnownConfig("OCI://BAAI/BGE-RERANKER-V2-M3")
	require.NotNil(t, preset)
	assert.Equal(t, int32(100), preset.MaxCandidates)
	assert.Equal(t, resource.MustParse("8Gi"), preset.Resources.Requests[corev1.ResourceMemory])
	assert.Nil(t, GetRerankerWellKnownConfig("hf://BAAI/bge-reranker-base"))
}

func parallelismExpert(cfg *servingv1alpha2.LLMInferenceServiceConfigSpec) bool {
	return cfg.Parallelism != nil && cfg.Parallelism.Expert
}

func parallelismEPLB(cfg *servingv1alpha2.LLMInferenceServiceConfigSpec) bool {
	return cfg.Parallelism != nil && cfg.Parallelism.EPLBEnabled
}

func wellKnownPtrInt32(value int32) *int32 { return &value }
