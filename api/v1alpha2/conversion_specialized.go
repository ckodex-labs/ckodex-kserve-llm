/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
)

func convertExperimentalToV1(src *LLMInferenceService, dst *servingv1.LLMInferenceService) {
	if src.Spec.Prefill == nil && src.Spec.Worker == nil && src.Spec.KVCache == nil &&
		src.Spec.SpeculativeDecoding == nil && src.Spec.Quantization == nil &&
		src.Spec.Engine == "" && src.Spec.ToolSurface == nil && src.Spec.Observability == nil {
		return
	}
	dst.Spec.Experimental = &servingv1.ExperimentalSpec{}
	if src.Spec.Prefill != nil {
		dst.Spec.Experimental.Prefill = &servingv1.PrefillSpec{Replicas: src.Spec.Prefill.Replicas, Template: src.Spec.Prefill.Template}
	}
	if src.Spec.Worker != nil {
		dst.Spec.Experimental.Worker = &servingv1.WorkerSpec{Template: src.Spec.Worker.Template}
	}
	if src.Spec.KVCache != nil {
		dst.Spec.Experimental.KVCache = convertKVCacheToV1(src.Spec.KVCache)
	}
	dst.Spec.Experimental.SpeculativeDecoding = convertSpeculativeDecodingToV1(src.Spec.SpeculativeDecoding)
	dst.Spec.Experimental.Quantization = convertQuantizationToV1(src.Spec.Quantization)
	dst.Spec.Experimental.Engine = src.Spec.Engine
	dst.Spec.Experimental.ToolSurface = convertToolSurfaceToV1(src.Spec.ToolSurface)
	dst.Spec.Experimental.Observability = convertObservabilityToV1(src.Spec.Observability)
}

func convertExperimentalFromV1(src *servingv1.LLMInferenceService, dst *LLMInferenceService) {
	if src.Spec.Experimental == nil {
		return
	}
	if src.Spec.Experimental.Prefill != nil {
		dst.Spec.Prefill = &PrefillSpec{Replicas: src.Spec.Experimental.Prefill.Replicas, Template: src.Spec.Experimental.Prefill.Template}
	}
	if src.Spec.Experimental.Worker != nil {
		dst.Spec.Worker = &WorkerSpec{Template: src.Spec.Experimental.Worker.Template}
	}
	if src.Spec.Experimental.KVCache != nil {
		dst.Spec.KVCache = convertKVCacheFromV1(src.Spec.Experimental.KVCache)
	}
	dst.Spec.SpeculativeDecoding = convertSpeculativeDecodingFromV1(src.Spec.Experimental.SpeculativeDecoding)
	dst.Spec.Quantization = convertQuantizationFromV1(src.Spec.Experimental.Quantization)
	dst.Spec.Engine = src.Spec.Experimental.Engine
	dst.Spec.ToolSurface = convertToolSurfaceFromV1(src.Spec.Experimental.ToolSurface)
	dst.Spec.Observability = convertObservabilityFromV1(src.Spec.Experimental.Observability)
}

func convertKVCacheToV1(src *KVCacheSpec) *servingv1.KVCacheSpec {
	if src == nil {
		return nil
	}
	dst := &servingv1.KVCacheSpec{Dtype: src.Dtype, SwapSpaceGB: src.SwapSpaceGB}
	if src.Transfer != nil {
		dst.Transfer = &servingv1.KVTransferSpec{Connector: src.Transfer.Connector, Role: src.Transfer.Role, ExtraConfig: deepCopyStringMap(src.Transfer.ExtraConfig), Env: deepCopyEnv(src.Transfer.Env)}
		if src.Transfer.LMCache != nil {
			dst.Transfer.LMCache = &servingv1.LMCacheSpec{
				Mode:            servingv1.LMCacheMode(src.Transfer.LMCache.Mode),
				ChunkSize:       src.Transfer.LMCache.ChunkSize,
				LocalCPU:        src.Transfer.LMCache.LocalCPU,
				LocalCPUSizeGiB: src.Transfer.LMCache.LocalCPUSizeGiB,
			}
			if src.Transfer.LMCache.EngineRef != nil {
				dst.Transfer.LMCache.EngineRef = src.Transfer.LMCache.EngineRef.DeepCopy()
			}
		}
	}
	return dst
}

func convertKVCacheFromV1(src *servingv1.KVCacheSpec) *KVCacheSpec {
	if src == nil {
		return nil
	}
	dst := &KVCacheSpec{Dtype: src.Dtype, SwapSpaceGB: src.SwapSpaceGB}
	if src.Transfer != nil {
		dst.Transfer = &KVTransferSpec{Connector: src.Transfer.Connector, Role: src.Transfer.Role, ExtraConfig: deepCopyStringMap(src.Transfer.ExtraConfig), Env: deepCopyEnv(src.Transfer.Env)}
		if src.Transfer.LMCache != nil {
			dst.Transfer.LMCache = &LMCacheSpec{
				Mode:            LMCacheMode(src.Transfer.LMCache.Mode),
				ChunkSize:       src.Transfer.LMCache.ChunkSize,
				LocalCPU:        src.Transfer.LMCache.LocalCPU,
				LocalCPUSizeGiB: src.Transfer.LMCache.LocalCPUSizeGiB,
			}
			if src.Transfer.LMCache.EngineRef != nil {
				dst.Transfer.LMCache.EngineRef = src.Transfer.LMCache.EngineRef.DeepCopy()
			}
		}
	}
	return dst
}

func convertSpeculativeDecodingToV1(src *SpeculativeDecodingSpec) *servingv1.SpeculativeDecodingSpec {
	if src == nil {
		return nil
	}
	return &servingv1.SpeculativeDecodingSpec{Method: src.Method, NumTokens: src.NumTokens, DraftModel: src.DraftModel}
}

func convertSpeculativeDecodingFromV1(src *servingv1.SpeculativeDecodingSpec) *SpeculativeDecodingSpec {
	if src == nil {
		return nil
	}
	return &SpeculativeDecodingSpec{Method: src.Method, NumTokens: src.NumTokens, DraftModel: src.DraftModel}
}

func convertQuantizationToV1(src *QuantizationSpec) *servingv1.QuantizationSpec {
	if src == nil {
		return nil
	}
	return &servingv1.QuantizationSpec{Method: src.Method, CheckpointPath: src.CheckpointPath}
}

func convertQuantizationFromV1(src *servingv1.QuantizationSpec) *QuantizationSpec {
	if src == nil {
		return nil
	}
	return &QuantizationSpec{Method: src.Method, CheckpointPath: src.CheckpointPath}
}

func convertToolSurfaceToV1(src *ToolSurface) *servingv1.ToolSurface {
	if src == nil {
		return nil
	}
	return &servingv1.ToolSurface{AllowedAPIs: append([]string(nil), src.AllowedAPIs...), AllowedCIDRs: append([]string(nil), src.AllowedCIDRs...)}
}

func convertToolSurfaceFromV1(src *servingv1.ToolSurface) *ToolSurface {
	if src == nil {
		return nil
	}
	return &ToolSurface{AllowedAPIs: append([]string(nil), src.AllowedAPIs...), AllowedCIDRs: append([]string(nil), src.AllowedCIDRs...)}
}

func convertObservabilityToV1(src *ObservabilitySpec) *servingv1.ObservabilitySpec {
	if src == nil {
		return nil
	}
	dst := &servingv1.ObservabilitySpec{}
	if src.Sink != nil {
		dst.Sink = &servingv1.TelemetrySink{Type: src.Sink.Type, Endpoint: src.Sink.Endpoint}
	}
	return dst
}

func convertObservabilityFromV1(src *servingv1.ObservabilitySpec) *ObservabilitySpec {
	if src == nil {
		return nil
	}
	dst := &ObservabilitySpec{}
	if src.Sink != nil {
		dst.Sink = &TelemetrySink{Type: src.Sink.Type, Endpoint: src.Sink.Endpoint}
	}
	return dst
}

func deepCopyEnv(src []corev1.EnvVar) []corev1.EnvVar {
	if src == nil {
		return nil
	}
	dst := make([]corev1.EnvVar, len(src))
	for i := range src {
		src[i].DeepCopyInto(&dst[i])
	}
	return dst
}

func deepCopyStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func deepCopyStrings(src []string) []string {
	if src == nil {
		return nil
	}
	return append([]string(nil), src...)
}
