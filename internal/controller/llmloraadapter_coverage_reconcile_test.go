package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestLLMLoraAdapterCoveragePreparationAndProgressBranches(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("prepare-lora", "default", "svc")
	original := lora.DeepCopy()
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	result, err := r.prepareLora(context.Background(), lora, original)
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "proposed", lora.Status.StatePlanes.Lifecycle)
	assert.Contains(t, lora.Finalizers, loraFinalizer)
	assert.Len(t, lora.Status.Conditions, 1)

	result, err = r.prepareLora(context.Background(), lora, lora.DeepCopy())
	require.NoError(t, err)
	assert.Nil(t, result)

	lora.Status.Conditions = []metav1.Condition{{Type: servingv1alpha2.AdapterConditionReady}}
	require.NoError(t, r.ensureProgressingCondition(context.Background(), lora))
	assert.Len(t, lora.Status.Conditions, 1)
}

func TestLLMLoraAdapterCoverageQuarantinePreparation(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("quarantine-lora", "default", "missing")
	lora.Status.StatePlanes.Lifecycle = "quarantined"
	lora.Status.StatePlanes.Trust = "denied"
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	result, err := r.prepareLora(context.Background(), lora, lora.DeepCopy())
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.RequeueAfter)
}

func TestLLMLoraAdapterCoverageCacheReadinessAndResultHelpers(t *testing.T) {
	assert.False(t, loraCacheReady(&servingv1alpha2.LocalModelCache{}))
	cache := &servingv1alpha2.LocalModelCache{Status: servingv1alpha2.LocalModelCacheStatus{Conditions: []metav1.Condition{{Type: servingv1alpha2.ConditionReady, Status: "False"}}}}
	assert.False(t, loraCacheReady(cache))
	cache.Status.Conditions[0].Status = "True"
	assert.True(t, loraCacheReady(cache))
	assert.Equal(t, ctrlResult{}, ctrlResultFrom(nil))
}
