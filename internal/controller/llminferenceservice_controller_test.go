/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/security"
)

// --- detectHardware tests ---

func TestDetectHardware(t *testing.T) {
	tests := []struct {
		name     string
		nodes    []corev1.Node
		expected HardwareType
	}{
		{
			name:     "empty node list returns Unknown",
			nodes:    nil,
			expected: HardwareUnknown,
		},
		{
			name: "ARM64 node without GPU returns AppleSilicon",
			nodes: []corev1.Node{
				nodeWithArch("arm64", nil, nil),
			},
			expected: HardwareAppleSilicon,
		},
		{
			name: "ARM64 node with apple.com/gpu capacity returns AppleSiliconMPS",
			nodes: []corev1.Node{
				nodeWithArch("arm64", map[corev1.ResourceName]resource.Quantity{
					"apple.com/gpu": resource.MustParse("1"),
				}, nil),
			},
			expected: HardwareAppleSiliconMPS,
		},
		{
			name: "ARM64 node with apple.com/gpu.present label returns AppleSiliconMPS",
			nodes: []corev1.Node{
				nodeWithArch("arm64", nil, map[string]string{
					"apple.com/gpu.present": "true",
				}),
			},
			expected: HardwareAppleSiliconMPS,
		},
		{
			name: "AMD64 node without GPU returns GenericX86",
			nodes: []corev1.Node{
				nodeWithArch("amd64", nil, nil),
			},
			expected: HardwareGenericX86,
		},
		{
			name: "node with nvidia.com/gpu capacity returns NVIDIA",
			nodes: []corev1.Node{
				nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
					"nvidia.com/gpu": resource.MustParse("1"),
				}, nil),
			},
			expected: HardwareNVIDIA,
		},
		{
			name: "node with nvidia.com/gpu.present label returns NVIDIA",
			nodes: []corev1.Node{
				nodeWithArch("amd64", nil, map[string]string{
					"nvidia.com/gpu.present": "true",
				}),
			},
			expected: HardwareNVIDIA,
		},
		{
			name: "node with amd.com/gpu capacity returns AMD",
			nodes: []corev1.Node{
				nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
					"amd.com/gpu": resource.MustParse("1"),
				}, nil),
			},
			expected: HardwareAMD,
		},
		{
			name: "mixed cluster ARM64 + NVIDIA picks highest priority (NVIDIA)",
			nodes: []corev1.Node{
				nodeWithArch("arm64", nil, nil),
				nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
					"nvidia.com/gpu": resource.MustParse("2"),
				}, nil),
			},
			expected: HardwareNVIDIA,
		},
		{
			name: "mixed cluster AMD GPU + x86 CPU picks AMD",
			nodes: []corev1.Node{
				nodeWithArch("amd64", nil, nil),
				nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
					"amd.com/gpu": resource.MustParse("1"),
				}, nil),
			},
			expected: HardwareAMD,
		},
	}

	r := &LLMInferenceServiceReconciler{
		Recorder: record.NewFakeRecorder(10),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.detectHardware(tt.nodes)
			if got != tt.expected {
				t.Errorf("detectHardware() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// --- applyHardwareOptimizations tests ---

func TestApplyHardwareOptimizations_AppleSilicon(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	podSpec := basePodSpec()

	llmSvc := baseLLMInferenceService()
	r.applyHardwareOptimizations(ctx, llmSvc, podSpec)

	// Image should be set to CPU ARM64 image
	if podSpec.Containers[0].Image != VLLMCPUArm64Image {
		t.Errorf("image = %q, want %q", podSpec.Containers[0].Image, VLLMCPUArm64Image)
	}

	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_OMP_THREADS_BIND", "nobind")
	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_KVCACHE_SPACE", "4")
	assertArgPair(t, podSpec.Containers[0].Args, "--host", "0.0.0.0")
	assertArgPair(t, podSpec.Containers[0].Args, "--port", "8000")
	assertArgPair(t, podSpec.Containers[0].Args, "--max-model-len", "4096")
}

func TestApplyHardwareOptimizations_GenericX86(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("amd64", nil, nil))
	podSpec := basePodSpec()

	llmSvc := baseLLMInferenceService()
	r.applyHardwareOptimizations(ctx, llmSvc, podSpec)

	if podSpec.Containers[0].Image != VLLMGenericImage {
		t.Errorf("image = %q, want %q", podSpec.Containers[0].Image, VLLMGenericImage)
	}

	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_OMP_THREADS_BIND", "auto")
	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_KVCACHE_SPACE", "10")
	assertArgPair(t, podSpec.Containers[0].Args, "--device", "cpu")
	assertArgPair(t, podSpec.Containers[0].Args, "--max-model-len", "4096")
}

func TestApplyHardwareOptimizations_NVIDIA(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("amd64", map[corev1.ResourceName]resource.Quantity{
		"nvidia.com/gpu": resource.MustParse("1"),
	}, nil))
	podSpec := basePodSpec()

	llmSvc := baseLLMInferenceService()
	r.applyHardwareOptimizations(ctx, llmSvc, podSpec)

	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_TARGET_DEVICE", "cuda")
}

func TestApplyHardwareOptimizations_UserImageNotOverridden(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	podSpec := basePodSpec()
	podSpec.Containers[0].Image = "my-custom-vllm:v1"

	llmSvc := baseLLMInferenceService()
	r.applyHardwareOptimizations(ctx, llmSvc, podSpec)

	if podSpec.Containers[0].Image != "my-custom-vllm:v1" {
		t.Errorf("user image was overridden: got %q", podSpec.Containers[0].Image)
	}
}

func TestApplyHardwareOptimizations_CUDAImageOverridden(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	podSpec := basePodSpec()
	podSpec.Containers[0].Image = "my-image-cuda:latest"

	llmSvc := baseLLMInferenceService()
	r.applyHardwareOptimizations(ctx, llmSvc, podSpec)

	if podSpec.Containers[0].Image != VLLMCPUArm64Image {
		t.Errorf("CUDA image should be overridden on ARM64: got %q, want %q",
			podSpec.Containers[0].Image, VLLMCPUArm64Image)
	}
}

func TestApplyHardwareOptimizations_UserEnvNotOverridden(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	podSpec := basePodSpec()
	podSpec.Containers[0].Env = []corev1.EnvVar{
		{Name: "VLLM_CPU_OMP_THREADS_BIND", Value: "0-3"},
	}

	llmSvc := baseLLMInferenceService()
	r.applyHardwareOptimizations(ctx, llmSvc, podSpec)

	assertEnvVar(t, podSpec.Containers[0].Env, "VLLM_CPU_OMP_THREADS_BIND", "0-3")
}

// --- helpers ---

func nodeWithArch(arch string, capacity map[corev1.ResourceName]resource.Quantity, labels map[string]string) corev1.Node {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-" + arch,
			Labels: labels,
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				Architecture: arch,
			},
		},
	}
	if capacity != nil {
		node.Status.Capacity = capacity
	}
	return node
}

func reconcilerWithNodes(t *testing.T, nodes ...corev1.Node) (*LLMInferenceServiceReconciler, context.Context) {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = servingv1alpha2.AddToScheme(scheme)

	objs := make([]runtime.Object, len(nodes))
	for i := range nodes {
		objs[i] = &nodes[i]
	}

	cb := fake.NewClientBuilder().WithScheme(scheme)
	for _, n := range nodes {
		n := n
		cb = cb.WithObjects(&n)
	}

	r := &LLMInferenceServiceReconciler{
		Client:   cb.Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}
	return r, context.Background()
}

func basePodSpec() *corev1.PodSpec {
	return &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "vllm",
				Image: "", // empty = let the controller set it
			},
		},
	}
}

func baseLLMInferenceService() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://test/model",
				Name: "test-model",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "vllm"},
					},
				},
			},
			Router: servingv1alpha2.RouterSpec{
				Gateway: servingv1alpha2.GatewaySpec{
					Managed: &servingv1alpha2.ManagedGatewaySpec{
						GatewayClassName: "envoy",
					},
				},
			},
		},
	}
}

// --- hf-mount CSI volume tests ---

func TestBuildStorageInitializer_HFMountReturnsNil(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	llmSvc := baseLLMInferenceService()
	llmSvc.Spec.Model.URI = "hf-mount://Qwen/Qwen2.5-0.5B-Instruct"

	result := r.buildStorageInitializer(ctx, llmSvc, nil)
	if result != nil {
		t.Error("buildStorageInitializer should return nil for hf-mount:// URIs")
	}
}

func TestBuildDeployment_HFMountCSIVolume(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	llmSvc := baseLLMInferenceService()
	llmSvc.Spec.Model.URI = "hf-mount://Qwen/Qwen2.5-0.5B-Instruct"

	deploy := r.buildDeployment(ctx, llmSvc, 1)
	podSpec := deploy.Spec.Template.Spec

	// Should have no init containers (hf-mount skips storage initializer)
	for _, ic := range podSpec.InitContainers {
		if ic.Name == "storage-initializer" {
			t.Error("hf-mount should not have a storage-initializer init container")
		}
	}

	// Should have a CSI volume with the hf-mount driver
	var csiVol *corev1.CSIVolumeSource
	for _, v := range podSpec.Volumes {
		if v.Name == ModelVolumeName && v.CSI != nil {
			csiVol = v.CSI
			break
		}
	}
	if csiVol == nil {
		t.Fatal("expected CSI volume for hf-mount:// URI, got none")
	}
	if csiVol.Driver != HFMountCSIDriver {
		t.Errorf("CSI driver = %q, want %q", csiVol.Driver, HFMountCSIDriver)
	}
	if csiVol.VolumeAttributes["repo"] != "Qwen/Qwen2.5-0.5B-Instruct" {
		t.Errorf("repo attr = %q, want %q", csiVol.VolumeAttributes["repo"], "Qwen/Qwen2.5-0.5B-Instruct")
	}
	if csiVol.ReadOnly == nil || !*csiVol.ReadOnly {
		t.Error("hf-mount volume should be read-only")
	}
}

func TestBuildDeployment_HFMountWithRevision(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	llmSvc := baseLLMInferenceService()
	llmSvc.Spec.Model.URI = "hf-mount://Qwen/Qwen2.5-0.5B-Instruct@v1.0"

	deploy := r.buildDeployment(ctx, llmSvc, 1)

	var csiVol *corev1.CSIVolumeSource
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.Name == ModelVolumeName && v.CSI != nil {
			csiVol = v.CSI
			break
		}
	}
	if csiVol == nil {
		t.Fatal("expected CSI volume")
	}
	if csiVol.VolumeAttributes["repo"] != "Qwen/Qwen2.5-0.5B-Instruct" {
		t.Errorf("repo = %q, want without revision", csiVol.VolumeAttributes["repo"])
	}
	if csiVol.VolumeAttributes["revision"] != "v1.0" {
		t.Errorf("revision = %q, want %q", csiVol.VolumeAttributes["revision"], "v1.0")
	}
}

func TestBuildDeployment_HFMountWithSecret(t *testing.T) {
	r, ctx := reconcilerWithNodes(t, nodeWithArch("arm64", nil, nil))
	llmSvc := baseLLMInferenceService()
	llmSvc.Spec.Model.URI = "hf-mount://myorg/private-model"
	llmSvc.Spec.Model.Storage = &servingv1alpha2.StorageSpec{
		SecretRef: &corev1.LocalObjectReference{Name: "hf-token-secret"},
	}

	deploy := r.buildDeployment(ctx, llmSvc, 1)

	var csiVol *corev1.CSIVolumeSource
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.Name == ModelVolumeName && v.CSI != nil {
			csiVol = v.CSI
			break
		}
	}
	if csiVol == nil {
		t.Fatal("expected CSI volume")
	}
	if csiVol.VolumeAttributes["tokenSecret"] != "hf-token-secret" {
		t.Errorf("tokenSecret = %q, want %q", csiVol.VolumeAttributes["tokenSecret"], "hf-token-secret")
	}
}

func assertEnvVar(t *testing.T, envs []corev1.EnvVar, name, expected string) {
	t.Helper()
	for _, ev := range envs {
		if ev.Name == name {
			if ev.Value != expected {
				t.Errorf("env %s = %q, want %q", name, ev.Value, expected)
			}
			return
		}
	}
	t.Errorf("env %s not found", name)
}

func assertArgPair(t *testing.T, args []string, flag, value string) {
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

func TestCleanupResources(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = servingv1alpha2.AddToScheme(scheme)

	ctx := context.Background()
	llmSvc := baseLLMInferenceService()
	llmSvc.Namespace = "my-ns"
	llmSvc.Name = "my-svc"

	// Mock SPIRE registration ConfigMap
	cmName := security.SPIRERegistrationCMPrefix + llmSvc.Namespace + "-" + llmSvc.Name
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: security.SPIRERegistrationNamespace,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		SPIRERegistration: &security.SPIRERegistrationReconciler{
			Client: cl,
			Scheme: scheme,
		},
	}

	// 1. Verify CM exists before cleanup
	var foundCM corev1.ConfigMap
	err := cl.Get(ctx, k8stypes.NamespacedName{Name: cmName, Namespace: security.SPIRERegistrationNamespace}, &foundCM)
	require.NoError(t, err)

	// 2. Run cleanup
	err = r.cleanupResources(ctx, llmSvc)
	require.NoError(t, err)

	// 3. Verify CM is deleted
	err = cl.Get(ctx, k8stypes.NamespacedName{Name: cmName, Namespace: security.SPIRERegistrationNamespace}, &foundCM)
	assert.True(t, errors.IsNotFound(err), "ConfigMap should have been deleted by cleanupResources")
}
