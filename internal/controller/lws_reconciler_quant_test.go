/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// assertLWSArgPair fails if the []interface{} slice does not contain flag immediately
// followed by value. LWS buildVLLMArgs returns []interface{} for JSON marshaling.
func assertLWSArgPair(t *testing.T, args []interface{}, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if s, ok := args[i].(string); ok && s == flag {
			if v, ok2 := args[i+1].(string); ok2 {
				if v != value {
					t.Errorf("LWS arg %s = %q, want %q", flag, v, value)
				}
			} else {
				t.Errorf("LWS arg after %s is not a string: %T", flag, args[i+1])
			}
			return
		}
	}
	t.Errorf("LWS arg %s not found in %v", flag, args)
}

// assertLWSNotContainsFlag fails if flag is present in args.
func assertLWSNotContainsFlag(t *testing.T, args []interface{}, flag string) {
	t.Helper()
	for _, a := range args {
		if s, ok := a.(string); ok && s == flag {
			t.Errorf("LWS arg %s should not be present", flag)
			return
		}
	}
}

func baseLWSLLMSvc(name string) *servingv1alpha2.LLMInferenceService {
	tp := int32(2)
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://test/model", Name: "test-model"},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "vllm"}},
				},
			},
			Parallelism: &servingv1alpha2.ParallelismSpec{
				Tensor: &tp,
			},
		},
	}
}

// TestLWSReconciler_Quantization_AWQ_PropagatedToArgs verifies that AWQ quant
// reaches the LWS vLLM arg list used in multi-node LeaderWorkerSet pods.
func TestLWSReconciler_Quantization_AWQ_PropagatedToArgs(t *testing.T) {
	r := &Reconciler{}
	svc := baseLWSLLMSvc("lws-awq")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "awq"}

	args := r.buildVLLMArgs(svc)
	assertLWSArgPair(t, args, "--quantization", "awq")
}

// TestLWSReconciler_Quantization_GPTQ_WithPath_PropagatedToArgs verifies that
// GPTQ + checkpoint path both appear in the multi-node vLLM args.
func TestLWSReconciler_Quantization_GPTQ_WithPath_PropagatedToArgs(t *testing.T) {
	r := &Reconciler{}
	svc := baseLWSLLMSvc("lws-gptq")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{
		Method:         "gptq",
		CheckpointPath: "/mnt/gptq",
	}

	args := r.buildVLLMArgs(svc)
	assertLWSArgPair(t, args, "--quantization", "gptq")
	assertLWSArgPair(t, args, "--gptq-ckpt-path", "/mnt/gptq")
}

// TestLWSReconciler_Quantization_GGUF_NotPropagatedToArgs verifies that GGUF
// is excluded from LWS args (quant-cpp engine handles GGUF; it is not a valid
// vLLM --quantization value for the LWS distributed serving path).
func TestLWSReconciler_Quantization_GGUF_NotPropagatedToArgs(t *testing.T) {
	r := &Reconciler{}
	svc := baseLWSLLMSvc("lws-gguf")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "gguf"}

	args := r.buildVLLMArgs(svc)
	assertLWSNotContainsFlag(t, args, "--quantization")
}
