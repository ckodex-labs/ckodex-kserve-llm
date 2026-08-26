package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestLLMLoraAdapterCoverageRuntimeTargetAndReadyBranches(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("runtime-lora", "default", "runtime-svc")
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}

	service, result, err := r.ensureTargetReady(context.Background(), lora)
	require.NoError(t, err)
	assert.Nil(t, service)
	assert.Equal(t, ctrlResult{}, ctrlResultFrom(result))

	target := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "runtime-svc", Namespace: "default"}}
	require.NoError(t, cl.Create(context.Background(), target))
	service, result, err = r.ensureTargetReady(context.Background(), lora)
	require.NoError(t, err)
	assert.Nil(t, service)
	assert.Equal(t, fiveSecondResult(), ctrlResultFrom(result))

	require.NoError(t, cl.Delete(context.Background(), target))
	target.Status.ModelReady = true
	target.ResourceVersion = ""
	require.NoError(t, cl.Create(context.Background(), target))
	service, result, err = r.ensureTargetReady(context.Background(), lora)
	require.NoError(t, err)
	assert.Equal(t, target.Name, service.Name)
	assert.Nil(t, result)
}

func TestLLMLoraAdapterCoverageMarkReadyAndRegistrationBranches(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("ready-lora", "default", "runtime-svc")
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}

	result, err := r.markLoraReady(context.Background(), lora)
	require.NoError(t, err)
	assert.Equal(t, int64(1), lora.Status.ActiveRevision)
	assert.Empty(t, result.RequeueAfter)
	assert.Len(t, lora.Status.Conditions, 1)

	result, err = r.markLoraReady(context.Background(), lora)
	require.NoError(t, err)
	assert.Empty(t, result.RequeueAfter)

	ready := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "runtime-svc", Namespace: "default"}, Status: servingv1alpha2.LLMInferenceServiceStatus{ModelReady: true}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "runtime-pod", Namespace: "default", Labels: map[string]string{"app.kubernetes.io/instance": "runtime-svc"}}, Status: corev1.PodStatus{Phase: corev1.PodPending}}
	require.NoError(t, cl.Create(context.Background(), ready))
	require.NoError(t, cl.Create(context.Background(), pod))
	result, err = r.registerAndMarkReady(context.Background(), lora, ready)
	require.NoError(t, err)
	assert.Empty(t, result.RequeueAfter)

	registered, err := r.registerOnPod(context.Background(), lora, *pod)
	require.NoError(t, err)
	assert.False(t, registered)
}

func TestLLMLoraAdapterCoverageRuntimeHTTPAndCircuitInitialization(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("http-lora", "default", "runtime-svc")
	r := &LLMLoraAdapterReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s, Recorder: record.NewFakeRecorder(10)}
	first := r.ensureLoadCircuitBreaker(lora)
	assert.NotNil(t, first)
	assert.Same(t, first, r.ensureLoadCircuitBreaker(lora))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/v1/load_lora_adapter", req.URL.Path)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	r.HTTPClient = &http.Client{Transport: &roundTripperMock{targetURL: srv.URL}}
	pod := corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.8"}}
	registered, err := r.registerOnPod(context.Background(), lora, pod)
	require.NoError(t, err)
	assert.True(t, registered)
}

func TestLLMLoraAdapterCircuitBreakersConcurrentReuseAndIsolation(t *testing.T) {
	s := buildLoraScheme(t)
	r := &LLMLoraAdapterReconciler{Scheme: s, Recorder: record.NewFakeRecorder(10)}
	lora := testLora("breaker-lora", "default", "breaker-svc")
	results := make(chan interface{}, 32)
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- r.ensureLoadCircuitBreaker(lora)
		}()
	}
	group.Wait()
	close(results)
	for result := range results {
		assert.Same(t, r.ensureLoadCircuitBreaker(lora), result)
	}

	otherTarget := lora.DeepCopy()
	otherTarget.Spec.TargetService = "other-svc"
	otherAdapter := lora.DeepCopy()
	otherAdapter.Name = "other-lora"
	assert.NotSame(t, r.ensureLoadCircuitBreaker(lora), r.ensureLoadCircuitBreaker(otherTarget))
	assert.NotSame(t, r.ensureLoadCircuitBreaker(lora), r.ensureLoadCircuitBreaker(otherAdapter))
	assert.NotSame(t, r.ensureLoadCircuitBreaker(lora), r.ensureUnloadCircuitBreaker(lora))
	assert.Len(t, r.circuitBreakers, 4)
}

// These aliases keep duration comparisons readable while avoiding repeated pointer plumbing.
type ctrlResult = struct {
	RequeueAfter time.Duration
}

func ctrlResultFrom(result *ctrl.Result) ctrlResult {
	if result == nil {
		return ctrlResult{}
	}
	return ctrlResult{RequeueAfter: result.RequeueAfter}
}

func fiveSecondResult() ctrlResult { return ctrlResult{RequeueAfter: 5 * time.Second} }
