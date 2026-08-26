/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

func TestRerankerBuildContainerSelection(t *testing.T) {
	r := &RerankerInferenceServiceReconciler{}
	service := newRerankerSvc("builder", "default")
	service.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "awq"}
	service.Spec.Resources = &corev1.ResourceRequirements{Requests: corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("2"),
	}}

	container := r.buildContainer(service)
	assert.Equal(t, rerankerContainerName, container.Name)
	assert.Contains(t, container.Args, "--quantization")
	assert.Contains(t, container.Args, "awq")
	assert.Equal(t, service.Spec.Resources.Requests, container.Resources.Requests)
	assert.Equal(t, api.VLLMImage, container.Image)
	assert.Equal(t, int32(servingv1alpha2.RerankerServerPort), container.Ports[0].ContainerPort)
	require.NotNil(t, container.LivenessProbe)
	require.NotNil(t, container.ReadinessProbe)

	service.Spec.Quantization.Method = "gguf"
	container = r.buildContainer(service)
	assert.NotContains(t, container.Args, "--quantization")

	r.AirGappedMode = true
	r.LocalRegistry = "registry.local:5000"
	container = r.buildContainer(service)
	assert.Equal(t, "registry.local:5000/"+api.VLLMImage, container.Image)
}

func TestRerankerBuildDeploymentReplicasAndDefaults(t *testing.T) {
	r := &RerankerInferenceServiceReconciler{}
	service := newRerankerSvc("deployment", "default")
	deployment := r.buildDeployment(service)
	assert.Equal(t, int32(1), *deployment.Spec.Replicas)
	assert.Equal(t, deployment.Spec.Selector.MatchLabels, deployment.Spec.Template.Labels)

	service.Spec.Replicas = ptr.To(int32(2))
	deployment = r.buildDeployment(service)
	assert.Equal(t, int32(2), *deployment.Spec.Replicas)
}

func TestRerankerHelperPaths(t *testing.T) {
	assert.Equal(t, "registry.local/library/vllm", rewriteImageRegistry("registry.local", "library/vllm"))
	assert.Equal(t, "BAAI/bge-reranker-v2-m3", rerankerModelID("hf://BAAI/bge-reranker-v2-m3"))
	assert.Equal(t, "local/model", rerankerModelID("local/model"))
	labels := rerankerLabels("example")
	assert.Equal(t, "example", labels["app.kubernetes.io/instance"])
}
