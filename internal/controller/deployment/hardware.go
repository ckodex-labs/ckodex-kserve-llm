package deployment

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type HardwareType string

const (
	HardwareAppleSilicon    HardwareType = "AppleSilicon"
	HardwareAppleSiliconMPS HardwareType = "AppleSiliconMPS"
	HardwareNVIDIA          HardwareType = "NVIDIA"
	HardwareAMD             HardwareType = "AMD"
	HardwareGenericX86      HardwareType = "GenericX86"
	HardwareUnknown         HardwareType = "Unknown"
)

// vLLM Images (Keep here for hardware-specific selection)
const (
	VLLMGenericImage  = "vllm/vllm-openai-cpu:v0.23.0"
	VLLMCPUArm64Image = "vllm/vllm-openai:v0.23.0"
	VLLMMPSImage      = "vllm/vllm-openai:v0.23.0"
	VLLMROCmImage     = "vllm/vllm-openai:v0.23.0-rocm"
	VLLMGemma4Image   = "vllm/vllm-openai:gemma4"
)

// DetectHardware identifies the best available hardware across all nodes.
func DetectHardware(nodes []corev1.Node) HardwareType {
	if len(nodes) == 0 {
		return HardwareUnknown
	}

	priority := map[HardwareType]int{
		HardwareUnknown:         0,
		HardwareGenericX86:      1,
		HardwareAppleSilicon:    2,
		HardwareAppleSiliconMPS: 3,
		HardwareAMD:             4,
		HardwareNVIDIA:          5,
	}

	best := HardwareUnknown
	for _, node := range nodes {
		var detected HardwareType

		if qty, ok := node.Status.Capacity["nvidia.com/gpu"]; ok && !qty.IsZero() {
			detected = HardwareNVIDIA
		} else if node.Labels["nvidia.com/gpu.present"] == "true" {
			detected = HardwareNVIDIA
		} else if qty, ok := node.Status.Capacity["amd.com/gpu"]; ok && !qty.IsZero() {
			detected = HardwareAMD
		} else if node.Status.NodeInfo.Architecture == "arm64" {
			if qty, ok := node.Status.Capacity["apple.com/gpu"]; ok && !qty.IsZero() {
				detected = HardwareAppleSiliconMPS
			} else if node.Labels["apple.com/gpu.present"] == "true" {
				detected = HardwareAppleSiliconMPS
			} else {
				detected = HardwareAppleSilicon
			}
		} else if node.Status.NodeInfo.Architecture == "amd64" {
			detected = HardwareGenericX86
		}

		if priority[detected] > priority[best] {
			best = detected
		}
	}

	return best
}

// GetClusterGPUCapacity returns the total count of NVIDIA GPUs in the cluster.
func GetClusterGPUCapacity(nodes []corev1.Node) int {
	total := 0
	for _, node := range nodes {
		if qty, ok := node.Status.Capacity["nvidia.com/gpu"]; ok {
			val, _ := qty.AsInt64()
			total += int(val)
		}
	}
	return total
}

// ApplyHardwareOptimizations applies best-practice defaults for the specific environment.
func ApplyHardwareOptimizations(ctx context.Context, hwType HardwareType, podSpec *corev1.PodSpec) {
	if hwType == HardwareUnknown || len(podSpec.Containers) == 0 {
		return
	}

	container := &podSpec.Containers[0]
	envVars := make(map[string]string)
	args := []string{}

	switch hwType {
	case HardwareAppleSiliconMPS:
		if container.Image == "" || container.Image == "vllm/vllm-openai:latest" || strings.Contains(container.Image, "cuda") {
			container.Image = VLLMMPSImage
		}
		envVars["VLLM_TARGET_DEVICE"] = "mps"
		envVars["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0.0"
		args = append(args, "--device", "mps", "--host", "0.0.0.0", "--port", "8000")

	case HardwareAppleSilicon:
		if container.Image == "" || container.Image == "vllm/vllm-openai:latest" || strings.Contains(container.Image, "cuda") {
			container.Image = VLLMGenericImage
		}
		envVars["VLLM_TARGET_DEVICE"] = "cpu"
		envVars["VLLM_CPU_OMP_THREADS_BIND"] = "nobind"
		envVars["VLLM_CPU_KVCACHE_SPACE"] = "4"
		args = append(args, "--host", "0.0.0.0", "--port", "8000", "--max-model-len", "4096")

	case HardwareGenericX86:
		if container.Image == "" || container.Image == "vllm/vllm-openai:latest" || strings.Contains(container.Image, "cuda") {
			container.Image = VLLMGenericImage
		}
		envVars["VLLM_CPU_KVCACHE_SPACE"] = "10"
		envVars["VLLM_CPU_OMP_THREADS_BIND"] = "auto"
		envVars["NVIDIA_VISIBLE_DEVICES"] = ""
		envVars["TORCHINDUCTOR_FREEZING"] = "1"
		args = append(args, "--host", "0.0.0.0", "--port", "8000", "--max-model-len", "4096")

	case HardwareNVIDIA:
		envVars["VLLM_TARGET_DEVICE"] = "cuda"
		envVars["GPU_MEMORY_UTILIZATION"] = "0.9"
		envVars["VLLM_ENABLE_CUDA_COMPATIBILITY"] = "true"

	case HardwareAMD:
		if container.Image == "" || !strings.Contains(container.Image, "-rocm") {
			container.Image = VLLMROCmImage
		}
		envVars["VLLM_TARGET_DEVICE"] = "rocm"
		envVars["VLLM_ATTENTION_BACKEND"] = "ROCM_FLASH_ATTN"
	}

	// Apply Env
	for k, v := range envVars {
		found := false
		for i, ev := range container.Env {
			if ev.Name == k {
				if ev.Value == "" || ev.Value == "auto" {
					container.Env[i].Value = v
				}
				found = true
				break
			}
		}
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{Name: k, Value: v})
		}
	}

	// Apply Args
	for i := 0; i < len(args); i += 2 {
		flag := args[i]
		value := args[i+1]
		found := false
		for _, a := range container.Args {
			if a == flag {
				found = true
				break
			}
		}
		if !found {
			container.Args = append(container.Args, flag, value)
		}
	}
}
