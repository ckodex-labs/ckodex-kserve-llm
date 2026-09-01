package deployment

import (
	"context"
	"strings"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuilder_BuildPrefillCreatesProducerDeployment(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build(), RuntimeImage: "vllm:v0.28.0"}
	workers := int32(2)
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "chat", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model:    servingv1alpha2.ModelSpec{URI: "pvc://weights"},
			Prefill:  &servingv1alpha2.PrefillSpec{Replicas: &workers, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "prefill"}}}}},
			KVCache:  &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{Connector: "nixl"}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm"}}}},
		},
	}
	dep := builder.BuildPrefill(context.Background(), llmSvc, HardwareNVIDIA)
	require.NotNil(t, dep)
	assert.Equal(t, "chat-prefill", dep.Name)
	assert.Equal(t, workers, *dep.Spec.Replicas)
	assert.Equal(t, "prefill", dep.Spec.Template.Labels["serving.ckodex.com/role"])
	assert.Contains(t, strings.Join(dep.Spec.Template.Spec.Containers[0].Args, " "), "NixlConnector")
	assert.Equal(t, "vllm:v0.28.0", dep.Annotations["serving.ckodex.com/runtime-image"])
	assert.Equal(t, "nixl", dep.Annotations["serving.ckodex.com/kv-connector"])
	assert.Equal(t, "kv_producer", dep.Annotations["serving.ckodex.com/kv-role"])
	assert.Equal(t, "true", dep.Annotations["serving.ckodex.com/pd-disaggregation"])
}

func TestBuilder_Build_RuntimeImageOverridePrecedesCPUFallback(t *testing.T) {
	builder := &Builder{
		Client:       fake.NewClientBuilder().Build(),
		RuntimeImage: "registry.example/vllm-cpu:v0.28.0",
	}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "pvc://weights"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "vllm"}},
			}},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 1, HardwareGenericX86, nil)
	assert.Equal(t, builder.RuntimeImage, dep.Spec.Template.Spec.Containers[0].Image)
}

func TestBuilder_Build_PreservesPersistentKernelCacheConfiguration(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build()}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "gemma", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "pvc://weights"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "vllm",
					Env: []corev1.EnvVar{
						{Name: "VLLM_FLASHINFER_AUTOTUNE_CACHE_DIR", Value: "/var/cache/vllm/flashinfer"},
						{Name: "TORCHINDUCTOR_CACHE_DIR", Value: "/var/cache/vllm/torchinductor"},
					},
					VolumeMounts: []corev1.VolumeMount{{Name: "kernel-cache", MountPath: "/var/cache/vllm"}},
				}},
				Volumes: []corev1.Volume{{
					Name: "kernel-cache",
					VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "gemma-kernel-cache",
					}},
				}},
			}},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)
	podSpec := dep.Spec.Template.Spec
	require.NotEmpty(t, podSpec.Containers)
	c := podSpec.Containers[0]
	assert.Contains(t, c.Env, corev1.EnvVar{
		Name: "VLLM_FLASHINFER_AUTOTUNE_CACHE_DIR", Value: "/var/cache/vllm/flashinfer",
	})
	assert.Contains(t, c.Env, corev1.EnvVar{
		Name: "TORCHINDUCTOR_CACHE_DIR", Value: "/var/cache/vllm/torchinductor",
	})
	assert.Contains(t, c.VolumeMounts, corev1.VolumeMount{Name: "kernel-cache", MountPath: "/var/cache/vllm"})
	assert.Contains(t, podSpec.Volumes, corev1.Volume{
		Name: "kernel-cache",
		VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: "gemma-kernel-cache",
		}},
	})
}

func TestBuilder_Build_ReplicaCountGreaterThanOneUsesRollingUpdate(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	builder := &Builder{Client: client}

	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: "default",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: "mistralai/Mistral-7B",
				URI:  "hf://mistralai/Mistral-7B",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "vllm",
						},
					},
				},
			},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 2, HardwareNVIDIA, nil)
	require.NotNil(t, dep)

	assert.Equal(t, appsv1.RollingUpdateDeploymentStrategyType, dep.Spec.Strategy.Type)
	require.NotNil(t, dep.Spec.Strategy.RollingUpdate)
	require.NotNil(t, dep.Spec.Strategy.RollingUpdate.MaxUnavailable)
	require.NotNil(t, dep.Spec.Strategy.RollingUpdate.MaxSurge)
	assert.Equal(t, intstr.Int, dep.Spec.Strategy.RollingUpdate.MaxUnavailable.Type)
	assert.Equal(t, int32(0), dep.Spec.Strategy.RollingUpdate.MaxUnavailable.IntVal)
	assert.Equal(t, intstr.Int, dep.Spec.Strategy.RollingUpdate.MaxSurge.Type)
	assert.Equal(t, int32(1), dep.Spec.Strategy.RollingUpdate.MaxSurge.IntVal)
}

type typedNilSPIREInjector struct{}

func (*typedNilSPIREInjector) InjectSidecar(podSpec *corev1.PodSpec, llmSvc *servingv1alpha2.LLMInferenceService) {
	panic("typed nil injector should never be called")
}

func TestBuilder_Build_TypedNilSPIREInjectorDoesNotPanic(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	builder := &Builder{
		Client: client,
	}

	var injector *typedNilSPIREInjector
	builder.SPIRE = injector

	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model",
			Namespace: "default",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: "mistralai/Mistral-7B",
				URI:  "hf://mistralai/Mistral-7B",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "vllm",
						},
					},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
		require.NotNil(t, dep)
	})
}
