package status

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
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

		err := r.Update(context.Background(), testLLM, llm, true)
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

		testLLM := llm.DeepCopy()
		testLLM.Status = servingv1alpha2.LLMInferenceServiceStatus{}

		err := r.Update(context.Background(), testLLM, llm, true)
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
	})
}
