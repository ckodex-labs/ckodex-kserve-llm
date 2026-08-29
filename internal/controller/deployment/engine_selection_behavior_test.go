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

func TestApplyEngineSelectionUsesRegisteredVLLMAdapter(t *testing.T) {
	for _, service := range []*servingv1alpha2.LLMInferenceService{
		{Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "vllm"}},
		{Spec: servingv1alpha2.LLMInferenceServiceSpec{}},
	} {
		pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
		(&Builder{}).applyEngineSelection(service, pod, HardwareAppleSilicon)
		assert.Equal(t, api.VLLMImage, pod.Containers[0].Image)
		assert.Contains(t, pod.Containers[0].Args, "--model")
		assert.Contains(t, pod.Containers[0].Args, "--host")
	}
}

func TestApplyEngineSelectionUsesDigestPinnedSGLangAdapter(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
		Engine: "sglang", Model: servingv1alpha2.ModelSpec{Name: "served-model"},
	}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: api.VLLMImage}}}
	(&Builder{RuntimeImage: api.VLLMImage}).applyEngineSelection(service, pod, HardwareNVIDIA)

	assert.Equal(t, "lmsysorg/sglang:v0.5.18@sha256:9e148f5ac788e856a06166bd6347a831831eb9fcfab4d1770874823a7c29a1a1", pod.Containers[0].Image)
	assert.Equal(t, []string{"python3", "-m", "sglang.launch_server"}, pod.Containers[0].Args[:3])
	assert.Contains(t, pod.Containers[0].Args, "--model-path")
	assert.Contains(t, pod.Containers[0].Args, "--enable-metrics")
}

func TestApplyEngineSelectionRejectsUnmappedSGLangFields(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
		Engine: "sglang", KVCache: &servingv1alpha2.KVCacheSpec{Dtype: "fp8"},
	}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: "unverified:test", Args: []string{"serve"}}}}
	(&Builder{}).applyEngineSelection(service, pod, HardwareNVIDIA)
	assert.Empty(t, pod.Containers[0].Image)
	assert.Empty(t, pod.Containers[0].Args)
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

func TestApplyEngineSelectionUnverifiedGGUFLeavesContainerUnrunnable(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
		Quantization: &servingv1alpha2.QuantizationSpec{Method: "gguf"},
	}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: "unverified:test", Args: []string{"serve"}}}}
	(&Builder{}).applyEngineSelection(service, pod, HardwareNVIDIA)
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

func TestApplyEngineSelectionVLLMRewritesImageInAirGappedMode(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "vllm"}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
	(&Builder{AirGappedMode: true, LocalRegistry: "registry.local"}).applyEngineSelection(service, pod, HardwareNVIDIA)
	assert.Equal(t, "registry.local/"+api.VLLMImage, pod.Containers[0].Image)
}

func TestApplyEngineSelectionSGLangRewritesPinnedImageInAirGappedMode(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "sglang"}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
	(&Builder{AirGappedMode: true, LocalRegistry: "registry.local"}).applyEngineSelection(service, pod, HardwareNVIDIA)
	assert.Equal(t, "registry.local/lmsysorg/sglang:v0.5.18@sha256:9e148f5ac788e856a06166bd6347a831831eb9fcfab4d1770874823a7c29a1a1", pod.Containers[0].Image)
}
