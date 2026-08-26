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
	(&Builder{}).ensureVLLMEnv(service, &corev1.PodSpec{})
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
