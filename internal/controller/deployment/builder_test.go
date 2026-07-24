package deployment

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

func TestBuilder_Build(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	builder := &Builder{
		Client:             client,
		LocalCosignKeyPath: "/etc/cosign/cosign.pub",
	}

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

	// Test Build with default hardware type
	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)

	assert.Equal(t, "test-model", dep.Name)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)
	assert.Equal(t, appsv1.RecreateDeploymentStrategyType, dep.Spec.Strategy.Type)

	// Verify pod spec modifications
	podSpec := dep.Spec.Template.Spec
	require.Len(t, podSpec.Containers, 1)

	c := podSpec.Containers[0]
	// Resources should be ensured
	assert.NotNil(t, c.Resources.Requests)
	assert.NotNil(t, c.Resources.Limits)

	// Prestop hook should be injected
	require.NotNil(t, c.Lifecycle)
	require.NotNil(t, c.Lifecycle.PreStop)

	// Check model volume injected
	var foundVol bool
	for _, v := range podSpec.Volumes {
		if v.Name == api.ModelVolumeName {
			foundVol = true
			break
		}
	}
	assert.True(t, foundVol, "model volume should be injected")

	// Check environment variables injected
	envs := make(map[string]string)
	for _, e := range c.Env {
		envs[e.Name] = e.Value
	}
	assert.Equal(t, "/tmp", envs["HOME"])
	assert.Equal(t, "nonroot", envs["USER"])
	assert.Equal(t, "nonroot", envs["LOGNAME"])

	// Check SecurityContext
	require.NotNil(t, c.SecurityContext)
	assert.True(t, *c.SecurityContext.ReadOnlyRootFilesystem)
	assert.True(t, *c.SecurityContext.RunAsNonRoot)
	assert.Equal(t, int64(65532), *c.SecurityContext.RunAsUser)
	assert.Contains(t, c.SecurityContext.Capabilities.Drop, corev1.Capability("ALL"))

	// Check /tmp scratch mount
	var foundTmp bool
	for _, m := range c.VolumeMounts {
		if m.MountPath == "/tmp" {
			foundTmp = true
			break
		}
	}
	assert.True(t, foundTmp, "/tmp scratch space should be mounted")

	require.NotNil(t, c.ReadinessProbe)
	require.NotNil(t, c.ReadinessProbe.HTTPGet)
	assert.Equal(t, "/health", c.ReadinessProbe.HTTPGet.Path)
	require.NotNil(t, c.LivenessProbe)
	require.NotNil(t, c.LivenessProbe.HTTPGet)
	assert.Equal(t, "/health", c.LivenessProbe.HTTPGet.Path)
	require.NotNil(t, c.StartupProbe)
	require.NotNil(t, c.StartupProbe.HTTPGet)
	assert.Equal(t, "/health", c.StartupProbe.HTTPGet.Path)
	assert.Equal(t, int32(30), c.StartupProbe.InitialDelaySeconds)
	assert.Equal(t, int32(15), c.StartupProbe.PeriodSeconds)
	assert.Equal(t, int32(60), c.StartupProbe.FailureThreshold)
}

func TestBuilder_Build_PreservesExplicitStartupProbe(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	builder := &Builder{Client: client}
	custom := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{Command: []string{"test", "-f", "/tmp/model-ready"}},
		},
		PeriodSeconds:    5,
		FailureThreshold: 120,
	}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-startup", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{Name: "custom", URI: "pvc://weights"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:         "vllm",
				StartupProbe: custom.DeepCopy(),
			}}}},
		},
	}

	deployment := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, deployment)
	require.NotEmpty(t, deployment.Spec.Template.Spec.Containers)
	assert.Equal(t, custom, deployment.Spec.Template.Spec.Containers[0].StartupProbe)
}

func TestBuilder_Build_DefaultVLLMArgsUseCpuFallback(t *testing.T) {
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
						{Name: "vllm"},
					},
				},
			},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 1, HardwareUnknown, nil)
	require.NotNil(t, dep)
	require.NotEmpty(t, dep.Spec.Template.Spec.Containers)
	assert.NotContains(t, dep.Spec.Template.Spec.Containers[0].Args, "--device")
	assert.Contains(t, dep.Spec.Template.Spec.Containers[0].Args, "--host")
	assert.Contains(t, dep.Spec.Template.Spec.Containers[0].Args, "0.0.0.0")
}

func TestBuilder_Build_RuntimeImageAndMountedModelOverride(t *testing.T) {
	builder := &Builder{
		Client:       fake.NewClientBuilder().Build(),
		RuntimeImage: "registry.example/vllm:v0.25.1",
	}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "gemma", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "pvc://weights/gemma"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "vllm", Args: []string{"--max-model-len", "4096"},
			}}}},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	c := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "registry.example/vllm:v0.25.1", c.Image)
	assert.Equal(t, []string{"--model", api.ModelMountPath}, c.Args[:2])
	assert.Contains(t, c.Args, "--max-model-len")
}

func TestBuilder_BuildWithRole_ConfiguresLMCacheTransfer(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build(), RuntimeImage: "vllm:v0.25.1"}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "chat", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "pvc://weights"},
			KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
				Connector: "lmcache", ExtraConfig: map[string]string{"chunk_size": "256"},
				Env: []corev1.EnvVar{{Name: "LMCACHE_CONFIG_FILE", Value: "/etc/lmcache/config.yaml"}},
			}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm"}}}},
		},
	}
	dep := builder.BuildWithRole(context.Background(), llmSvc, 1, HardwareNVIDIA, nil, "kv_consumer")
	args := dep.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--kv-transfer-config")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "LMCacheConnectorV1")
	assert.Contains(t, joined, "kv_consumer")
	assert.Equal(t, "/etc/lmcache/config.yaml", cEnv(dep, "LMCACHE_CONFIG_FILE"))
	assert.Equal(t, "True", cEnv(dep, "LMCACHE_USE_EXPERIMENTAL"))
	assert.Contains(t, joined, `"chunk_size":256`)
}

func TestBuilder_BuildWithRole_LMCachePreservesExplicitExperimentalFlag(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build(), RuntimeImage: "vllm:v0.25.1"}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "chat", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "pvc://weights"},
			KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
				Connector: "lmcache",
				Env:       []corev1.EnvVar{{Name: "LMCACHE_USE_EXPERIMENTAL", Value: "False"}},
			}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm"}}}},
		},
	}
	dep := builder.BuildWithRole(context.Background(), llmSvc, 1, HardwareNVIDIA, nil, "kv_both")
	assert.Equal(t, "False", cEnv(dep, "LMCACHE_USE_EXPERIMENTAL"))
}

func cEnv(dep *appsv1.Deployment, name string) string {
	for _, env := range dep.Spec.Template.Spec.Containers[0].Env {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}

func TestBuilder_BuildPrefillCreatesProducerDeployment(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build(), RuntimeImage: "vllm:v0.25.1"}
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
	assert.Equal(t, "vllm:v0.25.1", dep.Annotations["serving.ckodex.com/runtime-image"])
	assert.Equal(t, "nixl", dep.Annotations["serving.ckodex.com/kv-connector"])
	assert.Equal(t, "kv_producer", dep.Annotations["serving.ckodex.com/kv-role"])
	assert.Equal(t, "true", dep.Annotations["serving.ckodex.com/pd-disaggregation"])
}

func TestBuilder_Build_RuntimeImageOverridePrecedesCPUFallback(t *testing.T) {
	builder := &Builder{
		Client:       fake.NewClientBuilder().Build(),
		RuntimeImage: "registry.example/vllm-cpu:v0.25.1",
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

func TestBuilder_Build_PVCVolumeMount(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	builder := &Builder{Client: client}

	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI: "pvc://model-pvc-name",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "llm-server"}},
				},
			},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)

	podSpec := dep.Spec.Template.Spec

	// Verify PVC volume
	var foundPVC bool
	for _, v := range podSpec.Volumes {
		if v.Name == api.ModelVolumeName {
			require.NotNil(t, v.PersistentVolumeClaim)
			assert.Equal(t, "model-pvc-name", v.PersistentVolumeClaim.ClaimName)
			foundPVC = true
			break
		}
	}
	assert.True(t, foundPVC, "Direct PVC volume should be injected for pvc:// URIs")

	// Verify no storage-initializer init container
	assert.Empty(t, podSpec.InitContainers, "Init containers should be empty for native PVC mounting")
}

func TestBuilder_Build_PVCVolumeSubPath(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build()}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "gemma", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model:    servingv1alpha2.ModelSpec{URI: "pvc://gemma4-weights/gemma-4-26B-A4B-it-NVFP4"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm"}}}},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	var claimName, subPath string
	for _, volume := range dep.Spec.Template.Spec.Volumes {
		if volume.Name == api.ModelVolumeName && volume.PersistentVolumeClaim != nil {
			claimName = volume.PersistentVolumeClaim.ClaimName
		}
	}
	for _, mount := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
		if mount.Name == api.ModelVolumeName {
			subPath = mount.SubPath
		}
	}
	assert.Equal(t, "gemma4-weights", claimName)
	assert.Equal(t, "gemma-4-26B-A4B-it-NVFP4", subPath)
}

func TestBuilder_Build_PVCSubPathWithExistingModelMount(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build()}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "gemma", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "pvc://gemma4-weights/models/gemma-4"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{
					Name: api.ModelVolumeName,
					VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "gemma4-weights",
					}},
				}},
				Containers: []corev1.Container{{
					Name: "vllm",
					VolumeMounts: []corev1.VolumeMount{{
						Name: api.ModelVolumeName, MountPath: "/custom-models",
					}},
				}},
			}},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	podSpec := dep.Spec.Template.Spec
	require.Len(t, podSpec.Containers, 1)

	var modelMount, tmpMount *corev1.VolumeMount
	for i := range podSpec.Containers[0].VolumeMounts {
		mount := &podSpec.Containers[0].VolumeMounts[i]
		switch mount.Name {
		case api.ModelVolumeName:
			modelMount = mount
		case "tmp-scratch":
			tmpMount = mount
		}
	}
	require.NotNil(t, modelMount)
	assert.Equal(t, api.ModelMountPath, modelMount.MountPath)
	assert.True(t, modelMount.ReadOnly)
	assert.Equal(t, "models/gemma-4", modelMount.SubPath)
	require.NotNil(t, tmpMount)
	assert.Equal(t, "/tmp", tmpMount.MountPath)

	var tmpVolume *corev1.Volume
	for i := range podSpec.Volumes {
		if podSpec.Volumes[i].Name == "tmp-scratch" {
			tmpVolume = &podSpec.Volumes[i]
			break
		}
	}
	require.NotNil(t, tmpVolume)
	require.NotNil(t, tmpVolume.EmptyDir)
}

func TestBuilder_Build_PVCRootPreservesExistingModelMountSubPath(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build()}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "gemma", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "pvc://gemma4-weights"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "vllm",
					VolumeMounts: []corev1.VolumeMount{{
						Name: api.ModelVolumeName, MountPath: "/custom-models", SubPath: "models/gemma-4",
					}},
				}},
			}},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	var modelMount *corev1.VolumeMount
	for i := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
		mount := &dep.Spec.Template.Spec.Containers[0].VolumeMounts[i]
		if mount.Name == api.ModelVolumeName {
			modelMount = mount
			break
		}
	}
	require.NotNil(t, modelMount)
	assert.Equal(t, "models/gemma-4", modelMount.SubPath)
}

func TestBuilder_Build_PreservesExplicitModelMountPath(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build()}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "pvc://weights/model"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "vllm",
					Args: []string{"--model", "/custom-models"},
					VolumeMounts: []corev1.VolumeMount{{
						Name: api.ModelVolumeName, MountPath: "/custom-models", ReadOnly: true,
					}},
				}},
			}},
		},
	}

	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	mount := dep.Spec.Template.Spec.Containers[0].VolumeMounts[0]
	assert.Equal(t, "/custom-models", mount.MountPath)
	assert.Equal(t, []string{"--model", "/custom-models"}, dep.Spec.Template.Spec.Containers[0].Args[:2])
}

func TestBuilder_BuildStorageInitializer(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	builder := &Builder{
		Client:             client,
		LocalCosignKeyPath: "/etc/cosign/cosign.pub",
	}

	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI: "hf://mistralai/Mistral-7B",
				Storage: &servingv1alpha2.StorageSpec{
					SecretRef: &corev1.LocalObjectReference{Name: "hf-credentials"},
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "model-server",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "cosign-key",
									MountPath: "/etc/cosign",
									ReadOnly:  true,
								},
							},
						},
					},
				},
			},
		},
	}

	initContainer := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, initContainer)

	assert.Equal(t, "storage-initializer", initContainer.Name)
	assert.Equal(t, api.HuggingFaceInitializerImage, initContainer.Image)
	assert.Empty(t, initContainer.Command)
	assert.Equal(t, []string{"hf://mistralai/Mistral-7B", api.ModelMountPath}, initContainer.Args)
	require.Len(t, initContainer.EnvFrom, 1)
	require.NotNil(t, initContainer.EnvFrom[0].SecretRef)
	assert.Equal(t, "hf-credentials", initContainer.EnvFrom[0].SecretRef.Name)
	foundKeyEnv := false
	for _, env := range initContainer.Env {
		if env.Name == "CKODEX_LOCAL_COSIGN_KEY_PATH" && env.Value == "/etc/cosign/cosign.pub" {
			foundKeyEnv = true
			break
		}
	}
	assert.True(t, foundKeyEnv, "storage-initializer should receive CKODEX_LOCAL_COSIGN_KEY_PATH")

	// Check SecurityContext
	require.NotNil(t, initContainer.SecurityContext)
	assert.True(t, *initContainer.SecurityContext.ReadOnlyRootFilesystem)
	assert.Contains(t, initContainer.SecurityContext.Capabilities.Drop, corev1.Capability("ALL"))

	// Check /tmp scratch mount
	var foundTmp bool
	for _, m := range initContainer.VolumeMounts {
		if m.MountPath == "/tmp" {
			foundTmp = true
			break
		}
	}
	assert.True(t, foundTmp, "storage-initializer should have /tmp scratch mount")
	assert.Contains(t, initContainer.VolumeMounts, corev1.VolumeMount{
		Name:      "cosign-key",
		MountPath: "/etc/cosign",
		ReadOnly:  true,
	})

	// Ckodex image fallback
	llmSvc.Spec.Model.URI = "s3://bucket/model"
	initContainerCustom := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, initContainerCustom)
	assert.Equal(t, api.CKodexStorageInitializerImage, initContainerCustom.Image)

	// Legacy huggingface:// objects remain readable through the compatibility
	// initializer, while new API objects use the canonical KServe hf:// scheme.
	llmSvc.Spec.Model.URI = "huggingface://mistralai/Mistral-7B"
	initContainerLegacyHF := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, initContainerLegacyHF)
	assert.Equal(t, api.CKodexStorageInitializerImage, initContainerLegacyHF.Image)

	// Air-gapped HF references become OCI references before initializer selection.
	airGapBuilder := &Builder{Client: client, AirGappedMode: true, LocalRegistry: "registry.internal"}
	llmSvc.Spec.Model.URI = "hf://mistralai/Mistral-7B"
	initContainerAirGap := airGapBuilder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, initContainerAirGap)
	assert.Equal(t, airGapBuilder.rewriteImage(api.CKodexStorageInitializerImage), initContainerAirGap.Image)
	assert.Equal(t, []string{"oci://registry.internal/hf/mistralai/Mistral-7B", api.ModelMountPath}, initContainerAirGap.Args)
	assert.Empty(t, initContainerAirGap.Command)

	// PVC skip test
	llmSvc.Spec.Model.URI = "pvc://my-pvc"
	initContainerPVC := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	assert.Nil(t, initContainerPVC, "Storage initializer should be nil for pvc:// URI")
}

func TestBuilder_BuildStorageInitializer_PreservesHFImageOverride(t *testing.T) {
	builder := &Builder{
		Client:             fake.NewClientBuilder().Build(),
		HFInitializerImage: "registry.internal/hf-initializer@sha256:1234",
	}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://org/model@release"},
		},
	}

	container := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, container)
	assert.Equal(t, builder.HFInitializerImage, container.Image)
	assert.Equal(t, []string{"hf://org/model@release", api.ModelMountPath}, container.Args)
	for _, arg := range container.Args {
		assert.NotContains(t, arg, "pip install")
	}
}

func TestBuilder_BuildStorageInitializer_HFMirrorUsesXetInitializer(t *testing.T) {
	builder := &Builder{
		Client:      fake.NewClientBuilder().Build(),
		HFMirrorURL: "https://hf-mirror.corp.internal",
	}
	llmSvc := &servingv1alpha2.LLMInferenceService{
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf-mirror://org/model@release"},
		},
	}

	container := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, container)
	assert.Equal(t, api.HuggingFaceInitializerImage, container.Image)
	assert.Equal(t, []string{"hf-mirror://org/model@release", api.ModelMountPath}, container.Args)
	assert.Contains(t, container.Env, corev1.EnvVar{
		Name: "HF_ENDPOINT", Value: builder.HFMirrorURL,
	})
}
