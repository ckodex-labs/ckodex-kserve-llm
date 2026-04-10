package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

func TestBuilder_Build(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	builder := &Builder{
		Client: client,
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

func TestBuilder_BuildStorageInitializer(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	builder := &Builder{
		Client: client,
	}

	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-model",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI: "hf://mistralai/Mistral-7B",
			},
		},
	}

	initContainer := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, initContainer)

	assert.Equal(t, "storage-initializer", initContainer.Name)
	assert.Equal(t, api.StorageInitializerImage, initContainer.Image)

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

	// Ckodex image fallback
	llmSvc.Spec.Model.URI = "s3://bucket/model"
	initContainerCustom := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, initContainerCustom)
	assert.Equal(t, api.CKodexStorageInitializerImage, initContainerCustom.Image)

	// PVC skip test
	llmSvc.Spec.Model.URI = "pvc://my-pvc"
	initContainerPVC := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	assert.Nil(t, initContainerPVC, "Storage initializer should be nil for pvc:// URI")
}
