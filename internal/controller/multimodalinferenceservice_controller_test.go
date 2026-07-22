/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// newMultimodalScheme returns a scheme with all types needed for Multimodal controller tests.
func newMultimodalScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

// newMMSvc creates a minimal MultimodalInferenceService for testing.
func newMMSvc(name, ns string, opts ...func(*servingv1alpha2.MultimodalInferenceService)) *servingv1alpha2.MultimodalInferenceService {
	svc := &servingv1alpha2.MultimodalInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       types.UID("test-uid-" + name),
		},
		Spec: servingv1alpha2.MultimodalInferenceServiceSpec{
			Model:   servingv1alpha2.ModelSpec{URI: "hf://llava-hf/llava-v1.6-mistral-7b-hf", Name: "llava-v1.6-mistral-7b-hf"},
			Task:    servingv1alpha2.MultimodalTaskVisionLanguage,
			Runtime: servingv1alpha2.MultimodalRuntimeVLLM,
		},
	}
	for _, o := range opts {
		o(svc)
	}
	return svc
}

// TestMultimodalInferenceServiceReconciler_Create verifies finalizer + Deployment + Service
// are created on two reconcile passes.
func TestMultimodalInferenceServiceReconciler_Create(t *testing.T) {
	scheme := newMultimodalScheme(t)
	svc := newMMSvc("llava-test", "ckodex-inference")

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(&servingv1alpha2.MultimodalInferenceService{}).
		Build()
	r := &MultimodalInferenceServiceReconciler{Client: cl, Scheme: scheme}

	// Pass 1: adds finalizer.
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "llava-test"},
	})
	require.NoError(t, err)

	// Pass 2: creates Deployment and Service.
	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "llava-test"},
	})
	require.NoError(t, err)

	updated := &servingv1alpha2.MultimodalInferenceService{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "llava-test"}, updated))
	assert.Contains(t, updated.Finalizers, multimodalFinalizer, "finalizer must be present")

	dep := &appsv1.Deployment{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "llava-test"}, dep),
		"Deployment must be created")

	svcObj := &corev1.Service{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "llava-test"}, svcObj),
		"Service must be created")
}

// TestMultimodalInferenceServiceReconciler_Delete verifies the finalizer is removed on deletion.
func TestMultimodalInferenceServiceReconciler_Delete(t *testing.T) {
	scheme := newMultimodalScheme(t)
	now := metav1.Now()
	svc := newMMSvc("mm-del", "ckodex-inference")
	svc.Finalizers = []string{multimodalFinalizer}
	svc.DeletionTimestamp = &now

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(&servingv1alpha2.MultimodalInferenceService{}).
		Build()
	r := &MultimodalInferenceServiceReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "mm-del"},
	})
	require.NoError(t, err)

	updated := &servingv1alpha2.MultimodalInferenceService{}
	getErr := cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "mm-del"}, updated)
	if apierrors.IsNotFound(getErr) {
		// Object fully removed — finalizer was cleared and the fake client GC deleted it.
		return
	}
	require.NoError(t, getErr)
	assert.NotContains(t, updated.Finalizers, multimodalFinalizer, "finalizer must be removed")
}

// TestMultimodalInferenceServiceReconciler_NotFound verifies graceful handling of missing CR.
func TestMultimodalInferenceServiceReconciler_NotFound(t *testing.T) {
	scheme := newMultimodalScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &MultimodalInferenceServiceReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "gone"},
	})
	assert.NoError(t, err)
}

// TestBuildMultimodalContainer_VLLMDefaults verifies default vLLM image, args, and port.
func TestBuildMultimodalContainer_VLLMDefaults(t *testing.T) {
	r := &MultimodalInferenceServiceReconciler{}
	svc := newMMSvc("llava", "ns")

	c := r.buildMultimodalContainer(svc)

	assert.Equal(t, multimodalContainerName, c.Name)
	assert.Equal(t, "vllm/vllm-openai:v0.25.1", c.Image)
	assert.Equal(t, int32(servingv1alpha2.MultimodalServerPort), c.Ports[0].ContainerPort)

	// Model ID must be stripped of hf:// prefix.
	assert.Contains(t, c.Args, "llava-hf/llava-v1.6-mistral-7b-hf")
	assert.Contains(t, c.Args, "--model")

	// Default 1 image per prompt.
	assert.Contains(t, c.Args, "image=1")
	assert.Contains(t, c.Args, "--limit-mm-per-prompt")
}

// TestBuildMultimodalContainer_MaxImages verifies MaxImagesPerPrompt is propagated.
func TestBuildMultimodalContainer_MaxImages(t *testing.T) {
	r := &MultimodalInferenceServiceReconciler{}
	svc := newMMSvc("qwen-vl", "ns", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.Model = servingv1alpha2.ModelSpec{
			URI:  "hf://Qwen/Qwen2-VL-7B-Instruct",
			Name: "Qwen2-VL-7B-Instruct",
		}
		s.Spec.MaxImagesPerPrompt = ptr.To(int32(4))
	})

	c := r.buildMultimodalContainer(svc)
	assert.Contains(t, c.Args, "image=4", "MaxImagesPerPrompt must be used in --limit-mm-per-prompt")
}

// TestBuildMultimodalContainer_OverrideImage verifies spec.runtimeImage overrides the default.
func TestBuildMultimodalContainer_OverrideImage(t *testing.T) {
	r := &MultimodalInferenceServiceReconciler{}
	svc := newMMSvc("llava", "ns", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.RuntimeImage = "my-registry.io/vllm-patched:v0.7.0"
	})

	c := r.buildMultimodalContainer(svc)
	assert.Equal(t, "my-registry.io/vllm-patched:v0.7.0", c.Image)
}

// TestBuildMultimodalDeployment_ReplicasDefault verifies default 1 replica.
func TestBuildMultimodalDeployment_ReplicasDefault(t *testing.T) {
	r := &MultimodalInferenceServiceReconciler{}
	svc := newMMSvc("llava", "ns")

	dep := r.buildMultimodalDeployment(svc)
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)
}

// TestBuildMultimodalDeployment_CustomReplicas verifies spec.replicas is propagated.
func TestBuildMultimodalDeployment_CustomReplicas(t *testing.T) {
	r := &MultimodalInferenceServiceReconciler{}
	svc := newMMSvc("llava", "ns", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.Replicas = ptr.To(int32(2))
	})

	dep := r.buildMultimodalDeployment(svc)
	assert.Equal(t, int32(2), *dep.Spec.Replicas)
}

// TestBuildMultimodalService_Spec verifies Service selector and port.
func TestBuildMultimodalService_Spec(t *testing.T) {
	r := &MultimodalInferenceServiceReconciler{}
	svc := newMMSvc("mm-svc", "ckodex-inference")

	s := r.buildMultimodalService(svc)
	assert.Equal(t, corev1.ServiceTypeClusterIP, s.Spec.Type)
	assert.Equal(t, int32(servingv1alpha2.MultimodalServerPort), s.Spec.Ports[0].Port)
	assert.Equal(t, "multimodalinferenceservice", s.Spec.Selector["app.kubernetes.io/name"])
	assert.Equal(t, "mm-svc", s.Spec.Selector["app.kubernetes.io/instance"])
}

// TestMultimodalEndpointURL verifies task-specific URL generation.
func TestMultimodalEndpointURL(t *testing.T) {
	vlm := newMMSvc("qwen", "ckodex-inference")
	assert.Contains(t, multimodalEndpointURL(vlm), "/v1/chat/completions",
		"vision-language task must use chat completions endpoint")

	imgGen := newMMSvc("sdxl", "ckodex-inference", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.Task = servingv1alpha2.MultimodalTaskImageGeneration
	})
	assert.Contains(t, multimodalEndpointURL(imgGen), "/v1/images/generations",
		"image-generation task must use images endpoint")
}

// TestDefaultMultimodalRuntimeImage verifies correct defaults per runtime.
func TestDefaultMultimodalRuntimeImage(t *testing.T) {
	assert.Equal(t,
		"vllm/vllm-openai:v0.25.1",
		servingv1alpha2.DefaultMultimodalRuntimeImage(servingv1alpha2.MultimodalRuntimeVLLM),
	)
}

// TestBuildMultimodalContainer_TTS_LiquidAI verifies trust-remote-code for LiquidAI models.
func TestBuildMultimodalContainer_TTS_LiquidAI(t *testing.T) {
	r := &MultimodalInferenceServiceReconciler{}
	svc := newMMSvc("liquid-tts", "ns", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.Task = servingv1alpha2.MultimodalTaskTextToSpeech
		s.Spec.Model.URI = "hf://liquidai/LFM-audio-v1"
	})

	c := r.buildMultimodalContainer(svc)
	assert.Contains(t, c.Args, "--trust-remote-code")
	assert.Contains(t, c.Args, "--enforce-eager")
}

// TestBuildMultimodalContainer_TTS_Generic verifies TTS-specific flags without trust-remote-code.
func TestBuildMultimodalContainer_TTS_Generic(t *testing.T) {
	r := &MultimodalInferenceServiceReconciler{}
	svc := newMMSvc("generic-tts", "ns", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.Task = servingv1alpha2.MultimodalTaskTextToSpeech
		s.Spec.Model.URI = "hf://openai/whisper-large-v3"
	})

	c := r.buildMultimodalContainer(svc)
	assert.NotContains(t, c.Args, "--trust-remote-code")
	assert.Contains(t, c.Args, "--enforce-eager")
}

// TestMultimodalEndpointURL_TTS verifies the speech endpoint URL.
func TestMultimodalEndpointURL_TTS(t *testing.T) {
	svc := newMMSvc("tts-svc", "ns", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.Task = servingv1alpha2.MultimodalTaskTextToSpeech
	})
	url := multimodalEndpointURL(svc)
	assert.Contains(t, url, "/v1/audio/speech")
}

// TestMultimodalReconcile_InvalidModelURI verifies Ready=False on invalid scheme.
func TestMultimodalReconcile_InvalidModelURI(t *testing.T) {
	scheme := newMultimodalScheme(t)
	svc := newMMSvc("bad-vlm", "ns", func(s *servingv1alpha2.MultimodalInferenceService) {
		s.Spec.Model.URI = "s3://models/vlm"
	})
	svc.Finalizers = []string{multimodalFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(svc).
		Build()
	r := &MultimodalInferenceServiceReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "bad-vlm"},
	})
	require.NoError(t, err)

	updated := &servingv1alpha2.MultimodalInferenceService{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "bad-vlm"}, updated))

	var cond *metav1.Condition
	for i := range updated.Status.Conditions {
		if updated.Status.Conditions[i].Type == "Ready" {
			cond = &updated.Status.Conditions[i]
			break
		}
	}
	require.NotNil(t, cond, "Ready condition must be present. Conditions found: %+v", updated.Status.Conditions)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, "InvalidModelURI", cond.Reason)
}

// TestMultimodalReconcile_UpdateExistingDeployment verifies the deployment update path.
func TestMultimodalReconcile_UpdateExistingDeployment(t *testing.T) {
	scheme := newMultimodalScheme(t)
	svc := newMMSvc("update-vlm", "ns")
	svc.Finalizers = []string{multimodalFinalizer} // skip first pass

	// Ensure deployment already exists
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "update-vlm", Namespace: "ns"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: multimodalLabels("update-vlm")},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: multimodalLabels("update-vlm")},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "busybox", Image: "busybox"}}},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc, dep).
		WithStatusSubresource(&servingv1alpha2.MultimodalInferenceService{}).
		Build()
	r := &MultimodalInferenceServiceReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "update-vlm"},
	})
	require.NoError(t, err)

	updatedDep := &appsv1.Deployment{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "update-vlm"}, updatedDep))
	assert.Contains(t, updatedDep.Spec.Template.Spec.Containers[0].Image, "vllm-openai", "Image should have been updated")
}
