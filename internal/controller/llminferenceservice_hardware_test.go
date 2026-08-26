/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
)

// --- detectHardware tests ---

type hardwareDetectionCase struct {
	name     string
	nodes    []corev1.Node
	expected deployment.HardwareType
}

var hardwareDetectionCases = []hardwareDetectionCase{
	{
		name:     "empty node list returns Unknown",
		nodes:    nil,
		expected: deployment.HardwareUnknown,
	},
	{
		name: "ARM64 node without GPU returns AppleSilicon",
		nodes: []corev1.Node{
			nodeWithArch("arm64", nil, nil),
		},
		expected: deployment.HardwareAppleSilicon,
	},
	{
		name: "ARM64 node with apple.com/gpu capacity returns AppleSiliconMPS",
		nodes: []corev1.Node{
			nodeWithArch("arm64", map[corev1.ResourceName]resource.Quantity{
				"apple.com/gpu": resource.MustParse("1"),
			}, nil),
		},
		expected: deployment.HardwareAppleSiliconMPS,
	},
	{
		name: "ARM64 node with apple.com/gpu.present label returns AppleSiliconMPS",
		nodes: []corev1.Node{
			nodeWithArch("arm64", nil, map[string]string{
				"apple.com/gpu.present": "true",
			}),
		},
		expected: deployment.HardwareAppleSiliconMPS,
	},
	{
		name: "AMD64 node without GPU returns GenericX86",
		nodes: []corev1.Node{
			nodeWithArch("amd64", nil, nil),
		},
		expected: deployment.HardwareGenericX86,
	},
	{
		name: "node with nvidia.com/gpu capacity returns NVIDIA",
		nodes: []corev1.Node{
			nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
				"nvidia.com/gpu": resource.MustParse("1"),
			}, nil),
		},
		expected: deployment.HardwareNVIDIA,
	},
	{
		name: "node with nvidia.com/gpu.present label returns NVIDIA",
		nodes: []corev1.Node{
			nodeWithArch("amd64", nil, map[string]string{
				"nvidia.com/gpu.present": "true",
			}),
		},
		expected: deployment.HardwareNVIDIA,
	},
	{
		name: "node with amd.com/gpu capacity returns AMD",
		nodes: []corev1.Node{
			nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
				"amd.com/gpu": resource.MustParse("1"),
			}, nil),
		},
		expected: deployment.HardwareAMD,
	},
	{
		name: "mixed cluster ARM64 + NVIDIA picks highest priority (NVIDIA)",
		nodes: []corev1.Node{
			nodeWithArch("arm64", nil, nil),
			nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
				"nvidia.com/gpu": resource.MustParse("2"),
			}, nil),
		},
		expected: deployment.HardwareNVIDIA,
	},
	{
		name: "mixed cluster AMD GPU + x86 CPU picks AMD",
		nodes: []corev1.Node{
			nodeWithArch("amd64", nil, nil),
			nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
				"amd.com/gpu": resource.MustParse("1"),
			}, nil),
		},
		expected: deployment.HardwareAMD,
	},
}

func TestDetectHardware(t *testing.T) {
	for _, tt := range hardwareDetectionCases {
		t.Run(tt.name, func(t *testing.T) {
			got := deployment.DetectHardware(tt.nodes)
			if got != tt.expected {
				t.Errorf("detectHardware() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// --- applyHardwareOptimizations tests ---

func TestApplyHardwareOptimizations_AppleSilicon(t *testing.T) {
	ctx := context.Background()
	podSpec := basePodSpec()
	hwType := deployment.DetectHardware([]corev1.Node{nodeWithArch("arm64", nil, nil)})
	deployment.ApplyHardwareOptimizations(ctx, hwType, podSpec)

	// Image should be set to the generic CPU image on Apple Silicon.
	if podSpec.Containers[0].Image != deployment.VLLMGenericImage {
		t.Errorf("image = %q, want %q", podSpec.Containers[0].Image, deployment.VLLMGenericImage)
	}

	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_OMP_THREADS_BIND", "nobind")
	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_KVCACHE_SPACE", "4")
	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_TARGET_DEVICE", "cpu")
	assertArgPair(t, podSpec.Containers[0].Args, "--host", "0.0.0.0")
	assertArgPair(t, podSpec.Containers[0].Args, "--port", "8000")
	assertArgPair(t, podSpec.Containers[0].Args, "--max-model-len", "4096")
}

func TestApplyHardwareOptimizations_GenericX86(t *testing.T) {
	ctx := context.Background()
	podSpec := basePodSpec()
	hwType := deployment.DetectHardware([]corev1.Node{nodeWithArch("amd64", nil, nil)})
	deployment.ApplyHardwareOptimizations(ctx, hwType, podSpec)

	if podSpec.Containers[0].Image != deployment.VLLMGenericImage {
		t.Errorf("image = %q, want %q", podSpec.Containers[0].Image, deployment.VLLMGenericImage)
	}

	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_OMP_THREADS_BIND", "auto")
	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_KVCACHE_SPACE", "10")
	assertArgPair(t, podSpec.Containers[0].Args, "--host", "0.0.0.0")
	assertArgPair(t, podSpec.Containers[0].Args, "--max-model-len", "4096")
}

func TestApplyHardwareOptimizations_NVIDIA(t *testing.T) {
	ctx := context.Background()
	podSpec := basePodSpec()
	hwType := deployment.DetectHardware([]corev1.Node{nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
		"nvidia.com/gpu": resource.MustParse("1"),
	}, nil)})
	deployment.ApplyHardwareOptimizations(ctx, hwType, podSpec)

	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_TARGET_DEVICE", "cuda")
}

func TestApplyHardwareOptimizations_UserImageNotOverridden(t *testing.T) {
	ctx := context.Background()
	podSpec := basePodSpec()
	podSpec.Containers[0].Image = "my-custom-vllm:v1"
	hwType := deployment.DetectHardware([]corev1.Node{nodeWithArch("arm64", nil, nil)})
	deployment.ApplyHardwareOptimizations(ctx, hwType, podSpec)

	if podSpec.Containers[0].Image != "my-custom-vllm:v1" {
		t.Errorf("user image was overridden: got %q", podSpec.Containers[0].Image)
	}
}

func TestApplyHardwareOptimizations_CUDAImageOverridden(t *testing.T) {
	ctx := context.Background()
	podSpec := basePodSpec()
	podSpec.Containers[0].Image = "my-image-cuda:latest"
	hwType := deployment.DetectHardware([]corev1.Node{nodeWithArch("arm64", nil, nil)})
	deployment.ApplyHardwareOptimizations(ctx, hwType, podSpec)

	if podSpec.Containers[0].Image != deployment.VLLMGenericImage {
		t.Errorf("CUDA image should be overridden on ARM64: got %q, want %q",
			podSpec.Containers[0].Image, deployment.VLLMGenericImage)
	}
}

func TestApplyHardwareOptimizations_UserEnvNotOverridden(t *testing.T) {
	ctx := context.Background()
	podSpec := basePodSpec()
	podSpec.Containers[0].Env = []corev1.EnvVar{
		{Name: "VLLM_CPU_OMP_THREADS_BIND", Value: "0-3"},
	}
	hwType := deployment.DetectHardware([]corev1.Node{nodeWithArch("arm64", nil, nil)})
	deployment.ApplyHardwareOptimizations(ctx, hwType, podSpec)

	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_OMP_THREADS_BIND", "0-3")
}
