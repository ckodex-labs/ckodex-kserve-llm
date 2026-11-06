package deployment

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnsureVLLMEnvDefaultsModelAndDoesNotOverwriteExistingValues(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"}, Spec: servingv1alpha2.LLMInferenceServiceSpec{Model: servingv1alpha2.ModelSpec{URI: "hf://org/model"}}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Env: []corev1.EnvVar{{Name: "HOME", Value: "/custom"}}}}}
	(&Builder{}).ensureVLLMEnv(service, pod)
	assert.Equal(t, "/custom", envValue(pod.Containers[0].Env, "HOME"))
	assert.Equal(t, "hf:..org.model", envValue(pod.Containers[0].Env, "OIS_MODEL_ID"))
	assert.Equal(t, "128", envValue(pod.Containers[0].Env, "VLLM_MAX_N_SEQUENCES"))
	assert.Equal(t, "0", envValue(pod.Containers[0].Env, "VLLM_SERVER_DEV_MODE"))
	assert.Equal(t, "0", envValue(pod.Containers[0].Env, "VLLM_MEDIA_URL_ALLOW_REDIRECTS"))
	(&Builder{}).ensureVLLMEnv(service, &corev1.PodSpec{})
}

func TestEnsureVLLMEnvSecurityDefaultsAreVLLMOnlyAndRespectExplicitValues(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{
		Spec: servingv1alpha2.LLMInferenceServiceSpec{Engine: "sglang"},
	}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Env: []corev1.EnvVar{{Name: "VLLM_MAX_N_SEQUENCES", Value: "8"}}}}}
	(&Builder{}).ensureVLLMEnv(service, pod)
	assert.Equal(t, "8", envValue(pod.Containers[0].Env, "VLLM_MAX_N_SEQUENCES"))
	assert.Empty(t, envValue(pod.Containers[0].Env, "VLLM_SERVER_DEV_MODE"))
	assert.Empty(t, envValue(pod.Containers[0].Env, "VLLM_MEDIA_URL_ALLOW_REDIRECTS"))
}

func TestGPUDeviceSelectionDefaultsAndPreservesExplicitValue(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Parallelism: &servingv1alpha2.ParallelismSpec{GPUDevices: []string{"GPU-0", "GPU-1"}},
		},
	}

	pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
	(&Builder{}).applyGPUDeviceSelection(service, pod)
	assert.Equal(t, "GPU-0,GPU-1", envValue(pod.Containers[0].Env, "NVIDIA_VISIBLE_DEVICES"))

	pod = &corev1.PodSpec{Containers: []corev1.Container{{Env: []corev1.EnvVar{{Name: "NVIDIA_VISIBLE_DEVICES", Value: "all"}}}}}
	(&Builder{}).applyGPUDeviceSelection(service, pod)
	assert.Equal(t, "all", envValue(pod.Containers[0].Env, "NVIDIA_VISIBLE_DEVICES"))
}

func TestInjectVectorAndBuildAnnotationsReflectConfiguredSurfaces(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"}, Spec: servingv1alpha2.LLMInferenceServiceSpec{Canary: &servingv1alpha2.CanarySpec{Weight: 25}, ToolSurface: &servingv1alpha2.ToolSurface{AllowedAPIs: []string{"search"}}}}
	builder := &Builder{OTEL_Endpoint: "http://otel"}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Image: "vllm:test"}}}
	builder.injectVector(service, pod)
	assert.Len(t, pod.Containers, 2)
	annotations := builder.buildAnnotations(service, pod, "")
	assert.Equal(t, "25", annotations["ckodex.com/canary-weight"])
	assert.Equal(t, "true", annotations["sidecar.istio.io/inject"])
	builder.injectVector(&servingv1alpha2.LLMInferenceService{}, &corev1.PodSpec{Containers: []corev1.Container{{}}})
}
