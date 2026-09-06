/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
)

// containsToken reports whether token occurs in s as a standalone version
// token: the characters immediately before and after a match must not extend
// it into a longer alphanumeric run. "qwen38-27b" therefore does NOT contain
// the tokens "qwen3" (followed by '8') or "7b" (preceded by '2'), while
// "qwen3-7b" contains both. Preset matching on storage URIs (pvc://, oci://)
// encodes the model in a directory name, so a raw substring match misfires on
// names that merely embed a known model family as a prefix.
func containsToken(s, token string) bool {
	lower := strings.ToLower(s)
	needle := strings.ToLower(token)
	for i := 0; i <= len(lower)-len(needle); {
		at := strings.Index(lower[i:], needle)
		if at < 0 {
			return false
		}
		at += i
		before := at == 0 || !isAlnum(lower[at-1])
		after := at+len(needle) == len(lower) || !isAlnum(lower[at+len(needle)])
		if before && after {
			return true
		}
		i = at + 1
	}
	return false
}

func isAlnum(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// mergePresetArgs appends preset args whose flag is not already present in
// existing. The merge is flag-aware across the three CLI forms: "--flag=value"
// (equals form), "--flag value" (split pair), and bare "--flag" switches.
// Item-wise string dedupe is wrong here twice over: a preset split-pair value
// ("32768") whose flag was deduped gets appended bare — vLLM then dies with
// "unrecognized arguments" — and a preset split flag ("--tool-call-parser")
// is not matched by an existing equals-form arg ("--tool-call-parser=qwen3_xml"),
// so both the flag and its value are appended, silently overriding the user's
// parser. A preset split pair is therefore appended atomically with its value.
func mergePresetArgs(existing, preset []string) []string {
	merged := append([]string(nil), existing...)
	for i := 0; i < len(preset); i++ {
		arg := preset[i]
		if !strings.HasPrefix(arg, "-") {
			continue // value items are consumed together with their flag
		}
		eq := strings.Index(arg, "=")
		flag := arg
		if eq >= 0 {
			flag = arg[:eq]
		}
		if hasArgName(merged, flag) {
			continue // the user-supplied form wins
		}
		// Preserve the turboquant A/B conflict rule: an explicit user opt-out
		// suppresses the preset opt-in even though the flag names differ.
		if flag == "--enable-turboquant" && hasArgName(merged, "--disable-turboquant") {
			continue
		}
		if eq >= 0 {
			merged = append(merged, arg)
			continue
		}
		if i+1 < len(preset) && !strings.HasPrefix(preset[i+1], "-") {
			merged = append(merged, arg, preset[i+1])
			i++
			continue
		}
		merged = append(merged, arg)
	}
	return merged
}

func hasArgName(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

// ApplyConfigToSpec applies a well-known configuration to an
// LLMInferenceServiceSpec. Every merge respects user-supplied values: fields
// the user set are never overwritten, and preset args are merged flag-aware
// (see mergePresetArgs) so a preset can neither orphan a value nor duplicate
// a flag the user already supplied in a different CLI form.
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
				c.Args = mergePresetArgs(c.Args, cfg.VLLMDefaults.Args)
			}
			if cfg.VLLMDefaults.Resources != nil {
				mergeResources(&c.Resources, cfg.VLLMDefaults.Resources)
			}
			if cfg.VLLMDefaults.EnableTurboQuant {
				if !hasEnv(c.Env, "VLLM_TURBOQUANT") {
					c.Env = append(c.Env, corev1.EnvVar{Name: "VLLM_TURBOQUANT", Value: "true"})
				}
			}
			// Inject request IDs for the vLLM v0.24.0 Rust frontend.
			if !hasArgName(c.Args, "--enable-request-id-headers") {
				c.Args = append(c.Args, "--enable-request-id-headers")
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

func hasEnv(env []corev1.EnvVar, name string) bool {
	for _, e := range env {
		if e.Name == name {
			return true
		}
	}
	return false
}
