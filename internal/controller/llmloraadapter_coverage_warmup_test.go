package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

var llmLoraControllerRegistrationID atomic.Uint64

func TestLLMLoraAdapterCoverageWarmupDeduplicatesRequests(t *testing.T) {
	called := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called <- struct{}{}; w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	r := &LLMLoraAdapterReconciler{HTTPClient: &http.Client{Transport: &roundTripperMock{targetURL: srv.URL}}}
	lora := testLora("warmup-lora", "default", "svc")
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "default"}, Status: corev1.PodStatus{PodIP: "10.0.0.10"}}
	r.scheduleWarmup(context.Background(), lora, pod)
	r.scheduleWarmup(context.Background(), lora, pod)
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("warmup request not sent")
	}
	r.warmupMu.Lock()
	assert.Len(t, r.warmupDone, 1)
	r.warmupMu.Unlock()
}

func TestLLMLoraAdapterCoverageWarmupHandlesHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	r := &LLMLoraAdapterReconciler{HTTPClient: srv.Client()}
	r.HTTPClient.Transport = &roundTripperMock{targetURL: srv.URL}
	r.performWarmup(context.Background(), "10.0.0.11", "adapter")
	srv.Close()
	r.performWarmup(context.Background(), "10.0.0.11", "adapter")
}

func TestLLMLoraAdapterCoverageWarmupMappings(t *testing.T) {
	adapter := testLora("adapter", "tenant", "svc")
	other := testLora("other", "tenant", "other-svc")
	assert.Equal(t, []reconcile.Request{{NamespacedName: types.NamespacedName{Name: adapter.Name, Namespace: adapter.Namespace}}}, requestsForTarget([]servingv1alpha2.LLMLoraAdapter{*adapter, *other}, "svc"))
	assert.Empty(t, requestsForTarget([]servingv1alpha2.LLMLoraAdapter{*adapter}, "missing"))

	cache := &servingv1alpha2.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{loraCacheManagedByLabel: "other"}}}
	assert.Empty(t, mapCacheToLora(context.Background(), cache))
	cache.Labels[loraCacheManagedByLabel] = loraCacheManagedByAdapter
	assert.Empty(t, mapCacheToLora(context.Background(), cache))
	cache.Annotations = map[string]string{loraCacheOwnerNamespace: "tenant", loraCacheOwnerName: "adapter"}
	requests := mapCacheToLora(context.Background(), cache)
	require.Len(t, requests, 1)
	assert.Equal(t, "tenant", requests[0].Namespace)
	assert.Equal(t, "adapter", requests[0].Name)
}

type llmLoraCoverageManager struct {
	ctrl.Manager
	client.Client
}

func (m llmLoraCoverageManager) GetClient() client.Client { return m.Client }

func TestLLMLoraAdapterCoveragePodMappingBranches(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("adapter", "tenant", "svc")
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora).Build()
	r := (&LLMLoraAdapterReconciler{Client: cl}).mapPodToLoras(llmLoraCoverageManager{Client: cl})
	assert.Nil(t, r(context.Background(), &corev1.Service{}))
	assert.Nil(t, r(context.Background(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}}))
	requests := r(context.Background(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "tenant", Labels: map[string]string{"app.kubernetes.io/instance": "svc"}}})
	require.Len(t, requests, 1)
	assert.Equal(t, "adapter", requests[0].Name)
}

type failingListClient struct{ client.Client }

func (failingListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return assert.AnError
}

func TestLLMLoraAdapterCoveragePodMappingListFailure(t *testing.T) {
	failed := (&LLMLoraAdapterReconciler{}).mapPodToLoras(llmLoraCoverageManager{Client: failingListClient{}})
	assert.Nil(t, failed(context.Background(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "tenant", Labels: map[string]string{"app.kubernetes.io/instance": "svc"}}}))
}

func TestLLMLoraAdapterCoverageControllerRegistration(t *testing.T) {
	s := buildLoraScheme(t)
	mgr, err := ctrl.NewManager(&rest.Config{Host: "https://127.0.0.1"}, ctrl.Options{Scheme: s, Metrics: metricsserver.Options{BindAddress: "0"}})
	require.NoError(t, err)
	name := fmt.Sprintf("llmloraadapter-test-%d", llmLoraControllerRegistrationID.Add(1))
	require.NoError(t, (&LLMLoraAdapterReconciler{}).setupWithManager(mgr, name))
}
