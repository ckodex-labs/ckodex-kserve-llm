package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestApplyHardwareOptimizationsNVIDIADoesNotForceCUDACompatibility(t *testing.T) {
	podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm"}}}

	ApplyHardwareOptimizations(context.Background(), HardwareNVIDIA, podSpec)

	env := map[string]string{}
	for _, item := range podSpec.Containers[0].Env {
		env[item.Name] = item.Value
	}
	assert.Equal(t, "cuda", env["VLLM_TARGET_DEVICE"])
	assert.NotContains(t, env, "VLLM_ENABLE_CUDA_COMPATIBILITY")
}

func TestApplyHardwareOptimizationsPreservesExplicitCUDACompatibility(t *testing.T) {
	podSpec := &corev1.PodSpec{Containers: []corev1.Container{{
		Name: "vllm",
		Env:  []corev1.EnvVar{{Name: "VLLM_ENABLE_CUDA_COMPATIBILITY", Value: "0"}},
	}}}

	ApplyHardwareOptimizations(context.Background(), HardwareNVIDIA, podSpec)

	assert.Contains(t, podSpec.Containers[0].Env, corev1.EnvVar{
		Name: "VLLM_ENABLE_CUDA_COMPATIBILITY", Value: "0",
	})
}
