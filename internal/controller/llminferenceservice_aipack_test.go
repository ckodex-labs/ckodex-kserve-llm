/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

// Unit tests for applyAIPackConfig and normalizeQuantization.
// Both are package-level functions (no receiver) so they are directly callable.

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// --- helpers -----------------------------------------------------------------

func baseLLMSvcForAIPackTest(name string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://test/model", Name: "test-model"},
		},
	}
}

func baseModelPack(name, quantization string) servingv1alpha2.AIPack {
	return servingv1alpha2.AIPack{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.AIPackSpec{
			Kind:      servingv1alpha2.KindBaseModel,
			BaseModel: &servingv1alpha2.BaseModelSpec{Quantization: quantization},
		},
	}
}

// --- applyAIPackConfig -------------------------------------------------------

// TestApplyAIPackConfig_UserWins_NilGuard verifies that a user-set Quantization
// is never overwritten by an AIPack BaseModel quant value.
func TestApplyAIPackConfig_UserWins_NilGuard(t *testing.T) {
	svc := baseLLMSvcForAIPackTest("svc-user-quant")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "fp8"}

	packs := []servingv1alpha2.AIPack{baseModelPack("pack", "awq")}
	applyAIPackConfig(svc, packs)

	if svc.Spec.Quantization.Method != "fp8" {
		t.Errorf("user Quantization.Method = %q, want fp8 (user value must win)", svc.Spec.Quantization.Method)
	}
}

// TestApplyAIPackConfig_Int4AWQ_MapsToAWQ verifies int4-awq AIPack string → awq Method.
func TestApplyAIPackConfig_Int4AWQ_MapsToAWQ(t *testing.T) {
	svc := baseLLMSvcForAIPackTest("svc-int4awq")

	packs := []servingv1alpha2.AIPack{baseModelPack("pack", "int4-awq")}
	applyAIPackConfig(svc, packs)

	if svc.Spec.Quantization == nil {
		t.Fatal("Quantization is nil after injection")
	}
	if svc.Spec.Quantization.Method != "awq" {
		t.Errorf("Method = %q, want awq", svc.Spec.Quantization.Method)
	}
}

// TestApplyAIPackConfig_BF16_Skipped verifies that bf16 (training precision,
// not a vLLM --quantization value) results in no Quantization being injected.
func TestApplyAIPackConfig_BF16_Skipped(t *testing.T) {
	svc := baseLLMSvcForAIPackTest("svc-bf16")

	packs := []servingv1alpha2.AIPack{baseModelPack("pack", "bf16")}
	applyAIPackConfig(svc, packs)

	if svc.Spec.Quantization != nil {
		t.Errorf("bf16 must not set Quantization; got Method=%q", svc.Spec.Quantization.Method)
	}
}

// TestApplyAIPackConfig_W4A16_MapsToAWQ verifies w4a16 → awq (AWQ alias used by
// some model hubs to describe 4-bit weight-only quantization).
func TestApplyAIPackConfig_W4A16_MapsToAWQ(t *testing.T) {
	svc := baseLLMSvcForAIPackTest("svc-w4a16")

	packs := []servingv1alpha2.AIPack{baseModelPack("pack", "w4a16")}
	applyAIPackConfig(svc, packs)

	if svc.Spec.Quantization == nil {
		t.Fatal("Quantization is nil after w4a16 injection")
	}
	if svc.Spec.Quantization.Method != "awq" {
		t.Errorf("Method = %q, want awq", svc.Spec.Quantization.Method)
	}
}

// --- normalizeQuantization ---------------------------------------------------

// TestNormalizeQuantization_AllMappings exercises every input variant documented
// in llminferenceservice_controller.go to guard against regressions in the mapping.
func TestNormalizeQuantization_AllMappings(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// AWQ family
		{"awq", "awq"},
		{"int4-awq", "awq"},
		{"w4a16", "awq"},
		{"int4", "awq"},
		// GPTQ family
		{"gptq", "gptq"},
		{"int4-gptq", "gptq"},
		// GGUF
		{"gguf", "gguf"},
		// BitsAndBytes family
		{"bitsandbytes", "bitsandbytes"},
		{"bnb", "bitsandbytes"},
		{"int8", "bitsandbytes"},
		// FP8 family
		{"fp8", "fp8"},
		{"w8a8", "fp8"},
		{"sq", "fp8"},
		{"smoothquant", "fp8"},
		// Training precisions — skipped
		{"bf16", ""},
		{"bfloat16", ""},
		{"fp32", ""},
		// Completely unknown
		{"unknown-format", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeQuantization(tc.input)
			if got != tc.want {
				t.Errorf("normalizeQuantization(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
