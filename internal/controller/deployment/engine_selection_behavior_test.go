package deployment

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
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

func TestApplyEngineSelectionAcceptsVLLMRuntimeAcceptanceImage(t *testing.T) {
	image := "vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e"
	service := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			inferenceruntime.RuntimeQualificationAnnotation: "vllm-v0.33.0",
		}},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "vllm", Model: servingv1alpha2.ModelSpec{Name: "qualification-model"}},
	}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: image}}}
	(&Builder{}).applyEngineSelection(service, pod, HardwareNVIDIA)
	assert.Equal(t, image, pod.Containers[0].Image)
	assert.Contains(t, pod.Containers[0].Args, "--model")
	require.Contains(t, pod.Containers[0].Args, "--disable-fastapi-docs")
}

func TestApplyEngineSelectionRewritesVLLMRuntimeAcceptanceImageForAirGap(t *testing.T) {
	const acceptanceImage = "vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e"
	service := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			inferenceruntime.RuntimeQualificationAnnotation: "vllm-v0.33.0",
		}},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "vllm"},
	}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: acceptanceImage}}}
	(&Builder{AirGappedMode: true, LocalRegistry: "registry.local"}).applyEngineSelection(service, pod, HardwareNVIDIA)
	assert.Equal(t, "registry.local/"+acceptanceImage, pod.Containers[0].Image)
}

func TestApplyEngineSelectionAcceptsDigestPinnedVLLMMirror(t *testing.T) {
	const acceptanceImage = "registry.local/vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e"
	service := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			inferenceruntime.RuntimeQualificationAnnotation: "vllm-v0.33.0",
		}},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "vllm"},
	}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: acceptanceImage}}}
	(&Builder{}).applyEngineSelection(service, pod, HardwareNVIDIA)
	assert.Equal(t, acceptanceImage, pod.Containers[0].Image)
}

func TestApplyEngineSelectionRejectsUnknownOrMismatchedVLLMQualification(t *testing.T) {
	for _, test := range []struct {
		name    string
		version string
		image   string
	}{
		{name: "unknown version", version: "vllm-v0.99.0", image: "vllm/vllm-openai:v0.99.0@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{name: "unapproved image", version: "vllm-v0.33.0", image: "vllm/vllm-openai:v0.33.0"},
		{name: "different digest", version: "vllm-v0.33.0", image: "vllm/vllm-openai:v0.33.0@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &servingv1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
					inferenceruntime.RuntimeQualificationAnnotation: test.version,
				}},
				Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "vllm"},
			}
			pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: test.image, Command: []string{"serve"}, Args: []string{"unsafe"}}}}
			(&Builder{}).applyEngineSelection(service, pod, HardwareNVIDIA)
			assert.Empty(t, pod.Containers[0].Image)
			assert.Empty(t, pod.Containers[0].Command)
			assert.Empty(t, pod.Containers[0].Args)
		})
	}
}

func TestApplyEngineSelectionKeepsBaselineUnchangedWithoutQualification(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "vllm"}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
	(&Builder{RuntimeImage: api.VLLMImage}).applyEngineSelection(service, pod, HardwareNVIDIA)
	assert.Equal(t, api.VLLMImage, pod.Containers[0].Image)
}

func TestApplyEngineSelectionUsesDigestPinnedSGLangAdapter(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
		Engine: "sglang", Model: servingv1alpha2.ModelSpec{Name: "served-model"},
	}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: api.VLLMImage}}}
	(&Builder{RuntimeImage: api.VLLMImage}).applyEngineSelection(service, pod, HardwareNVIDIA)

	assert.Equal(t, "lmsysorg/sglang:v0.5.22@sha256:e3a4f06001a29542fc4a8638536d9ffc5eb9c9942b08603c09b19a56eae4a1ae", pod.Containers[0].Image)
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
	assert.Equal(t, "registry.local/library/vllm:tag", (&Builder{LocalRegistry: "registry.local"}).rewriteImage("registry.local/library/vllm:tag"))
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
	assert.Equal(t, "registry.local/lmsysorg/sglang:v0.5.22@sha256:e3a4f06001a29542fc4a8638536d9ffc5eb9c9942b08603c09b19a56eae4a1ae", pod.Containers[0].Image)
}
