package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestSessionReconcileClearsStaleBindingWhenNoReplacementIsReady(t *testing.T) {
	scheme := buildSessionScheme(t)
	service := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "tenant"}}
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta:  metav1.ObjectMeta{Name: "model-1", Namespace: "tenant", Labels: map[string]string{"kubernetes.io/service-name": "model"}},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints:   []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.2"}, Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(false)}}},
	}
	session := &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{Name: "session", Namespace: "tenant"},
		Spec:       servingv1alpha2.InferenceSessionSpec{ModelRef: "model"},
		Status:     servingv1alpha2.InferenceSessionStatus{Phase: servingv1alpha2.SessionPhaseActive, BoundEndpoint: "10.0.0.1:8000"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(service, slice, session).WithStatusSubresource(session).Build()
	r := &SessionReconciler{Client: cl, Scheme: scheme}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(session)})
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)
	var updated servingv1alpha2.InferenceSession
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(session), &updated))
	assert.Empty(t, updated.Status.BoundEndpoint)
}

func TestSessionReconcileCompletesAndReleasesResidency(t *testing.T) {
	scheme := buildSessionScheme(t)
	session := &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{Name: "session", Namespace: "tenant"},
		Spec:       servingv1alpha2.InferenceSessionSpec{ModelRef: "model", MaxTurns: 2},
		Status:     servingv1alpha2.InferenceSessionStatus{Phase: servingv1alpha2.SessionPhaseActive, TurnCount: 2, BoundEndpoint: "10.0.0.1:8000", KVCacheSize: 4096},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(session).WithStatusSubresource(session).Build()
	r := &SessionReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(session)})
	require.NoError(t, err)
	var updated servingv1alpha2.InferenceSession
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(session), &updated))
	assert.Equal(t, servingv1alpha2.SessionPhaseCompleted, updated.Status.Phase)
	assert.Empty(t, updated.Status.BoundEndpoint)
	assert.Zero(t, updated.Status.KVCacheSize)
}
