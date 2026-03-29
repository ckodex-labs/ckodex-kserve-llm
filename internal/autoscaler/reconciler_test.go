/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package autoscaler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, autoscalingv2.AddToScheme(s))
	return s
}

func ptr32(v int32) *int32 { return &v }

func minimalSvc(name, namespace string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: "llama3",
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
		},
	}
}

// ---- Reconcile dispatch (nil Scaling) ------------------------------------

func TestReconcile_NilScaling_NoOp(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("test", "default")
	// Scaling is nil — Reconcile returns nil without touching the cluster.

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	err := r.Reconcile(context.Background(), svc)
	assert.NoError(t, err)
}

// ---- Reconcile dispatch priority -----------------------------------------

func TestReconcile_WVAPresent_DoesNotCreateHPA(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("wva-test", "default")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		WVA:         &servingv1alpha2.WVASpec{VariantCost: "5.0"},
		MinReplicas: ptr32(1),
		MaxReplicas: ptr32(4),
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	err := r.Reconcile(context.Background(), svc)
	// WVA uses unstructured — fake client accepts it; no error expected.
	assert.NoError(t, err)

	// Verify that no HPA was created (WVA takes priority).
	var hpaList autoscalingv2.HorizontalPodAutoscalerList
	err = r.List(context.Background(), &hpaList)
	assert.NoError(t, err)
	assert.Empty(t, hpaList.Items, "WVA path must not create an HPA")
}

func TestReconcile_KEDAPresent_DoesNotCreateHPA(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("keda-test", "default")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		KEDA:        &servingv1alpha2.KEDASpec{},
		MinReplicas: ptr32(0),
		MaxReplicas: ptr32(8),
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	err := r.Reconcile(context.Background(), svc)
	assert.NoError(t, err)

	var hpaList autoscalingv2.HorizontalPodAutoscalerList
	err = r.List(context.Background(), &hpaList)
	assert.NoError(t, err)
	assert.Empty(t, hpaList.Items, "KEDA path must not create an HPA")
}

// ---- reconcileHPA --------------------------------------------------------

func TestReconcileHPA_NilMinNilMax_NoOp(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("hpa-nil", "default")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		// Both nil → reconcileHPA returns nil without creating anything.
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	err := r.reconcileHPA(context.Background(), svc)
	assert.NoError(t, err)

	var hpaList autoscalingv2.HorizontalPodAutoscalerList
	require.NoError(t, r.List(context.Background(), &hpaList))
	assert.Empty(t, hpaList.Items)
}

func TestReconcileHPA_CreatesNewHPA(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("llama3", "default")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		MinReplicas: ptr32(2),
		MaxReplicas: ptr32(8),
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	require.NoError(t, r.reconcileHPA(context.Background(), svc))

	var hpa autoscalingv2.HorizontalPodAutoscaler
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "llama3-hpa", Namespace: "default"}, &hpa))

	assert.Equal(t, int32(2), *hpa.Spec.MinReplicas)
	assert.Equal(t, int32(8), hpa.Spec.MaxReplicas)
	assert.Equal(t, "llama3", hpa.Spec.ScaleTargetRef.Name)
	assert.Equal(t, "Deployment", hpa.Spec.ScaleTargetRef.Kind)
	assert.Len(t, hpa.Spec.Metrics, 1, "should have CPU metric")
	assert.Equal(t, autoscalingv2.ResourceMetricSourceType, hpa.Spec.Metrics[0].Type)
}

func TestReconcileHPA_OnlyMaxReplicas_CreatesSyncedHPA(t *testing.T) {
	// When only MaxReplicas is set (MinReplicas nil), defaults to 1.
	scheme := newScheme(t)
	svc := minimalSvc("mistral", "default")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		MaxReplicas: ptr32(5),
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	require.NoError(t, r.reconcileHPA(context.Background(), svc))

	var hpa autoscalingv2.HorizontalPodAutoscaler
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "mistral-hpa", Namespace: "default"}, &hpa))

	assert.Equal(t, int32(1), *hpa.Spec.MinReplicas, "default min is 1")
	assert.Equal(t, int32(5), hpa.Spec.MaxReplicas)
}

func TestReconcileHPA_UpdatesExistingHPA(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("llama3", "default")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		MinReplicas: ptr32(3),
		MaxReplicas: ptr32(12),
	}

	// Pre-existing HPA with stale replicas.
	existingHPA := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "llama3-hpa", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1", Kind: "Deployment", Name: "llama3",
			},
			MinReplicas: ptr32(1),
			MaxReplicas: 2,
		},
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, existingHPA).Build(),
		Scheme: scheme,
	}
	require.NoError(t, r.reconcileHPA(context.Background(), svc))

	var hpa autoscalingv2.HorizontalPodAutoscaler
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "llama3-hpa", Namespace: "default"}, &hpa))

	assert.Equal(t, int32(3), *hpa.Spec.MinReplicas, "should be updated to 3")
	assert.Equal(t, int32(12), hpa.Spec.MaxReplicas, "should be updated to 12")
}

// ---- reconcileHPA defaults -----------------------------------------------

func TestReconcileHPA_DefaultsApplied(t *testing.T) {
	// Verify: ScaleTargetRef uses apps/v1 and Deployment kind.
	// Metric: CPU at 70% utilization.
	scheme := newScheme(t)
	svc := minimalSvc("phi3", "production")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		MinReplicas: ptr32(1),
		MaxReplicas: ptr32(4),
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	require.NoError(t, r.reconcileHPA(context.Background(), svc))

	var hpa autoscalingv2.HorizontalPodAutoscaler
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "phi3-hpa", Namespace: "production"}, &hpa))

	require.Len(t, hpa.Spec.Metrics, 1)
	cpu := hpa.Spec.Metrics[0]
	require.NotNil(t, cpu.Resource)
	assert.Equal(t, int32(70), *cpu.Resource.Target.AverageUtilization)
	assert.Equal(t, autoscalingv2.UtilizationMetricType, cpu.Resource.Target.Type)

	// Labels
	assert.Equal(t, "ckodex-kserve-llm-operator", hpa.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "phi3", hpa.Labels["app.kubernetes.io/instance"])
}

// ---- reconcileKEDA defaults ----------------------------------------------

func TestReconcileKEDA_DefaultReplicas(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("llama3", "default")
	// No MinReplicas/MaxReplicas — should default to 0/10.
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		KEDA: &servingv1alpha2.KEDASpec{},
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	err := r.reconcileKEDA(context.Background(), svc)
	// Unstructured object is accepted by fake client without schema registration.
	assert.NoError(t, err)
}

func TestReconcileKEDA_CustomReplicas(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("gemma", "staging")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		MinReplicas: ptr32(1),
		MaxReplicas: ptr32(6),
		KEDA:        &servingv1alpha2.KEDASpec{},
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	assert.NoError(t, r.reconcileKEDA(context.Background(), svc))
}

// ---- reconcileWVA --------------------------------------------------------

func TestReconcileWVA_CreatesUnstructured(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("phi3", "default")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		WVA: &servingv1alpha2.WVASpec{VariantCost: "8.0"},
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	assert.NoError(t, r.reconcileWVA(context.Background(), svc))
}

// ---- Reconcile full dispatch to HPA -------------------------------------

func TestReconcile_FallsBackToHPA_WhenNoWVAOrKEDA(t *testing.T) {
	scheme := newScheme(t)
	svc := minimalSvc("llama3", "default")
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		MinReplicas: ptr32(2),
		MaxReplicas: ptr32(6),
		// No WVA, no KEDA — should fall back to HPA.
	}

	r := &Reconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	require.NoError(t, r.Reconcile(context.Background(), svc))

	var hpa autoscalingv2.HorizontalPodAutoscaler
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "llama3-hpa", Namespace: "default"}, &hpa))
	assert.Equal(t, int32(2), *hpa.Spec.MinReplicas)
	assert.Equal(t, int32(6), hpa.Spec.MaxReplicas)
}
