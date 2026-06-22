/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

// assertContainsArgPair fails if args does not contain flag immediately followed by value.
func assertContainsArgPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			if args[i+1] != value {
				t.Errorf("arg %s = %q, want %q", flag, args[i+1], value)
			}
			return
		}
	}
	t.Errorf("arg %s not found in %v", flag, args)
}

// assertNotContainsFlag fails if args contains flag.
func assertNotContainsFlag(t *testing.T, args []string, flag string) {
	t.Helper()
	for _, a := range args {
		if a == flag {
			t.Errorf("arg %s should not be present in %v", flag, args)
			return
		}
	}
}

func baseQuantLLMSvc(name string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://test/model",
				Name: "test-model",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "vllm"}},
				},
			},
		},
	}
}

func builderForQuantTest(t *testing.T) *Builder {
	t.Helper()
	return &Builder{Client: fake.NewClientBuilder().Build()}
}

// TestBuilder_Quantization_AWQ verifies --quantization awq is emitted.
func TestBuilder_Quantization_AWQ(t *testing.T) {
	b := builderForQuantTest(t)
	svc := baseQuantLLMSvc("awq-test")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "awq"}

	dep := b.Build(context.Background(), svc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assertContainsArgPair(t, args, "--quantization", "awq")
}

// TestBuilder_Quantization_GPTQ_WithCheckpointPath verifies --quantization gptq
// and --gptq-ckpt-path are both emitted when CheckpointPath is set.
func TestBuilder_Quantization_GPTQ_WithCheckpointPath(t *testing.T) {
	b := builderForQuantTest(t)
	svc := baseQuantLLMSvc("gptq-test")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{
		Method:         "gptq",
		CheckpointPath: "/mnt/models/gptq-ckpt",
	}

	dep := b.Build(context.Background(), svc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assertContainsArgPair(t, args, "--quantization", "gptq")
	assertContainsArgPair(t, args, "--gptq-ckpt-path", "/mnt/models/gptq-ckpt")
}

// TestBuilder_Quantization_BitsAndBytes verifies --quantization bitsandbytes.
func TestBuilder_Quantization_BitsAndBytes(t *testing.T) {
	b := builderForQuantTest(t)
	svc := baseQuantLLMSvc("bnb-test")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "bitsandbytes"}

	dep := b.Build(context.Background(), svc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assertContainsArgPair(t, args, "--quantization", "bitsandbytes")
}

// TestBuilder_Quantization_FP8 verifies --quantization fp8.
func TestBuilder_Quantization_FP8(t *testing.T) {
	b := builderForQuantTest(t)
	svc := baseQuantLLMSvc("fp8-test")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "fp8"}

	dep := b.Build(context.Background(), svc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assertContainsArgPair(t, args, "--quantization", "fp8")
}

// TestBuilder_Quantization_GGUF_UsesQuantCppEngine verifies that GGUF routes
// to the quant-cpp image and does NOT emit --quantization.
func TestBuilder_Quantization_GGUF_UsesQuantCppEngine(t *testing.T) {
	b := builderForQuantTest(t)
	svc := baseQuantLLMSvc("gguf-test")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "gguf"}

	dep := b.Build(context.Background(), svc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)

	c := dep.Spec.Template.Spec.Containers[0]
	if c.Image != api.QuantCppImage {
		t.Errorf("GGUF must use quant-cpp image, got %q, want %q", c.Image, api.QuantCppImage)
	}
	// --quantization must NOT be passed for GGUF (engine handles it via args)
	assertNotContainsFlag(t, c.Args, "--quantization")
}

// TestBuilder_Quantization_GPTQ_WithoutCheckpointPath verifies that
// --gptq-ckpt-path is absent when CheckpointPath is empty.
func TestBuilder_Quantization_GPTQ_WithoutCheckpointPath(t *testing.T) {
	b := builderForQuantTest(t)
	svc := baseQuantLLMSvc("gptq-nopath")
	svc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "gptq"}

	dep := b.Build(context.Background(), svc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assertContainsArgPair(t, args, "--quantization", "gptq")
	assertNotContainsFlag(t, args, "--gptq-ckpt-path")
}

// TestBuilder_Quantization_NilSpec verifies no --quantization arg when spec is nil.
func TestBuilder_Quantization_NilSpec(t *testing.T) {
	b := builderForQuantTest(t)
	svc := baseQuantLLMSvc("no-quant")
	// Quantization is nil by default

	dep := b.Build(context.Background(), svc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assertNotContainsFlag(t, args, "--quantization")
}
