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
	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA)
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

	// Ckodex image fallback
	llmSvc.Spec.Model.URI = "s3://bucket/model"
	initContainerCustom := builder.BuildStorageInitializer(context.Background(), llmSvc, HardwareNVIDIA, nil)
	require.NotNil(t, initContainerCustom)
	assert.Equal(t, api.CKodexStorageInitializerImage, initContainerCustom.Image)
}
