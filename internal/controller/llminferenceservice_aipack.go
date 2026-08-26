/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (r *LLMInferenceServiceReconciler) applyEarlyAIPackConfig(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) {
	logger := log.FromContext(ctx)
	var list servingv1alpha2.AIPackList
	if err := r.List(ctx, &list, client.InNamespace(llmSvc.Namespace)); err != nil {
		logger.Error(err, "failed to list AIPacks for pre-deployment injection (non-blocking)")
		return
	}
	applyAIPackConfig(llmSvc, matchingAIPacks(llmSvc.Name, list.Items))
}

func matchingAIPacks(workload string, packs []servingv1alpha2.AIPack) []servingv1alpha2.AIPack {
	active := make([]servingv1alpha2.AIPack, 0, len(packs))
	for _, pack := range packs {
		if pack.Labels["serving.ckodex.com/workload"] == workload {
			active = append(active, pack)
		}
	}
	return active
}

// applyAIPackConfig injects BaseModel quantization metadata from governance-bound AIPacks.
func applyAIPackConfig(llmSvc *servingv1alpha2.LLMInferenceService, packs []servingv1alpha2.AIPack) {
	if llmSvc.Spec.Quantization != nil {
		return
	}
	for i := range packs {
		pack := &packs[i]
		if pack.Spec.Kind != servingv1alpha2.KindBaseModel || pack.Spec.BaseModel == nil {
			continue
		}
		if method := normalizeQuantization(pack.Spec.BaseModel.Quantization); method != "" {
			llmSvc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: method}
			return
		}
	}
}

// normalizeQuantization maps AIPack quantization strings to inference methods.
func normalizeQuantization(q string) string {
	switch q {
	case "awq", "int4-awq", "w4a16", "int4":
		return "awq"
	case "gptq", "int4-gptq":
		return "gptq"
	case "gguf":
		return "gguf"
	case "bitsandbytes", "bnb", "int8":
		return "bitsandbytes"
	case "fp8", "w8a8", "sq", "smoothquant":
		return "fp8"
	default:
		return ""
	}
}
