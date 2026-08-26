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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestLLMLoraAdapterCoverageReadyCacheAndRegistrationFailures(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("failure-lora", "default", "svc")
	cache := newLoraCache(lora)
	cache.Status.Conditions = []metav1.Condition{{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, cache).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	got, result, err := r.ensureLoraCache(context.Background(), lora)
	require.NoError(t, err)
	assert.Equal(t, cache.Name, got.Name)
	assert.Nil(t, result)

	badCache := cache.DeepCopy()
	badCache.Annotations[loraCacheOwnerUID] = "wrong"
	cl = fake.NewClientBuilder().WithScheme(s).WithObjects(lora, badCache).Build()
	r.Client = cl
	got, result, err = r.ensureLoraCache(context.Background(), lora)
	assert.Nil(t, got)
	assert.Nil(t, result)
	assert.Error(t, err)

	r.Client = fake.NewClientBuilder().WithScheme(s).WithObjects(lora).Build()
	statusLora := lora.DeepCopy()
	statusLora.Status.ActiveRevision = 0
	_, err = r.markLoraReady(context.Background(), statusLora)
	assert.Error(t, err)
}

func TestLLMLoraAdapterCoverageRegistrationWrapperAndFinalizeErrors(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("failure-lora", "default", "svc")
	svc := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "default", Labels: map[string]string{"app.kubernetes.io/instance": "svc"}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.20"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10), HTTPClient: &http.Client{Transport: &roundTripperMock{targetURL: srv.URL}}}
	result, err := r.registerAndMarkReady(context.Background(), lora, svc)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)

	lora.Finalizers = []string{loraFinalizer}
	original := lora.DeepCopy()
	badCache := newLoraCache(lora)
	badCache.Annotations[loraCacheOwnerUID] = "wrong"
	cl = fake.NewClientBuilder().WithScheme(s).WithObjects(lora, badCache).Build()
	r.Client = cl
	_, err = r.finalizeLora(context.Background(), lora, original)
	assert.Error(t, err)
}

func TestLLMLoraAdapterCoverageStringAndHTTPFailureBranches(t *testing.T) {
	assert.Equal(t, []string{"keep"}, removeString([]string{"keep", "remove"}, "remove"))
	assert.Equal(t, []string(nil), removeString([]string{"remove"}, "remove"))
	r := &LLMLoraAdapterReconciler{HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { return nil, assert.AnError })}}
	err := r.postWithRetry(context.Background(), "http://adapter", nil)
	assert.Error(t, err)
}

func TestLLMLoraAdapterCoveragePreparationAndGovernanceNoopBranches(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("missing", "default", "svc")
	r := &LLMLoraAdapterReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s, Recorder: record.NewFakeRecorder(10)}
	_, err := r.prepareLora(context.Background(), lora, lora.DeepCopy())
	assert.Error(t, err)

	active := testLora("active", "default", "svc")
	active.Status.StatePlanes.Lifecycle = "active"
	active.Status.StatePlanes.Trust = "verified"
	active.Status.EvidenceBundle.SignatureDigest = "sha256:sig"
	active.Status.EvidenceBundle.AttestationURI = "oci://attestation"
	active.Status.EvidenceBundle.SBOMDigest = "sha256:sbom"
	cache := newLoraCache(active)
	cache.Status.Conditions = []metav1.Condition{{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue}}
	r.Client = fake.NewClientBuilder().WithScheme(s).WithObjects(active, cache).WithStatusSubresource(active).Build()
	result, err := r.hydrateAndGovernLora(context.Background(), active, cache)
	require.NoError(t, err)
	assert.Nil(t, result)
}

type failingMutationClient struct{ client.Client }

func (failingMutationClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return apierrors.NewNotFound(schema.GroupResource{Group: "serving.ckodex.com", Resource: "localmodelcaches"}, "missing")
}

func (failingMutationClient) Create(context.Context, client.Object, ...client.CreateOption) error {
	return assert.AnError
}

func TestLLMLoraAdapterCoverageCacheCreateAndUnloadListErrors(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("errors", "default", "svc")
	r := &LLMLoraAdapterReconciler{Client: failingMutationClient{}, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	_, _, err := r.ensureLoraCache(context.Background(), lora)
	assert.Error(t, err)

	svc := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"}}
	r.Client = failingListClient{Client: fake.NewClientBuilder().WithScheme(s).WithObjects(svc).Build()}
	err = r.unloadFromTargetService(context.Background(), lora)
	assert.Error(t, err)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
