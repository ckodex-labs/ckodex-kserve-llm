package deployment

import (
	"context"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuilder_BuildStorageInitializer(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	builder := &Builder{Client: client, LocalCosignKeyPath: "/etc/cosign/cosign.pub"}
	llmSvc := newStorageInitializerService()
	container := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, container)
	assertStorageInitializerBase(t, container)
	assertStorageInitializerVariants(t, builder, client, llmSvc)
}

func newStorageInitializerService() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-model"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://mistralai/Mistral-7B", Storage: &servingv1alpha2.StorageSpec{
				SecretRef: &corev1.LocalObjectReference{Name: "hf-credentials"},
			}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "model-server", VolumeMounts: []corev1.VolumeMount{{Name: "cosign-key", MountPath: "/etc/cosign", ReadOnly: true}},
			}}}},
		},
	}
}

func assertStorageInitializerBase(t *testing.T, c *corev1.Container) {
	assert.Equal(t, "storage-initializer", c.Name)
	assert.Equal(t, api.HuggingFaceInitializerImage, c.Image)
	assert.Empty(t, c.Command)
	assert.Equal(t, []string{"hf://mistralai/Mistral-7B", api.ModelMountPath}, c.Args)
	require.Len(t, c.EnvFrom, 1)
	require.NotNil(t, c.EnvFrom[0].SecretRef)
	assert.Equal(t, "hf-credentials", c.EnvFrom[0].SecretRef.Name)
	assertStorageSecurity(t, c)
}

func assertStorageSecurity(t *testing.T, c *corev1.Container) {
	foundKey := false
	for _, env := range c.Env {
		if env.Name == "CKODEX_LOCAL_COSIGN_KEY_PATH" && env.Value == "/etc/cosign/cosign.pub" {
			foundKey = true
		}
	}
	assert.True(t, foundKey, "storage-initializer should receive CKODEX_LOCAL_COSIGN_KEY_PATH")
	require.NotNil(t, c.SecurityContext)
	assert.True(t, *c.SecurityContext.ReadOnlyRootFilesystem)
	assert.Contains(t, c.SecurityContext.Capabilities.Drop, corev1.Capability("ALL"))
	assert.Contains(t, c.VolumeMounts, corev1.VolumeMount{Name: "cosign-key", MountPath: "/etc/cosign", ReadOnly: true})
	assertStorageScratch(t, c)
}

func assertStorageScratch(t *testing.T, c *corev1.Container) {
	var found bool
	for _, m := range c.VolumeMounts {
		if m.MountPath == "/tmp" {
			found = true
		}
	}
	assert.True(t, found, "storage-initializer should have /tmp scratch mount")
}

func assertStorageInitializerVariants(t *testing.T, builder *Builder, client client.Client, svc *servingv1alpha2.LLMInferenceService) {
	svc.Spec.Model.URI = "s3://bucket/model"
	custom := builder.BuildStorageInitializer(context.Background(), svc, HardwareNVIDIA, nil)
	require.NotNil(t, custom)
	assert.Equal(t, api.CKodexStorageInitializerImage, custom.Image)
	svc.Spec.Model.URI = "huggingface://mistralai/Mistral-7B"
	legacy := builder.BuildStorageInitializer(context.Background(), svc, HardwareNVIDIA, nil)
	require.NotNil(t, legacy)
	assert.Equal(t, api.CKodexStorageInitializerImage, legacy.Image)
	airGap := &Builder{Client: client, AirGappedMode: true, LocalRegistry: "registry.internal"}
	svc.Spec.Model.URI = "hf://mistralai/Mistral-7B"
	container := airGap.BuildStorageInitializer(context.Background(), svc, HardwareNVIDIA, nil)
	require.NotNil(t, container)
	assert.Equal(t, airGap.rewriteImage(api.CKodexStorageInitializerImage), container.Image)
	assert.Equal(t, []string{"oci://registry.internal/hf/mistralai/Mistral-7B", api.ModelMountPath}, container.Args)
	assert.Empty(t, container.Command)
	svc.Spec.Model.URI = "pvc://my-pvc"
	assert.Nil(t, builder.BuildStorageInitializer(context.Background(), svc, HardwareNVIDIA, nil))
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
