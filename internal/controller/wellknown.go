/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// GetWellKnownConfig returns a predefined configuration for a known model URI.
// Returns nil if the model is not well-known.
func GetWellKnownConfig(modelURI string) *servingv1alpha2.LLMInferenceServiceConfigSpec {
	// Helper to check for Gemma 4 variants regardless of URI scheme (hf://, oci://, swfs://, etc.)
	// Matchers use token-boundary comparison (containsToken): a preset fires
	// only when both the family and the size appear as standalone version
	// tokens. Raw substring matching misfires on storage URIs whose directory
	// name embeds a family as a prefix — "pvc://qwen38-27b-weights" contains
	// "qwen3" (inside "qwen38") and "7b" (inside "27b") and used to trigger
	// the Qwen3-7B preset, whose merged default args crashed the pod.
	isGemma4 := func(variant string) bool {
		// Matches patterns like "google/gemma-4-E2B-it", "oci://registry/gemma-4-e2b", "swfs://filer/gemma-4-e2b", etc.
		return containsToken(modelURI, "gemma-4") && containsToken(modelURI, variant)
	}
	isDeepSeek := func(variant string) bool {
		return containsToken(modelURI, "deepseek") && containsToken(modelURI, variant)
	}
	isQwen3 := func(size string) bool {
		return containsToken(modelURI, "qwen3") && containsToken(modelURI, size)
	}
	qwen3ToolArgs := func() []string {
		norm := strings.ToLower(modelURI)
		parser := "hermes"
		if strings.Contains(norm, "coder") {
			parser = "qwen3_coder"
		}
		args := []string{"--enable-auto-tool-choice", "--tool-call-parser", parser}
		if strings.Contains(norm, "coder") || strings.Contains(norm, "thinking") || strings.Contains(norm, "reasoning") {
			args = append(args, "--reasoning-parser", "qwen3")
		}
		return args
	}
	toolArgs := func(parser string) []string {
		return []string{"--enable-auto-tool-choice", "--tool-call-parser", parser}
	}

	switch {
	case isGemma4("E2B"):
		// 5B params, Any-to-Any, Dense. Single GPU (8 GB VRAM).
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: append([]string{
					"--max-model-len", "131072",
					"--trust-remote-code",
					"--enforce-eager",
					"--gpu-memory-utilization", "0.95",
					"--max-num-seqs", "256",
				}, toolArgs("hermes")...),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
				},
			},
		}
	case isGemma4("E4B"):
		// 8B params, Any-to-Any, Dense. Single GPU (16 GB VRAM).
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: append([]string{
					"--max-model-len", "131072",
					"--trust-remote-code",
					"--enforce-eager",
					"--gpu-memory-utilization", "0.95",
					"--max-num-seqs", "128",
				}, toolArgs("hermes")...),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("16"),
						corev1.ResourceMemory: resource.MustParse("64Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("16"),
						corev1.ResourceMemory: resource.MustParse("64Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
				},
			},
		}
	case isGemma4("26B-A4B"):
		// 27B total / 4B active params. MoE architecture! Expert parallelism.
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			Parallelism: &servingv1alpha2.ParallelismSpec{
				Expert: true, // Enable MoE expert routing
			},
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: append([]string{
					"--max-model-len", "65536",
					"--trust-remote-code",
					"--enforce-eager",
				}, toolArgs("hermes")...),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("32"),
						corev1.ResourceMemory: resource.MustParse("128Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("32"),
						corev1.ResourceMemory: resource.MustParse("128Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
				},
			},
		}
	case isGemma4("31B"):
		// 33B params, Dense. Needs 2× GPU with TP=2.
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			Parallelism: &servingv1alpha2.ParallelismSpec{
				Tensor: ptr.To(int32(2)),
			},
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: append([]string{
					"--max-model-len", "65536",
					"--trust-remote-code",
					"--enforce-eager",
				}, toolArgs("hermes")...),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("32"),
						corev1.ResourceMemory: resource.MustParse("256Gi"),
						"nvidia.com/gpu":      resource.MustParse("2"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("32"),
						corev1.ResourceMemory: resource.MustParse("256Gi"),
						"nvidia.com/gpu":      resource.MustParse("2"),
					},
				},
			},
		}
	case strings.Contains(modelURI, "meta-llama/Llama-3.1-8B-it"):
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: []string{
					"--max-model-len", "32768",
					"--trust-remote-code",
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
				},
			},
		}
	case strings.Contains(strings.ToLower(modelURI), "llama-4"):
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: append([]string{
					"--max-model-len", "32768",
					"--trust-remote-code",
				}, toolArgs("llama4_json")...),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("16"),
						corev1.ResourceMemory: resource.MustParse("64Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
				},
			},
		}
	case strings.Contains(modelURI, "meta-llama/Llama-3.1-70B-it"):
		tps := int32(4)
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			Parallelism: &servingv1alpha2.ParallelismSpec{
				Tensor: &tps,
			},
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: []string{
					"--max-model-len", "16384",
					"--trust-remote-code",
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("32"),
						corev1.ResourceMemory: resource.MustParse("256Gi"),
						"nvidia.com/gpu":      resource.MustParse("4"),
					},
				},
			},
		}
	case strings.Contains(modelURI, "mistralai/Mistral-7B-v0.3"):
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: []string{
					"--max-model-len", "32768",
					"--tokenizer-mode", "mistral",
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
				},
			},
		}
	case isDeepSeek("V4") || isDeepSeek("V3.2"):
		// DeepSeek-V4: 671B sparse MoE, 37B active. EPLB mandatory for balanced expert load.
		// FP8 KV cache halves VRAM vs BF16; 8×H100 required.
		tp := int32(8)
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			Parallelism: &servingv1alpha2.ParallelismSpec{
				Tensor:      &tp,
				Expert:      true,
				EPLBEnabled: true,
			},
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: []string{
					"--max-model-len", "8192",
					"--trust-remote-code",
					"--enable-chunked-prefill",
					"--kv-cache-dtype", "fp8",
					"--gpu-memory-utilization", "0.90",
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("64"),
						corev1.ResourceMemory: resource.MustParse("512Gi"),
						"nvidia.com/gpu":      resource.MustParse("8"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("64"),
						corev1.ResourceMemory: resource.MustParse("512Gi"),
						"nvidia.com/gpu":      resource.MustParse("8"),
					},
				},
			},
		}

	case isQwen3("72B"):
		// Qwen3-72B dense; MRv2 is the v0.24.0 default. 4x A100/H100 with TP=4.
		tp := int32(4)
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			Parallelism: &servingv1alpha2.ParallelismSpec{Tensor: &tp},
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: append([]string{
					"--max-model-len", "32768",
					"--trust-remote-code",
					"--gpu-memory-utilization", "0.90",
				}, qwen3ToolArgs()...),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("32"),
						corev1.ResourceMemory: resource.MustParse("256Gi"),
						"nvidia.com/gpu":      resource.MustParse("4"),
					},
				},
			},
		}

	case isQwen3("8B") || isQwen3("7B"):
		// Qwen3-8B/7B dense; single GPU (16 GB VRAM), MRv2 is the v0.24.0 default.
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Args: append([]string{
					"--max-model-len", "32768",
					"--trust-remote-code",
					"--gpu-memory-utilization", "0.90",
				}, qwen3ToolArgs()...),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
						"nvidia.com/gpu":      resource.MustParse("1"),
					},
				},
			},
		}
	case containsToken(modelURI, "qwen3"):
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{Args: qwen3ToolArgs()},
		}
	}

	return nil
}

func mergePodSpec(base, override *corev1.PodSpec) {
	if len(override.Containers) > 0 && len(base.Containers) > 0 {
		// Only primary container (vllm) is typically merged here.
		baseC := &base.Containers[0]
		overC := override.Containers[0]

		if overC.Image != "" {
			baseC.Image = overC.Image
		}
		if len(overC.Env) > 0 {
			baseC.Env = append(baseC.Env, overC.Env...)
		}
		if overC.Resources.Limits != nil || overC.Resources.Requests != nil {
			mergeResources(&baseC.Resources, &overC.Resources)
		}
	}
}

func mergeResources(base, defaultResources *corev1.ResourceRequirements) {
	if defaultResources.Requests != nil {
		if base.Requests == nil {
			base.Requests = make(corev1.ResourceList)
		}
		// Default-if-Missing: Only add resources that the user hasn't specified
		for k, v := range defaultResources.Requests {
			if _, exists := base.Requests[k]; !exists {
				base.Requests[k] = v
			}
		}
	}
	if defaultResources.Limits != nil {
		if base.Limits == nil {
			base.Limits = make(corev1.ResourceList)
		}
		// Default-if-Missing: Only add limits that the user hasn't specified
		for k, v := range defaultResources.Limits {
			if _, exists := base.Limits[k]; !exists {
				base.Limits[k] = v
			}
		}
	}
}

// GetRerankerWellKnownConfig returns resource defaults for known cross-encoder reranker models.
// Returns nil for unknown models; the controller applies its generic defaults.
func GetRerankerWellKnownConfig(modelURI string) *servingv1alpha2.RerankerInferenceServiceSpec {
	norm := strings.ToLower(modelURI)
	switch {
	case strings.Contains(norm, "bge-reranker-v2-m3"):
		// BAAI/bge-reranker-v2-m3: 568M params, multilingual cross-encoder. 1 GPU, ~8 GiB VRAM.
		return &servingv1alpha2.RerankerInferenceServiceSpec{
			MaxCandidates: 100,
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					"nvidia.com/gpu":      resource.MustParse("1"),
				},
			},
		}
	}
	return nil
}
