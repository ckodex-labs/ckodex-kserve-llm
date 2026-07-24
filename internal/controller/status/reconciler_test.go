package status

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	kserveintegration "github.com/ckodex-labs/kserve-llm-operator/internal/kserve"
)

func TestReconciler_Update_Gating(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = servingv1alpha2.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	llm := &servingv1alpha2.LLMInferenceService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "serving.ckodex.com/v1alpha2",
			Kind:       "LLMInferenceService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "default",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: "gemma-4-e2b",
			},
		},
	}

	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "default",
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1,
			Replicas:      1,
		},
	}

	t.Run("Hardening Disabled (Default)", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&servingv1alpha2.LLMInferenceService{}).
			WithObjects(llm, deploy).
			Build()

		r := &Reconciler{
			Client:          fakeClient,
			EnableHardening: false,
		}

		testLLM := llm.DeepCopy()
		testLLM.Status = servingv1alpha2.LLMInferenceServiceStatus{}

		err := r.Update(context.Background(), testLLM, llm, true, nil)
		assert.NoError(t, err)

		// Verify DeploymentReady condition is NOT set
		found := false
		for _, c := range testLLM.Status.Conditions {
			if c.Type == servingv1alpha2.ConditionDeploymentReady {
				found = true
				break
			}
		}
		assert.False(t, found, "ConditionDeploymentReady should not be set when hardening is disabled")
	})

	t.Run("Hardening Enabled", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&servingv1alpha2.LLMInferenceService{}).
			WithObjects(llm, deploy).
			Build()

		r := &Reconciler{
			Client:          fakeClient,
			EnableHardening: true,
		}

		metrics := &servingv1alpha2.AdaptiveMetrics{
			P50Latency: "25ms",
			P99Latency: "150ms",
			LoadLevel:  "Light",
		}

		testLLM := llm.DeepCopy()
		testLLM.Status = servingv1alpha2.LLMInferenceServiceStatus{}

		err := r.Update(context.Background(), testLLM, llm, true, metrics)
		assert.NoError(t, err)

		// Verify DeploymentReady condition IS set
		found := false
		for _, c := range testLLM.Status.Conditions {
			if c.Type == servingv1alpha2.ConditionDeploymentReady {
				found = true
				assert.Equal(t, metav1.ConditionTrue, c.Status)
				break
			}
		}
		assert.True(t, found, "ConditionDeploymentReady should be set when hardening is enabled")

		// Verify metrics are set
		assert.NotNil(t, testLLM.Status.AdaptiveMetrics)
		assert.Equal(t, "150ms", testLLM.Status.AdaptiveMetrics.P99Latency)
		assert.Equal(t, "Light", testLLM.Status.AdaptiveMetrics.LoadLevel)
	})
}
func TestReconciler_UpdateFromKServe_ProjectsReadyStatusAndURL(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = servingv1alpha2.AddToScheme(scheme)
	llm := &servingv1alpha2.LLMInferenceService{
		TypeMeta: metav1.TypeMeta{APIVersion: servingv1alpha2.GroupVersion, Kind: "LLMInferenceService"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "distributed", Namespace: "models",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{Name: "gemma", URI: "pvc://weights"},
		},
	}
	isvc := kserveintegration.NewInferenceService()
	isvc.SetName(llm.Name)
	isvc.SetNamespace(llm.Namespace)
	isvc.Object["status"] = map[string]interface{}{
		"url": "http://distributed.models.example",
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True"},
		},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&servingv1alpha2.LLMInferenceService{}).
		WithObjects(llm, isvc).
		Build()

	current := llm.DeepCopy()
	err := (&Reconciler{Client: fakeClient}).UpdateFromKServe(
		context.Background(), current, llm.DeepCopy(), false, nil,
	)
	assert.NoError(t, err)
	assert.True(t, current.Status.ModelReady)
	assert.Equal(t, int32(1), current.Status.Replicas)
	assert.Equal(t, "http://distributed.models.example", current.Status.URL)
}

func TestReconciler_Update_DistributedConditions(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = servingv1alpha2.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	replicas := int32(2)
	llm := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "distributed", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model:   servingv1alpha2.ModelSpec{Name: "model"},
			KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{Connector: "lmcache"}},
			Prefill: &servingv1alpha2.PrefillSpec{Replicas: &replicas},
		},
	}
	prefill := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "distributed-prefill", Namespace: "default"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 2}}
	primary := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "distributed", Namespace: "default"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&servingv1alpha2.LLMInferenceService{}).WithObjects(llm, primary, prefill).Build()
	testLLM := llm.DeepCopy()
	require.NoError(t, (&Reconciler{Client: client}).Update(context.Background(), testLLM, llm, false, nil))
	conditions := map[string]metav1.ConditionStatus{}
	for _, condition := range testLLM.Status.Conditions {
		conditions[condition.Type] = condition.Status
	}
	assert.Equal(t, metav1.ConditionTrue, conditions[servingv1alpha2.ConditionKVTransferConfigured])
	assert.Equal(t, metav1.ConditionTrue, conditions[servingv1alpha2.ConditionPrefillReady])
}
