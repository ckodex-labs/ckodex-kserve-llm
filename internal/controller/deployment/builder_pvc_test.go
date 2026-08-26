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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
	llmSvc := newPVCSubPathService()
	dep := builder.Build(context.Background(), llmSvc, 1, HardwareNVIDIA, nil)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	assertPVCModelAndScratchMounts(t, dep.Spec.Template.Spec)
}

func newPVCSubPathService() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "gemma", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "pvc://gemma4-weights/models/gemma-4"},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: api.ModelVolumeName, VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "gemma4-weights"},
				}}},
				Containers: []corev1.Container{{Name: "vllm", VolumeMounts: []corev1.VolumeMount{{
					Name: api.ModelVolumeName, MountPath: "/custom-models",
				}}}},
			}},
		},
	}
}

func assertPVCModelAndScratchMounts(t *testing.T, podSpec corev1.PodSpec) {
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
	assertPVCVolume(t, podSpec)
}

func assertPVCVolume(t *testing.T, podSpec corev1.PodSpec) {
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
