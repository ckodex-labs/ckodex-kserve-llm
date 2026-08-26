package deployment

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyEngineSelectionUsesQuantCppForExplicitEngineAndGGUF(t *testing.T) {
	for _, service := range []*servingv1alpha2.LLMInferenceService{
		{Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "quant-cpp"}},
		{Spec: servingv1alpha2.LLMInferenceServiceSpec{Quantization: &servingv1alpha2.QuantizationSpec{Method: "gguf"}}},
	} {
		pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
		(&Builder{}).applyEngineSelection(service, pod, HardwareAppleSilicon)
		assert.Equal(t, api.QuantCppImage, pod.Containers[0].Image)
		assert.Contains(t, pod.Containers[0].Args, "-m")
		assert.Contains(t, pod.Containers[0].Args, "--n-gpu-layers")
	}
}

func TestApplyEngineSelectionUnsupportedEngineLeavesContainerUnrunnable(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
		Engine: "unsupported-engine", Model: servingv1alpha2.ModelSpec{URI: "hf://org/model"},
	}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
	(&Builder{RuntimeImage: "vllm:test"}).applyEngineSelection(service, pod, HardwareNVIDIA)
	assert.Empty(t, pod.Containers[0].Image)
	assert.Empty(t, pod.Containers[0].Args)
}

func TestApplyLoraAdaptersAddsMountsAndHandlesEmptyPod(t *testing.T) {
	builder := &Builder{}
	service := servingv1alpha2.LLMLoraAdapter{ObjectMeta: metav1.ObjectMeta{Name: "adapter-a"}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
	builder.applyLoraAdapters([]servingv1alpha2.LLMLoraAdapter{service}, pod)
	args := pod.Containers[0].Args
	require.Contains(t, args, "--enable-lora")
	require.Len(t, pod.Volumes, 1)
	assert.Equal(t, "lora-adapter-a", pod.Volumes[0].Name)
	assert.Equal(t, api.ModelMountPath+"/lora-adapter-a", pod.Containers[0].VolumeMounts[0].MountPath)
	builder.applyLoraAdapters(nil, &corev1.PodSpec{})
}

func TestRewriteImageAddsRegistryWithoutChangingPath(t *testing.T) {
	assert.Equal(t, "registry.local/library/vllm:tag", (&Builder{LocalRegistry: "registry.local"}).rewriteImage("library/vllm:tag"))
	assert.Equal(t, "image:tag", (&Builder{}).rewriteImage("image:tag"))
}

func TestApplyEngineSelectionQuantCppRewritesImageInAirGappedMode(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "quant-cpp"}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
	(&Builder{AirGappedMode: true, LocalRegistry: "registry.local"}).applyEngineSelection(service, pod, HardwareNVIDIA)
	assert.Equal(t, "registry.local/"+api.QuantCppImage, pod.Containers[0].Image)
}
