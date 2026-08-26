package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDetectHardwareChoosesHighestPriorityCapability(t *testing.T) {
	nodes := []corev1.Node{
		{Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{Architecture: "unknown"}}},
		{Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{Architecture: "amd64"}}},
		{Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{Architecture: "arm64"}}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"apple.com/gpu.present": "true"}}, Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{Architecture: "arm64"}}},
		{Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"amd.com/gpu": resource.MustParse("1")}}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"nvidia.com/gpu.present": "true"}}},
	}

	assert.Equal(t, HardwareNVIDIA, DetectHardware(nodes))
	assert.Equal(t, HardwareUnknown, DetectHardware(nil))
}

func TestDetectHardwareRecognizesAppleAndGPUCapacity(t *testing.T) {
	assert.Equal(t, HardwareAppleSiliconMPS, DetectHardware([]corev1.Node{{
		Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{Architecture: "arm64"}, Capacity: corev1.ResourceList{"apple.com/gpu": resource.MustParse("1")}},
	}}))
	assert.Equal(t, HardwareAMD, DetectHardware([]corev1.Node{{
		Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"amd.com/gpu": resource.MustParse("1")}},
	}}))
}

func TestApplyHardwareOptimizationsCoversCPUAppleAMDAndNoOpCases(t *testing.T) {
	for _, tc := range []struct {
		name string
		hw   HardwareType
		want string
	}{
		{"apple mps", HardwareAppleSiliconMPS, "mps"},
		{"apple cpu", HardwareAppleSilicon, "cpu"},
		{"generic cpu", HardwareGenericX86, ""},
		{"amd", HardwareAMD, "rocm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: "vllm/vllm-openai:latest"}}}
			ApplyHardwareOptimizations(context.Background(), tc.hw, pod)
			if tc.want != "" {
				assert.Equal(t, tc.want, testEnvValue(pod.Containers[0].Env, "VLLM_TARGET_DEVICE"))
			}
			if tc.hw != HardwareAMD {
				assert.Contains(t, pod.Containers[0].Args, "--host")
			}
		})
	}
	for _, hw := range []HardwareType{HardwareUnknown, HardwareNVIDIA} {
		pod := &corev1.PodSpec{}
		ApplyHardwareOptimizations(context.Background(), hw, pod)
		assert.Empty(t, pod.Containers)
	}
}

func TestApplyHardwareOptimizationsPreservesExplicitEnvironmentAndArguments(t *testing.T) {
	pod := &corev1.PodSpec{Containers: []corev1.Container{{
		Image: "custom/image", Env: []corev1.EnvVar{{Name: "GPU_MEMORY_UTILIZATION", Value: "0.7"}}, Args: []string{"--host", "custom"},
	}}}
	ApplyHardwareOptimizations(context.Background(), HardwareNVIDIA, pod)
	assert.Equal(t, "0.7", testEnvValue(pod.Containers[0].Env, "GPU_MEMORY_UTILIZATION"))
	assert.Equal(t, []string{"--host", "custom"}, pod.Containers[0].Args)
}

func testEnvValue(env []corev1.EnvVar, name string) string {
	for _, item := range env {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}
