/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
)

func buildLoraScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, discoveryv1.AddToScheme(s))
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	return s
}

// TestLLMLoraAdapter_ReconcileNotFound returns no error.
func TestLLMLoraAdapter_ReconcileNotFound(t *testing.T) {
	s := buildLoraScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "missing", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestLLMLoraAdapter_CreatesLocalModelCache on first reconcile.
func TestLLMLoraAdapter_CreatesLocalModelCache(t *testing.T) {
	s := buildLoraScheme(t)
	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-lora",
			Namespace: "default",
			UID:       k8stypes.UID("lora-uid"),
		},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			TargetService: "my-llm",
			AdapterName:   "sql-helper",
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://org/lora-weights",
				Name: "sql-helper",
			},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(lora).
		WithStatusSubresource(lora).
		Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-lora", Namespace: "default"},
	})
	require.NoError(t, err)
	// Should requeue to check cache readiness.
	assert.Greater(t, result.RequeueAfter, time.Duration(0))

	var lmcList servingv1alpha2.LocalModelCacheList
	require.NoError(t, cl.List(context.Background(), &lmcList))
	require.Len(t, lmcList.Items, 1)
	cache := lmcList.Items[0]
	assert.Equal(t, loraCacheName("default", "my-lora"), cache.Name)
	assert.Empty(t, cache.Namespace)
	assert.Empty(t, cache.OwnerReferences)
	assert.Equal(t, "default", cache.Annotations[loraCacheOwnerNamespace])
	assert.Equal(t, "my-lora", cache.Annotations[loraCacheOwnerName])
	assert.Equal(t, "lora-uid", cache.Annotations[loraCacheOwnerUID])
	assert.Equal(t, "default", cache.Annotations[cacheWorkloadNamespaceAnnotation])
}

func TestLLMLoraAdapter_DeletionRemovesClusterCache(t *testing.T) {
	s := buildLoraScheme(t)
	now := metav1.Now()
	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-lora",
			Namespace:         "tenant-a",
			UID:               k8stypes.UID("lora-uid"),
			Finalizers:        []string{loraFinalizer},
			DeletionTimestamp: &now,
		},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			TargetService: "missing",
			Model:         servingv1alpha2.ModelSpec{URI: "hf://org/lora-weights"},
		},
	}
	cache := newLoraCache(lora)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, cache).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: lora.Name, Namespace: lora.Namespace},
	})
	require.NoError(t, err)

	var deletedCache servingv1alpha2.LocalModelCache
	err = cl.Get(context.Background(), client.ObjectKey{Name: cache.Name}, &deletedCache)
	assert.True(t, apierrors.IsNotFound(err), "cluster cache must be deleted before finalizer removal")
}

func TestLoraCacheNameSeparatesNamespaces(t *testing.T) {
	assert.NotEqual(t, loraCacheName("tenant-a", "adapter"), loraCacheName("tenant-b", "adapter"))
	assert.Len(t, loraCacheName("tenant-a", "adapter"), 25)
}

// TestLLMLoraAdapter_WaitsForCache when cache is not yet ready.
func TestLLMLoraAdapter_WaitsForCache(t *testing.T) {
	s := buildLoraScheme(t)
	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-lora",
			Namespace: "default",
			UID:       k8stypes.UID("lora-uid"),
		},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			TargetService: "my-llm",
			AdapterName:   "sql-helper",
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://org/lora-weights",
				Name: "sql-helper",
			},
		},
	}

	// Pre-create cache that's not ready (no conditions).
	cache := newLoraCache(lora)

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(lora, cache).
		WithStatusSubresource(lora).
		Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-lora", Namespace: "default"},
	})
	require.NoError(t, err)
	// Should requeue after delay waiting for cache to download.
	assert.Greater(t, result.RequeueAfter, time.Duration(0))
}

// TestLLMLoraAdapter_TargetServiceNotReady waits when target service not ready.
func TestLLMLoraAdapter_TargetServiceNotReady(t *testing.T) {
	s := buildLoraScheme(t)
	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-lora",
			Namespace: "default",
			UID:       k8stypes.UID("lora-uid"),
		},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			TargetService: "my-llm",
			AdapterName:   "sql-helper",
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://org/lora-weights",
				Name: "sql-helper",
			},
		},
		Status: servingv1alpha2.LLMLoraAdapterStatus{
			StatePlanes: servingv1alpha2.StatePlanes{
				Lifecycle: "active",
			},
			EvidenceBundle: servingv1alpha2.EvidenceBundle{
				SignatureDigest: "sha256:dummy",
				AttestationURI:  "https://dummy/attestation",
				SBOMDigest:      "sha256:dummy-sbom",
			},
		},
	}

	// Cache is ready.
	cache := newLoraCache(lora)
	cache.Status = servingv1alpha2.LocalModelCacheStatus{
		Conditions: []metav1.Condition{
			{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue, Reason: "Downloaded", LastTransitionTime: metav1.Now()},
		},
	}

	// Target service exists but not ready.
	targetSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
		Status:     servingv1alpha2.LLMInferenceServiceStatus{ModelReady: false},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(lora, cache, targetSvc).
		WithStatusSubresource(lora).
		Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-lora", Namespace: "default"},
	})
	require.NoError(t, err)
	// Should requeue waiting for target to become ready.
	assert.Greater(t, result.RequeueAfter, time.Duration(0))
}

func TestLLMLoraAdapter_HydratesVerifiedEvidenceFromWarmupPod(t *testing.T) {
	s := buildLoraScheme(t)
	modelURI := "oci://registry.example.com/lora@sha256:abc"
	modelHash := ModelURIHash(modelURI)
	nodeHash := fmt.Sprintf("%x", sha256.Sum256([]byte("node-a")))[:8]
	recordPayload, err := json.Marshal(provenance.RuntimeVerificationRecord{
		Subject:             modelURI,
		Scheme:              "oci",
		SignatureVerified:   true,
		AttestationVerified: true,
		SBOMVerified:        true,
		SignatureDigest:     "sha256:abc",
		AttestationURI:      modelURI + "#attestation:slsaprovenance1",
		SBOMDigest:          "sha256:def",
		VerifiedAt:          "2026-05-11T12:00:00Z",
	})
	require.NoError(t, err)

	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "verified-lora",
			Namespace: "default",
			UID:       k8stypes.UID("lora-uid"),
		},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			TargetService: "my-llm",
			AdapterName:   "verified",
			Model: servingv1alpha2.ModelSpec{
				URI:  modelURI,
				Name: "verified",
			},
		},
		Status: servingv1alpha2.LLMLoraAdapterStatus{
			StatePlanes: servingv1alpha2.StatePlanes{
				Lifecycle: "active",
			},
		},
	}

	cache := newLoraCache(lora)
	cache.Status = servingv1alpha2.LocalModelCacheStatus{
		Conditions: []metav1.Condition{
			{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue, Reason: "Downloaded", LastTransitionTime: metav1.Now()},
		},
		NodeStatuses: []servingv1alpha2.NodeCacheStatus{
			{
				NodeName:     "node-a",
				Phase:        "Ready",
				ModelURIHash: modelHash,
			},
		},
	}

	jobPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "warmup-pod",
			Namespace: "default",
			Labels: map[string]string{
				"job-name": "lmc-warmup-" + modelHash + "-" + nodeHash,
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "warmup",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 0,
							Message:  string(recordPayload),
						},
					},
				},
			},
		},
	}

	targetSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
		Status:     servingv1alpha2.LLMInferenceServiceStatus{ModelReady: false},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(lora, cache, targetSvc, jobPod).
		WithStatusSubresource(lora).
		Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "verified-lora", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.LLMLoraAdapter
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{Name: "verified-lora", Namespace: "default"}, &updated))
	assert.Equal(t, "sha256:abc", updated.Status.EvidenceBundle.SignatureDigest)
	assert.Equal(t, "sha256:def", updated.Status.EvidenceBundle.SBOMDigest)
	assert.Equal(t, "verified", updated.Status.StatePlanes.Trust)
	require.NotNil(t, updated.Status.EvidenceBundle.LastVerifiedAt)
}

// TestLLMLoraAdapter_TargetServiceMissing waits when target not found.
func TestLLMLoraAdapter_TargetServiceMissing(t *testing.T) {
	s := buildLoraScheme(t)
	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-lora",
			Namespace: "default",
			UID:       k8stypes.UID("lora-uid"),
		},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			TargetService: "missing-llm",
			AdapterName:   "sql-helper",
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://org/lora-weights",
				Name: "sql-helper",
			},
		},
	}

	// Cache is ready.
	cache := newLoraCache(lora)
	cache.Status = servingv1alpha2.LocalModelCacheStatus{
		Conditions: []metav1.Condition{
			{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue, Reason: "Downloaded", LastTransitionTime: metav1.Now()},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(lora, cache).
		WithStatusSubresource(lora).
		Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-lora", Namespace: "default"},
	})
	require.NoError(t, err)
	// No error — returns empty result (wait for target to appear).
	assert.Equal(t, ctrl.Result{}, result)
}

// TestRegisterWithTargetService_Success verifies that POST requests are sent to pod IPs.
func TestRegisterWithTargetService_Success(t *testing.T) {
	s := buildLoraScheme(t)

	// Mock server handles both the synchronous load and asynchronous warmup.
	loadRequests := make(chan VLLMLoadLoraRequest, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/load_lora_adapter":
			var body VLLMLoadLoraRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			loadRequests <- body
			w.WriteHeader(http.StatusOK)
		case "/v1/completions":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{Name: "my-lora", Namespace: "default"},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			AdapterName:   "sql-helper",
			TargetService: "my-llm",
		},
	}
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "llm-pod",
			Namespace: "default",
			Labels:    map[string]string{"app.kubernetes.io/instance": "my-llm"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "1.2.3.4", // dummy IP
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()

	// Custom mock client to redirect all traffic to our mock server.
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
		HTTPClient: &http.Client{
			Transport: &roundTripperMock{targetURL: srv.URL},
		},
	}

	err := r.registerWithTargetService(context.Background(), lora, svc)
	require.NoError(t, err)

	select {
	case body := <-loadRequests:
		assert.Equal(t, "sql-helper", body.LoraName)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for vLLM adapter-load request")
	}
	assert.Empty(t, loadRequests, "vLLM adapter-load API should be called once")
}

// TestRegisterWithTargetService_NoPods verifies error when no pods match.
func TestRegisterWithTargetService_NoPods(t *testing.T) {
	s := buildLoraScheme(t)
	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{Name: "my-lora", Namespace: "default"},
		Spec:       servingv1alpha2.LLMLoraAdapterSpec{TargetService: "my-llm"},
	}
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
	}

	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	err := r.registerWithTargetService(context.Background(), lora, svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pods found")
}

// TestRegisterWithTargetService_HTTPError verifies error handling for failed POST.
func TestRegisterWithTargetService_HTTPError(t *testing.T) {
	s := buildLoraScheme(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{Name: "my-lora", Namespace: "default"},
		Spec:       servingv1alpha2.LLMLoraAdapterSpec{TargetService: "my-llm"},
	}
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "llm-pod",
			Namespace: "default",
			Labels:    map[string]string{"app.kubernetes.io/instance": "my-llm"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "1.2.3.4"},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
	r := &LLMLoraAdapterReconciler{
		Client:     cl,
		Scheme:     s,
		Recorder:   record.NewFakeRecorder(10),
		HTTPClient: &http.Client{Transport: &roundTripperMock{targetURL: srv.URL}},
	}

	err := r.registerWithTargetService(context.Background(), lora, svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vLLM returned non-OK status 500")
}

type roundTripperMock struct {
	targetURL string
}

func (m *roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid side effects.
	newReq := req.Clone(req.Context())

	// Parse the target URL (httptest server).
	target, _ := url.Parse(m.targetURL)

	// Override seulement Host et Scheme, garder le Path original du controller.
	newReq.URL.Scheme = target.Scheme
	newReq.URL.Host = target.Host

	return http.DefaultTransport.RoundTrip(newReq)
}
