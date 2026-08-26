package deployment

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetClusterGPUCapacitySumsNVIDIARequestsOnly(t *testing.T) {
	nodes := []corev1.Node{
		{Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("2")}}},
		{Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"amd.com/gpu": resource.MustParse("8")}}},
		{Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")}}},
	}
	assert.Equal(t, 3, GetClusterGPUCapacity(nodes))
}

func TestCheckGPURequirementsReportsSatisfiedAndInsufficientCapacity(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("2")}}}}}}}}
	assert.Equal(t, "", mustGPUMessage(t, service, 2))
	assert.Equal(t, "Requested 2 GPUs but only 1 available in cluster", mustGPUMessage(t, service, 1))
	assert.Equal(t, "", mustGPUMessage(t, &servingv1alpha2.LLMInferenceService{}, 0))
}

func mustGPUMessage(t *testing.T, service *servingv1alpha2.LLMInferenceService, capacity int) string {
	t.Helper()
	ok, message := CheckGPURequirements(service, capacity)
	assert.Equal(t, message == "", ok)
	return message
}
