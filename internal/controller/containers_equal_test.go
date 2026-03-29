/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func makeReconcilerForContainerTests(t *testing.T) *LLMInferenceServiceReconciler {
	t.Helper()
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	return &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}
}

func makeContainer(name, image string, args []string, env []corev1.EnvVar) corev1.Container {
	return corev1.Container{
		Name:  name,
		Image: image,
		Args:  args,
		Env:   env,
	}
}

// TestContainersEqual_Identical returns true for identical slices.
func TestContainersEqual_Identical(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	c := makeContainer("vllm", "vllm:latest", []string{"--port=8000"}, nil)
	assert.True(t, r.containersEqual([]corev1.Container{c}, []corev1.Container{c}))
}

// TestContainersEqual_EmptySlices returns true for two empty slices.
func TestContainersEqual_EmptySlices(t *testing.T) {
	r := makeReconcilerForContainerTests(t)
	assert.True(t, r.containersEqual(nil, nil))
	assert.True(t, r.containersEqual([]corev1.Container{}, []corev1.Container{}))
}

// TestContainersEqual_DifferentLength returns false when lengths differ.
func TestContainersEqual_DifferentLength(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	c := makeContainer("vllm", "vllm:latest", nil, nil)
	assert.False(t, r.containersEqual([]corev1.Container{c, c}, []corev1.Container{c}))
}

// TestContainersEqual_DifferentName returns false on name mismatch.
func TestContainersEqual_DifferentName(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	a := makeContainer("vllm", "vllm:latest", nil, nil)
	b := makeContainer("other", "vllm:latest", nil, nil)
	assert.False(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_DifferentImage returns false on image mismatch.
func TestContainersEqual_DifferentImage(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	a := makeContainer("vllm", "vllm:v1", nil, nil)
	b := makeContainer("vllm", "vllm:v2", nil, nil)
	assert.False(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_DifferentArgs returns false on args mismatch.
func TestContainersEqual_DifferentArgs(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	a := makeContainer("vllm", "vllm:latest", []string{"--port=8000"}, nil)
	b := makeContainer("vllm", "vllm:latest", []string{"--port=9000"}, nil)
	assert.False(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_DifferentEnv returns false on env var mismatch.
func TestContainersEqual_DifferentEnv(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	envA := []corev1.EnvVar{{Name: "MODEL", Value: "llama3"}}
	envB := []corev1.EnvVar{{Name: "MODEL", Value: "mistral"}}
	a := makeContainer("vllm", "vllm:latest", nil, envA)
	b := makeContainer("vllm", "vllm:latest", nil, envB)
	assert.False(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_EnvUnsortedOrder returns true when env vars differ only in order.
func TestContainersEqual_EnvUnsortedOrder(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	envA := []corev1.EnvVar{
		{Name: "Z_VAR", Value: "z"},
		{Name: "A_VAR", Value: "a"},
	}
	envB := []corev1.EnvVar{
		{Name: "A_VAR", Value: "a"},
		{Name: "Z_VAR", Value: "z"},
	}
	a := makeContainer("vllm", "vllm:latest", nil, envA)
	b := makeContainer("vllm", "vllm:latest", nil, envB)
	assert.True(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_ExtraEnvInA returns false when A has extra env var.
func TestContainersEqual_ExtraEnvInA(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	envA := []corev1.EnvVar{
		{Name: "A_VAR", Value: "a"},
		{Name: "B_VAR", Value: "b"},
	}
	envB := []corev1.EnvVar{
		{Name: "A_VAR", Value: "a"},
	}
	a := makeContainer("vllm", "vllm:latest", nil, envA)
	b := makeContainer("vllm", "vllm:latest", nil, envB)
	assert.False(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_ExtraEnvInB returns false when B has extra env var.
func TestContainersEqual_ExtraEnvInB(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	envA := []corev1.EnvVar{{Name: "A_VAR", Value: "a"}}
	envB := []corev1.EnvVar{
		{Name: "A_VAR", Value: "a"},
		{Name: "B_VAR", Value: "b"},
	}
	a := makeContainer("vllm", "vllm:latest", nil, envA)
	b := makeContainer("vllm", "vllm:latest", nil, envB)
	assert.False(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_DifferentVolumeMounts returns false on volume mount mismatch.
func TestContainersEqual_DifferentVolumeMounts(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	a := corev1.Container{
		Name:  "vllm",
		Image: "vllm:latest",
		VolumeMounts: []corev1.VolumeMount{
			{Name: "model", MountPath: "/model"},
		},
	}
	b := corev1.Container{
		Name:  "vllm",
		Image: "vllm:latest",
		VolumeMounts: []corev1.VolumeMount{
			{Name: "model", MountPath: "/other"},
		},
	}
	assert.False(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_DifferentResources returns false on resource mismatch.
func TestContainersEqual_DifferentResources(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	a := corev1.Container{
		Name:  "vllm",
		Image: "vllm:latest",
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			},
		},
	}
	b := corev1.Container{
		Name:  "vllm",
		Image: "vllm:latest",
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("8"),
			},
		},
	}
	assert.False(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_SameResources returns true for identical resource requirements.
func TestContainersEqual_SameResources(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	res := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		},
	}
	a := corev1.Container{Name: "vllm", Image: "vllm:latest", Resources: res}
	b := corev1.Container{Name: "vllm", Image: "vllm:latest", Resources: res}
	assert.True(t, r.containersEqual([]corev1.Container{a}, []corev1.Container{b}))
}

// TestContainersEqual_MultipleContainers checks all containers pairwise.
func TestContainersEqual_MultipleContainers(t *testing.T) {
	r := makeReconcilerForContainerTests(t)

	c1 := makeContainer("vllm", "vllm:v1", nil, nil)
	c2 := makeContainer("sidecar", "sidecar:v1", nil, nil)
	c2diff := makeContainer("sidecar", "sidecar:v2", nil, nil) // different image

	assert.True(t, r.containersEqual([]corev1.Container{c1, c2}, []corev1.Container{c1, c2}))
	assert.False(t, r.containersEqual([]corev1.Container{c1, c2}, []corev1.Container{c1, c2diff}))
}
