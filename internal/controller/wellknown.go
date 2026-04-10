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
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

// GetWellKnownConfig returns a predefined configuration for a known model URI.
// Returns nil if the model is not well-known.
func GetWellKnownConfig(modelURI string) *servingv1alpha2.LLMInferenceServiceConfigSpec {
	// Helper to check for Gemma 4 variants regardless of URI scheme (hf://, oci://, swfs://, etc.)
	isGemma4 := func(variant string) bool {
		// Matches patterns like "google/gemma-4-E2B-it", "oci://registry/gemma-4-e2b", "swfs://filer/gemma-4-e2b", etc.
		normalizedMatch := strings.ToLower(modelURI)
		return strings.Contains(normalizedMatch, "gemma-4") &&
			strings.Contains(normalizedMatch, strings.ToLower(variant))
	}

	switch {
	case isGemma4("E2B"):
		// 5B params, Any-to-Any, Dense. Single GPU (8 GB VRAM).
		return &servingv1alpha2.LLMInferenceServiceConfigSpec{
			VLLMDefaults: &servingv1alpha2.VLLMDefaultsSpec{
				Image: api.VLLMGemma4Image,
				Args: []string{
					"--max-model-len", "131072",
					"--trust-remote-code",
					"--enforce-eager",
					"--enable-turboquant",
					"--gpu-memory-utilization", "0.95",
					"--max-num-seqs", "256",
				},
				EnableTurboQuant: true,
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
				Image: api.VLLMGemma4Image,
				Args: []string{
					"--max-model-len", "131072",
					"--trust-remote-code",
					"--enforce-eager",
					"--enable-turboquant",
					"--gpu-memory-utilization", "0.95",
					"--max-num-seqs", "128",
				},
				EnableTurboQuant: true,
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
				Image: api.VLLMGemma4Image,
				Args: []string{
					"--max-model-len", "65536",
					"--trust-remote-code",
					"--enforce-eager",
					"--enable-turboquant",
				},
				EnableTurboQuant: true,
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
				Image: api.VLLMGemma4Image,
				Args: []string{
					"--max-model-len", "65536",
					"--trust-remote-code",
					"--enforce-eager",
					"--enable-turboquant",
				},
				EnableTurboQuant: true,
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
	}

	return nil
}

// ApplyConfigToSpec applies a configuration spec to an LLMInferenceServiceSpec.
func (r *LLMInferenceServiceReconciler) ApplyConfigToSpec(spec *servingv1alpha2.LLMInferenceServiceSpec, cfg *servingv1alpha2.LLMInferenceServiceConfigSpec) {
	if cfg == nil {
		return
	}

	if cfg.Parallelism != nil {
		if spec.Parallelism == nil {
			spec.Parallelism = cfg.Parallelism.DeepCopy()
		} else {
			// Merge parallelism fields - ONLY if not already set by user
			if cfg.Parallelism.Tensor != nil && spec.Parallelism.Tensor == nil {
				spec.Parallelism.Tensor = cfg.Parallelism.Tensor
			}
			if cfg.Parallelism.Data != nil && spec.Parallelism.Data == nil {
				spec.Parallelism.Data = cfg.Parallelism.Data
			}
			// Expert parallelism is a boolean flag, if WellKnown says true and user hasn't set it (implicit false),
			// we can't easily distinguish "User set false" vs "User didn't set".
			// But usually, if the user didn't specify Parallelism at all, we use defaults.
			// If they DID specify something, we respect their explicit choice for Expert.
		}
	}

	if cfg.Scaling != nil {
		if spec.Scaling == nil {
			spec.Scaling = cfg.Scaling.DeepCopy()
		}
	}

	if cfg.Template != nil {
		mergePodSpec(&spec.Template.Spec, &cfg.Template.Spec)
	}

	if cfg.VLLMDefaults != nil {
		// Apply defaults to the primary container (index 0)
		if len(spec.Template.Spec.Containers) > 0 {
			c := &spec.Template.Spec.Containers[0]
			if cfg.VLLMDefaults.Image != "" && c.Image == "" {
				c.Image = cfg.VLLMDefaults.Image
			}
			if len(cfg.VLLMDefaults.Args) > 0 {
				// Only append args that aren't already present
				for _, arg := range cfg.VLLMDefaults.Args {
					found := false
					conflicted := false

					for _, existing := range c.Args {
						if existing == arg {
							found = true
							break
						}
						// Specific logic for A/B testing: suppress --enable if --disable is present
						if arg == "--enable-turboquant" && existing == "--disable-turboquant" {
							conflicted = true
							break
						}
					}
					if !found && !conflicted {
						c.Args = append(c.Args, arg)
					}
				}
			}
			if cfg.VLLMDefaults.Resources != nil {
				mergeResources(&c.Resources, cfg.VLLMDefaults.Resources)
			}
			// Phase 4: Handle TurboQuant env injection
			if cfg.VLLMDefaults.EnableTurboQuant {
				// Only inject if not already overridden by user
				found := false
				for _, e := range c.Env {
					if e.Name == "VLLM_TURBOQUANT" {
						found = true
						break
					}
				}
				if !found {
					c.Env = append(c.Env, corev1.EnvVar{Name: "VLLM_TURBOQUANT", Value: "true"})
				}
			}
		}
	}
}

// MergeConfigs merges two configuration specs. base is updated with values from override.
func (r *LLMInferenceServiceReconciler) MergeConfigs(base, override *servingv1alpha2.LLMInferenceServiceConfigSpec) {
	if override == nil {
		return
	}

	if override.Template != nil {
		if base.Template == nil {
			base.Template = override.Template.DeepCopy()
		} else {
			mergePodSpec(&base.Template.Spec, &override.Template.Spec)
		}
	}

	if override.VLLMDefaults != nil {
		if base.VLLMDefaults == nil {
			base.VLLMDefaults = override.VLLMDefaults.DeepCopy()
		} else {
			if override.VLLMDefaults.Image != "" {
				base.VLLMDefaults.Image = override.VLLMDefaults.Image
			}
			if len(override.VLLMDefaults.Args) > 0 {
				base.VLLMDefaults.Args = append(base.VLLMDefaults.Args, override.VLLMDefaults.Args...)
			}
			if override.VLLMDefaults.EnableTurboQuant {
				base.VLLMDefaults.EnableTurboQuant = true
			}
			if override.VLLMDefaults.TurboQuantMetadataPath != "" {
				base.VLLMDefaults.TurboQuantMetadataPath = override.VLLMDefaults.TurboQuantMetadataPath
			}
			if override.VLLMDefaults.Resources != nil {
				if base.VLLMDefaults.Resources == nil {
					base.VLLMDefaults.Resources = override.VLLMDefaults.Resources.DeepCopy()
				} else {
					mergeResources(base.VLLMDefaults.Resources, override.VLLMDefaults.Resources)
				}
			}
		}
	}
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
