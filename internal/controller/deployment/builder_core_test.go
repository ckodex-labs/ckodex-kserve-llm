package deployment

import (
	"context"
	"strings"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuilder_Build(t *testing.T) {
	builder := &Builder{
		Client:             fake.NewClientBuilder().Build(),
		LocalCosignKeyPath: "/etc/cosign/cosign.pub",
	}
	llmSvc := newDefaultBuildService()
	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	require.NotNil(t, dep)
	assertBuilderDeployment(t, dep)
	assertBuilderContainer(t, dep.Spec.Template.Spec.Containers[0])
}

func newDefaultBuildService() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-model", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{Name: "mistralai/Mistral-7B", URI: "hf://mistralai/Mistral-7B"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "vllm"}},
			}},
		},
	}
}

func assertBuilderDeployment(t *testing.T, dep *appsv1.Deployment) {
	assert.Equal(t, "test-model", dep.Name)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)
	assert.Equal(t, appsv1.RecreateDeploymentStrategyType, dep.Spec.Strategy.Type)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
}

func assertBuilderContainer(t *testing.T, c corev1.Container) {
	assert.NotNil(t, c.Resources.Requests)
	assert.NotNil(t, c.Resources.Limits)
	require.NotNil(t, c.Lifecycle)
	require.NotNil(t, c.Lifecycle.PreStop)
	assertBuilderModelVolume(t, c)
	assertBuilderEnvironment(t, c)
	assertBuilderSecurity(t, c)
	assertBuilderProbes(t, c)
}

func assertBuilderModelVolume(t *testing.T, c corev1.Container) {
	var found bool
	for _, v := range c.VolumeMounts {
		if v.MountPath == api.ModelMountPath {
			found = true
			break
		}
	}
	assert.True(t, found, "model volume should be injected")
}

func assertBuilderEnvironment(t *testing.T, c corev1.Container) {
	envs := make(map[string]string)
	for _, e := range c.Env {
		envs[e.Name] = e.Value
	}
	assert.Equal(t, "/tmp", envs["HOME"])
	assert.Equal(t, "nonroot", envs["USER"])
	assert.Equal(t, "nonroot", envs["LOGNAME"])
}

func assertBuilderSecurity(t *testing.T, c corev1.Container) {
	require.NotNil(t, c.SecurityContext)
	assert.True(t, *c.SecurityContext.ReadOnlyRootFilesystem)
	assert.True(t, *c.SecurityContext.RunAsNonRoot)
	assert.Equal(t, int64(65532), *c.SecurityContext.RunAsUser)
	assert.Contains(t, c.SecurityContext.Capabilities.Drop, corev1.Capability("ALL"))
}

func assertBuilderProbes(t *testing.T, c corev1.Container) {
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

func TestBuilder_Build_SelectsHardwareDefaultBeforeRuntimeFallback(t *testing.T) {
	service := newDefaultBuildService()
	builder := &Builder{
		Client:       fake.NewClientBuilder().Build(),
		RuntimeImage: "vllm/vllm-openai:v0.28.0",
	}

	dep := builder.Build(context.Background(), service, 1, HardwareAppleSilicon, nil)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, VLLMGenericImage, container.Image)
	assert.Contains(t, container.Env, corev1.EnvVar{Name: "VLLM_TARGET_DEVICE", Value: "cpu"})
}

func TestBuilder_Build_UsesStableSelectorWhenModelIdentityChanges(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build()}
	first := newDefaultBuildService()
	second := first.DeepCopy()
	second.Spec.Model.Name = "new-model"

	firstDeployment := builder.Build(context.Background(), first, 1, HardwareNVIDIA, nil)
	secondDeployment := builder.Build(context.Background(), second, 1, HardwareNVIDIA, nil)
	assert.Equal(t, firstDeployment.Spec.Selector.MatchLabels, secondDeployment.Spec.Selector.MatchLabels)
	assert.Equal(t, "mistralai.Mistral-7B", firstDeployment.Spec.Template.Labels["serving.ckodex.com/model"])
	assert.Equal(t, "new-model", secondDeployment.Spec.Template.Labels["serving.ckodex.com/model"])
}

func TestBuilder_Build_PreservesExplicitRuntimeImageOnHardwareMismatch(t *testing.T) {
	service := newDefaultBuildService()
	service.Spec.Template.Spec.Containers[0].Image = "registry.example/custom-vllm:v1"
	builder := &Builder{
		Client:       fake.NewClientBuilder().Build(),
		RuntimeImage: "vllm/vllm-openai:v0.28.0",
	}

	dep := builder.Build(context.Background(), service, 1, HardwareAppleSilicon, nil)
	assert.Equal(t, "registry.example/custom-vllm:v1", dep.Spec.Template.Spec.Containers[0].Image)
}

func TestBuilder_Build_RuntimeImageAndMountedModelOverride(t *testing.T) {
	builder := &Builder{
		Client:       fake.NewClientBuilder().Build(),
		RuntimeImage: "registry.example/vllm:v0.28.0",
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
	assert.Equal(t, "registry.example/vllm:v0.28.0", c.Image)
	assert.Equal(t, []string{"--model", api.ModelMountPath}, c.Args[:2])
	assert.Contains(t, c.Args, "--max-model-len")
}

func TestBuilder_BuildWithRole_ConfiguresLMCacheTransfer(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build(), RuntimeImage: "vllm:v0.28.0"}
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
	builder := &Builder{Client: fake.NewClientBuilder().Build(), RuntimeImage: "vllm:v0.28.0"}
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
