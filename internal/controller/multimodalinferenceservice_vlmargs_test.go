/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

// VLM arg tests for MultimodalInferenceService: ImageInputType, ImageProcessorModel,
// and Quantization fields added in the Extended Model Type Support feature branch.
// Uses buildMultimodalDeployment directly (white-box, same package).

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// assertMMArgPair fails if args does not contain flag immediately followed by value.
func assertMMArgPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			if args[i+1] != value {
				t.Errorf("multimodal arg %s = %q, want %q", flag, args[i+1], value)
			}
			return
		}
	}
	t.Errorf("multimodal arg %s not found in %v", flag, args)
}

// assertMMNotContainsFlag fails if flag is present in args.
func assertMMNotContainsFlag(t *testing.T, args []string, flag string) {
	t.Helper()
	for _, a := range args {
		if a == flag {
			t.Errorf("multimodal arg %s should not be present in %v", flag, args)
			return
		}
	}
}

// TestMultimodal_ImageInputType_EmitsArg verifies --image-input-type is present.
func TestMultimodal_ImageInputType_EmitsArg(t *testing.T) {
	svc := newMMSvc("vlm-imgtype", "default", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.ImageInputType = "pixel_values"
	})

	r := &MultimodalInferenceServiceReconciler{}
	dep := r.buildMultimodalDeployment(svc)

	args := dep.Spec.Template.Spec.Containers[0].Args
	assertMMArgPair(t, args, "--image-input-type", "pixel_values")
}

// TestMultimodal_ImageProcessorModel_EmitsArg verifies --image-processor is present.
func TestMultimodal_ImageProcessorModel_EmitsArg(t *testing.T) {
	svc := newMMSvc("vlm-imgproc", "default", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.ImageProcessorModel = "openai/clip-vit-large-patch14"
	})

	r := &MultimodalInferenceServiceReconciler{}
	dep := r.buildMultimodalDeployment(svc)

	args := dep.Spec.Template.Spec.Containers[0].Args
	assertMMArgPair(t, args, "--image-processor", "openai/clip-vit-large-patch14")
}

// TestMultimodal_Quantization_AWQ_EmitsArg verifies --quantization awq is emitted.
func TestMultimodal_Quantization_AWQ_EmitsArg(t *testing.T) {
	svc := newMMSvc("vlm-awq", "default", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "awq"}
	})

	r := &MultimodalInferenceServiceReconciler{}
	dep := r.buildMultimodalDeployment(svc)

	args := dep.Spec.Template.Spec.Containers[0].Args
	assertMMArgPair(t, args, "--quantization", "awq")
}

// TestMultimodal_Quantization_GGUF_NotEmitted verifies that GGUF is not passed
// to vLLM args for multimodal (VLMs cannot use quant-cpp; GGUF is skipped).
func TestMultimodal_Quantization_GGUF_NotEmitted(t *testing.T) {
	svc := newMMSvc("vlm-gguf", "default", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "gguf"}
	})

	r := &MultimodalInferenceServiceReconciler{}
	dep := r.buildMultimodalDeployment(svc)

	args := dep.Spec.Template.Spec.Containers[0].Args
	assertMMNotContainsFlag(t, args, "--quantization")
}

// TestMultimodal_NoImageFields_NoExtraArgs verifies clean baseline
// (no --image-input-type or --image-processor when fields are empty).
func TestMultimodal_NoImageFields_NoExtraArgs(t *testing.T) {
	svc := newMMSvc("vlm-baseline", "default")

	r := &MultimodalInferenceServiceReconciler{}
	dep := r.buildMultimodalDeployment(svc)

	args := dep.Spec.Template.Spec.Containers[0].Args
	assertMMNotContainsFlag(t, args, "--image-input-type")
	assertMMNotContainsFlag(t, args, "--image-processor")
	assertMMNotContainsFlag(t, args, "--quantization")
}
