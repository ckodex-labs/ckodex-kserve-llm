/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// TestSessionReconcile_BoundEndpointGone_Rebinds clears the bound endpoint when
// the backing pod IP is no longer in the Endpoints resource.
func TestSessionReconcile_BoundEndpointGone_Rebinds(t *testing.T) {
	s := buildSessionScheme(t)

	// LLMInferenceService referenced by the session.
	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-model", Namespace: "default"},
	}

	// Endpoints for the model service — but the old bound IP (10.0.0.99) is gone.
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "my-model", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.1"}, // different IP
				},
			},
		},
	}

	session := &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bound-session",
			Namespace: "default",
			UID:       k8stypes.UID("sess-uid"),
		},
		Spec: servingv1alpha2.InferenceSessionSpec{
			ModelRef: "my-model",
		},
		Status: servingv1alpha2.InferenceSessionStatus{
			Phase:         servingv1alpha2.SessionPhaseActive,
			BoundEndpoint: "10.0.0.99:8000", // stale — not in endpoints
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(session, llmSvc, endpoints).
		WithStatusSubresource(session).
		Build()
	r := &SessionReconciler{Client: cl, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "bound-session", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Greater(t, int64(result.RequeueAfter), int64(0))

	var updated servingv1alpha2.InferenceSession
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "bound-session", Namespace: "default",
	}, &updated))

	// The stale endpoint must be cleared or replaced.
	assert.NotEqual(t, "10.0.0.99:8000", updated.Status.BoundEndpoint,
		"stale bound endpoint should have been replaced")
}

// TestSessionReconcile_BoundEndpointValid_KeepsBinding retains endpoint when still valid.
func TestSessionReconcile_BoundEndpointValid_KeepsBinding(t *testing.T) {
	s := buildSessionScheme(t)

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "my-model", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.5"}, // still alive
				},
			},
		},
	}

	ttl := &metav1.Duration{Duration: 60 * time.Second}
	session := &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "live-session",
			Namespace: "default",
			UID:       k8stypes.UID("sess-uid-2"),
		},
		Spec: servingv1alpha2.InferenceSessionSpec{
			ModelRef: "my-model",
			TTL:      ttl,
		},
		Status: servingv1alpha2.InferenceSessionStatus{
			Phase:         servingv1alpha2.SessionPhaseActive,
			BoundEndpoint: "10.0.0.5:8000", // still in endpoints
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(session, endpoints).
		WithStatusSubresource(session).
		Build()
	r := &SessionReconciler{Client: cl, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "live-session", Namespace: "default"},
	})
	require.NoError(t, err)
	// TTL/2 requeue expected.
	assert.Equal(t, ttl.Duration/2, result.RequeueAfter)

	var updated servingv1alpha2.InferenceSession
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "live-session", Namespace: "default",
	}, &updated))
	// Valid endpoint must be retained.
	assert.Equal(t, "10.0.0.5:8000", updated.Status.BoundEndpoint)
}

// TestSessionReconcile_EvictedSession_NoBind does not bind an evicted session.
func TestSessionReconcile_EvictedSession_NoBind(t *testing.T) {
	s := buildSessionScheme(t)

	session := &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "evicted-session",
			Namespace: "default",
			UID:       k8stypes.UID("sess-uid-3"),
		},
		Spec: servingv1alpha2.InferenceSessionSpec{
			ModelRef: "my-model",
		},
		Status: servingv1alpha2.InferenceSessionStatus{
			Phase:         servingv1alpha2.SessionPhaseEvicted,
			BoundEndpoint: "",
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(session).
		WithStatusSubresource(session).
		Build()
	r := &SessionReconciler{Client: cl, Scheme: s}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "evicted-session", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.InferenceSession
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "evicted-session", Namespace: "default",
	}, &updated))

	// Evicted session must not be rebound.
	assert.Equal(t, "", updated.Status.BoundEndpoint)
	assert.Equal(t, servingv1alpha2.SessionPhaseEvicted, updated.Status.Phase)
}

// TestSessionReconcile_UnboundSession_Binds binds a session with no prior endpoint.
func TestSessionReconcile_UnboundSession_Binds(t *testing.T) {
	s := buildSessionScheme(t)

	llmSvc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-model", Namespace: "default"},
	}

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "my-model", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.1"},
				},
			},
		},
	}

	session := &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unbound-session",
			Namespace: "default",
			UID:       k8stypes.UID("sess-uid-4"),
		},
		Spec: servingv1alpha2.InferenceSessionSpec{
			ModelRef: "my-model",
		},
		Status: servingv1alpha2.InferenceSessionStatus{
			Phase: servingv1alpha2.SessionPhaseActive,
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(session, llmSvc, endpoints).
		WithStatusSubresource(session).
		Build()
	r := &SessionReconciler{Client: cl, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "unbound-session", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Greater(t, int64(result.RequeueAfter), int64(0))

	var updated servingv1alpha2.InferenceSession
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "unbound-session", Namespace: "default",
	}, &updated))
	assert.NotEmpty(t, updated.Status.BoundEndpoint)
}
