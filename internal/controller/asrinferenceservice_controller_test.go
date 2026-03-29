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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// newASRScheme returns a scheme with all types needed for ASR controller tests.
func newASRScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

// newASRSvc creates a minimal ASRInferenceService for testing.
func newASRSvc(name, ns string, opts ...func(*servingv1alpha2.ASRInferenceService)) *servingv1alpha2.ASRInferenceService {
	svc := &servingv1alpha2.ASRInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: servingv1alpha2.ASRInferenceServiceSpec{
			Model:   servingv1alpha2.ModelSpec{URI: "hf://openai/whisper-large-v3", Name: "whisper-large-v3"},
			Runtime: servingv1alpha2.ASRRuntimeFasterWhisper,
		},
	}
	for _, o := range opts {
		o(svc)
	}
	return svc
}

// TestASRInferenceServiceReconciler_Create verifies finalizer + Deployment + Service
// are created on first reconcile.
func TestASRInferenceServiceReconciler_Create(t *testing.T) {
	scheme := newASRScheme(t)
	svc := newASRSvc("whisper-test", "ckodex-inference")
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(&servingv1alpha2.ASRInferenceService{}).
		Build()
	r := &ASRInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	// First reconcile: adds finalizer (requires a second reconcile for resources).
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "whisper-test"},
	})
	require.NoError(t, err)

	// Second reconcile: creates Deployment and Service.
	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "whisper-test"},
	})
	require.NoError(t, err)

	// Verify finalizer was registered.
	updated := &servingv1alpha2.ASRInferenceService{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "whisper-test"}, updated))
	assert.Contains(t, updated.Finalizers, asrFinalizer, "finalizer must be present")

	// Verify Deployment was created.
	dep := &appsv1.Deployment{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "whisper-test"}, dep),
		"Deployment must be created")

	// Verify Service was created.
	svcObj := &corev1.Service{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "whisper-test"}, svcObj),
		"Service must be created")
}

// TestASRInferenceServiceReconciler_Delete verifies the finalizer is removed on deletion.
func TestASRInferenceServiceReconciler_Delete(t *testing.T) {
	scheme := newASRScheme(t)
	now := metav1.Now()
	svc := newASRSvc("asr-del", "ckodex-inference")
	svc.Finalizers = []string{asrFinalizer}
	svc.DeletionTimestamp = &now

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(&servingv1alpha2.ASRInferenceService{}).
		Build()
	r := &ASRInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "asr-del"},
	})
	require.NoError(t, err)

	updated := &servingv1alpha2.ASRInferenceService{}
	getErr := cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "asr-del"}, updated)
	if errors.IsNotFound(getErr) {
		// Object fully removed — finalizer was cleared and the fake client GC deleted it.
		return
	}
	require.NoError(t, getErr)
	assert.NotContains(t, updated.Finalizers, asrFinalizer, "finalizer must be removed on deletion")
}

// TestASRInferenceServiceReconciler_NotFound verifies graceful handling of a missing CR.
func TestASRInferenceServiceReconciler_NotFound(t *testing.T) {
	scheme := newASRScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ASRInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "gone"},
	})
	assert.NoError(t, err, "missing CR must not return an error")
}

// TestASRInferenceServiceReconciler_TransformersNoImage verifies that runtime=transformers
// without spec.runtimeImage blocks reconciliation and sets Ready=False.
func TestASRInferenceServiceReconciler_TransformersNoImage(t *testing.T) {
	scheme := newASRScheme(t)
	svc := newASRSvc("cohere-transcribe", "ckodex-inference", func(s *servingv1alpha2.ASRInferenceService) {
		s.Spec.Runtime = servingv1alpha2.ASRRuntimeTransformers
		s.Spec.Model = servingv1alpha2.ModelSpec{
			URI:  "hf://CohereLabs/cohere-transcribe-03-2026",
			Name: "cohere-transcribe-03-2026",
		}
		// RuntimeImage intentionally omitted.
	})

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(&servingv1alpha2.ASRInferenceService{}).
		Build()
	r := &ASRInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	// Reconcile 1: add finalizer.
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "cohere-transcribe"},
	})
	require.NoError(t, err)

	// Reconcile 2: blocked by missing runtimeImage.
	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "cohere-transcribe"},
	})
	require.NoError(t, err)

	// Deployment must NOT have been created.
	dep := &appsv1.Deployment{}
	err = cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "cohere-transcribe"}, dep)
	assert.True(t, errors.IsNotFound(err), "Deployment must not be created when runtimeImage is missing")
}

// TestASRInferenceServiceReconciler_TransformersWithImage verifies that runtime=transformers
// with a user-supplied image proceeds to create the Deployment.
func TestASRInferenceServiceReconciler_TransformersWithImage(t *testing.T) {
	scheme := newASRScheme(t)
	svc := newASRSvc("cohere-transcribe-full", "ckodex-inference", func(s *servingv1alpha2.ASRInferenceService) {
		s.Spec.Runtime = servingv1alpha2.ASRRuntimeTransformers
		s.Spec.RuntimeImage = "ghcr.io/ckodex-labs/asr-transformers-server:v0.1.0"
		s.Spec.Model = servingv1alpha2.ModelSpec{
			URI:  "hf://CohereLabs/cohere-transcribe-03-2026",
			Name: "cohere-transcribe-03-2026",
		}
	})

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(&servingv1alpha2.ASRInferenceService{}).
		Build()
	r := &ASRInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	for range 2 {
		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: "ckodex-inference", Name: "cohere-transcribe-full"},
		})
		require.NoError(t, err)
	}

	dep := &appsv1.Deployment{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Namespace: "ckodex-inference", Name: "cohere-transcribe-full"}, dep))

	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "ghcr.io/ckodex-labs/asr-transformers-server:v0.1.0",
		dep.Spec.Template.Spec.Containers[0].Image,
		"custom runtimeImage must be used")
	assert.Equal(t, asrContainerName, dep.Spec.Template.Spec.Containers[0].Name)
}

// TestBuildASRContainer_FasterWhisperDefaults verifies the default image and
// environment variables for the faster-whisper runtime.
func TestBuildASRContainer_FasterWhisperDefaults(t *testing.T) {
	r := &ASRInferenceServiceReconciler{
		Recorder: record.NewFakeRecorder(10),
	}
	svc := newASRSvc("wh", "ns")
	svc.Spec.Languages = []string{"en", "fr"}

	c := r.buildASRContainer(svc)

	assert.Equal(t, asrContainerName, c.Name)
	assert.Equal(t, "ghcr.io/fedirz/faster-whisper-server:latest-cpu", c.Image,
		"default faster-whisper image must be used when runtimeImage is empty")
	assert.Equal(t, int32(asrServerPort), c.Ports[0].ContainerPort)

	var modelEnv, langEnv string
	for _, e := range c.Env {
		switch e.Name {
		case "MODEL":
			modelEnv = e.Value
		case "LANGUAGE":
			langEnv = e.Value
		}
	}
	assert.Equal(t, "hf://openai/whisper-large-v3", modelEnv)
	assert.Equal(t, "en,fr", langEnv)
}

// TestBuildASRContainer_OverrideImage verifies spec.runtimeImage overrides the default.
func TestBuildASRContainer_OverrideImage(t *testing.T) {
	r := &ASRInferenceServiceReconciler{
		Recorder: record.NewFakeRecorder(10),
	}
	svc := newASRSvc("wh", "ns", func(s *servingv1alpha2.ASRInferenceService) {
		s.Spec.RuntimeImage = "my-registry.io/custom-asr:v2"
	})

	c := r.buildASRContainer(svc)
	assert.Equal(t, "my-registry.io/custom-asr:v2", c.Image)
}

// TestBuildASRDeployment_ReplicasDefault verifies the Deployment gets 1 replica by default.
func TestBuildASRDeployment_ReplicasDefault(t *testing.T) {
	r := &ASRInferenceServiceReconciler{
		Recorder: record.NewFakeRecorder(10),
	}
	svc := newASRSvc("wh", "ns")

	dep := r.buildASRDeployment(svc)
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)
}

// TestBuildASRDeployment_CustomReplicas verifies spec.replicas is propagated.
func TestBuildASRDeployment_CustomReplicas(t *testing.T) {
	r := &ASRInferenceServiceReconciler{
		Recorder: record.NewFakeRecorder(10),
	}
	svc := newASRSvc("wh", "ns", func(s *servingv1alpha2.ASRInferenceService) {
		s.Spec.Replicas = ptr.To(int32(3))
	})

	dep := r.buildASRDeployment(svc)
	assert.Equal(t, int32(3), *dep.Spec.Replicas)
}

// TestBuildASRDeployment_SidecarsMerged verifies that user-supplied containers
// in spec.template are appended after the primary asr-server container.
func TestBuildASRDeployment_SidecarsMerged(t *testing.T) {
	r := &ASRInferenceServiceReconciler{
		Recorder: record.NewFakeRecorder(10),
	}
	svc := newASRSvc("wh", "ns", func(s *servingv1alpha2.ASRInferenceService) {
		s.Spec.Template.Spec.Containers = []corev1.Container{
			{Name: "prometheus-exporter", Image: "prom/exporter:latest"},
		}
	})

	dep := r.buildASRDeployment(svc)
	require.Len(t, dep.Spec.Template.Spec.Containers, 2)
	assert.Equal(t, asrContainerName, dep.Spec.Template.Spec.Containers[0].Name,
		"asr-server must be the first (primary) container")
	assert.Equal(t, "prometheus-exporter", dep.Spec.Template.Spec.Containers[1].Name,
		"user sidecar must be appended")
}

// TestBuildASRService_Spec verifies Service selector, port, and type.
func TestBuildASRService_Spec(t *testing.T) {
	r := &ASRInferenceServiceReconciler{
		Recorder: record.NewFakeRecorder(10),
	}
	svc := newASRSvc("asr-svc", "ckodex-inference")

	s := r.buildASRService(svc)
	assert.Equal(t, corev1.ServiceTypeClusterIP, s.Spec.Type)
	assert.Equal(t, int32(asrServerPort), s.Spec.Ports[0].Port)
	assert.Equal(t, "asrinferenceservice", s.Spec.Selector["app.kubernetes.io/name"])
	assert.Equal(t, "asr-svc", s.Spec.Selector["app.kubernetes.io/instance"])
}

// TestDefaultASRRuntimeImage verifies correct defaults per runtime.
func TestDefaultASRRuntimeImage(t *testing.T) {
	assert.Equal(t,
		"ghcr.io/fedirz/faster-whisper-server:latest-cpu",
		servingv1alpha2.DefaultASRRuntimeImage(servingv1alpha2.ASRRuntimeFasterWhisper),
	)
	// transformers has no default — user must supply runtimeImage.
	assert.Empty(t, servingv1alpha2.DefaultASRRuntimeImage(servingv1alpha2.ASRRuntimeTransformers),
		"transformers runtime must not have a default image")
}

// TestASRReconcile_UpdateExisting verifies that Deployment and Service are updated
// when the spec changes.
func TestASRReconcile_UpdateExisting(t *testing.T) {
	scheme := newASRScheme(t)
	svc := newASRSvc("update-asr", "ns")
	svc.Finalizers = []string{asrFinalizer}

	// Pre-create Deployment with old image.
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "update-asr", Namespace: "ns"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: asrLabels("update-asr")},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: asrLabels("update-asr")},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: asrContainerName, Image: "old-image"}}},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc, dep).
		WithStatusSubresource(&servingv1alpha2.ASRInferenceService{}).
		Build()
	r := &ASRInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "update-asr"},
	})
	require.NoError(t, err)

	updatedDep := &appsv1.Deployment{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "update-asr"}, updatedDep))
	assert.Equal(t, servingv1alpha2.DefaultASRRuntimeImage(servingv1alpha2.ASRRuntimeFasterWhisper),
		updatedDep.Spec.Template.Spec.Containers[0].Image, "Image must be updated to default")
}

// TestASRSyncStatus_Replicas verifies that ASR status is updated based on Deployment.
func TestASRSyncStatus_Replicas(t *testing.T) {
	scheme := newASRScheme(t)
	svc := newASRSvc("sync-status", "ns")
	svc.Finalizers = []string{asrFinalizer}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "sync-status", Namespace: "ns"},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc, dep).
		WithStatusSubresource(&servingv1alpha2.ASRInferenceService{}).
		Build()
	r := &ASRInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "sync-status"},
	})
	require.NoError(t, err)

	updated := &servingv1alpha2.ASRInferenceService{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sync-status"}, updated))

	assert.Equal(t, int32(1), updated.Status.Replicas)
	cond := meta.FindStatusCondition(updated.Status.Conditions, servingv1alpha2.ASRConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

// TestASRSyncStatus_NotFound verifies coverage for the case where Deployment is not found during status sync.
func TestASRSyncStatus_NotFound(t *testing.T) {
	scheme := newASRScheme(t)
	svc := newASRSvc("sync-nf", "ns")
	svc.Finalizers = []string{asrFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(&servingv1alpha2.ASRInferenceService{}).
		Build()
	r := &ASRInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	// Create a context that will trigger status sync but skip deployment creation in reconcile.
	// We'll call syncASRStatus directly to hit the apierrors.IsNotFound branch.
	err := r.syncASRStatus(context.Background(), svc, svc.DeepCopy())
	require.NoError(t, err)
	assert.Equal(t, int32(0), svc.Status.Replicas)
}
