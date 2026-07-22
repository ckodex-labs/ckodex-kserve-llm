package deployment

import (
	"context"
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
	assert.Equal(t, []string{"/bin/sh", "-ec"}, initContainer.Command)
	assert.Contains(t, initContainer.Args[0], "hf download")
	assert.Contains(t, initContainer.Args[0], "hf-xet=="+api.HuggingFaceXetVersion)
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
