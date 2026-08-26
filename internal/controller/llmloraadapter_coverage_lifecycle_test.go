package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestLLMLoraAdapterCoverageCacheOwnerAndDeleteBranches(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("cache-lora", "default", "missing")
	cache := newLoraCache(lora)
	assert.NoError(t, validateLoraCacheOwner(cache, lora))

	bad := cache.DeepCopy()
	bad.Labels[loraCacheManagedByLabel] = "other"
	assert.Error(t, validateLoraCacheOwner(bad, lora))
	bad = cache.DeepCopy()
	bad.Spec.SourceModelURI = "hf://different"
	assert.Error(t, validateLoraCacheOwner(bad, lora))

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, cache).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	require.NoError(t, r.deleteLoraCache(context.Background(), lora))
	var deleted servingv1alpha2.LocalModelCache
	err := cl.Get(context.Background(), client.ObjectKey{Name: cache.Name}, &deleted)
	assert.True(t, apierrors.IsNotFound(err))
	require.NoError(t, r.deleteLoraCache(context.Background(), lora))
}

func TestLLMLoraAdapterCoverageUnloadAndFinalizeBranches(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("unload-lora", "default", "missing")
	r := &LLMLoraAdapterReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s, Recorder: record.NewFakeRecorder(10)}
	require.NoError(t, r.unloadFromTargetService(context.Background(), lora))
	assert.Len(t, r.circuitBreakers, 1)

	original := lora.DeepCopy()
	result, err := r.finalizeLora(context.Background(), lora, original)
	require.NoError(t, err)
	assert.Empty(t, result)

	lora.Finalizers = []string{loraFinalizer}
	original = lora.DeepCopy()
	cache := newLoraCache(lora)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, cache).Build()
	r = &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	result, err = r.finalizeLora(context.Background(), lora, original)
	require.NoError(t, err)
	assert.Empty(t, result.RequeueAfter)
	assert.NotContains(t, lora.Finalizers, loraFinalizer)
}

func TestLLMLoraAdapterCoverageUnloadRunningAndSkippedPods(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("unload-lora", "default", "unload-svc")
	svc := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "unload-svc", Namespace: "default"}}
	skipped := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default", Labels: map[string]string{"app.kubernetes.io/instance": "unload-svc"}}, Status: corev1.PodStatus{Phase: corev1.PodPending}}
	called := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called <- struct{}{}; w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	running := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: "default", Labels: map[string]string{"app.kubernetes.io/instance": "unload-svc"}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.9"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(svc, skipped, running).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10), HTTPClient: &http.Client{Transport: &roundTripperMock{targetURL: srv.URL}}}
	require.NoError(t, r.unloadFromTargetService(context.Background(), lora))
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("unload request not sent")
	}
}
