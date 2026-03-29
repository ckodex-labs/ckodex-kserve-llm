/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func client_key(name, ns string) client.ObjectKey {
	return types.NamespacedName{Name: name, Namespace: ns}
}

func sessionScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	return s
}

func activeSession(name, ns, endpoint string) *servingv1alpha2.InferenceSession {
	return &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: servingv1alpha2.InferenceSessionStatus{
			Phase:         servingv1alpha2.SessionPhaseActive,
			BoundEndpoint: endpoint,
		},
	}
}

func idleSession(name, ns string) *servingv1alpha2.InferenceSession {
	return &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: servingv1alpha2.InferenceSessionStatus{
			Phase:         servingv1alpha2.SessionPhaseIdle,
			BoundEndpoint: "10.0.0.5:8080",
		},
	}
}

func activeSessionNoBound(name, ns string) *servingv1alpha2.InferenceSession {
	return &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: servingv1alpha2.InferenceSessionStatus{
			Phase:         servingv1alpha2.SessionPhaseActive,
			BoundEndpoint: "", // no endpoint bound
		},
	}
}

// ---- Route -----------------------------------------------------------------

func TestRoute_NoSessionID_EPPFallback(t *testing.T) {
	scheme := sessionScheme(t)
	r := &SessionRouter{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	decision, err := r.Route(context.Background(), "llama3", "", "default")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "no-session-affinity-epp-routing", decision.Reason)
	assert.False(t, decision.CacheHit)
}

func TestRoute_ActiveSession_CacheHit(t *testing.T) {
	scheme := sessionScheme(t)
	session := activeSession("sess-001", "default", "10.0.0.1:8080")

	r := &SessionRouter{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(session).
			WithStatusSubresource(session).
			Build(),
	}

	decision, err := r.Route(context.Background(), "llama3", "sess-001", "default")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "10.0.0.1:8080", decision.Endpoint)
	assert.Equal(t, "sess-001", decision.SessionID)
	assert.True(t, decision.CacheHit)
	assert.Equal(t, "session-affinity-kv-cache-hit", decision.Reason)
}

func TestRoute_SessionNotFound_EPPFallback(t *testing.T) {
	scheme := sessionScheme(t)
	r := &SessionRouter{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	decision, err := r.Route(context.Background(), "llama3", "nonexistent-sess", "default")
	require.NoError(t, err)
	require.NotNil(t, decision)
	// Not found → fall through to EPP
	assert.Equal(t, "no-session-affinity-epp-routing", decision.Reason)
	assert.False(t, decision.CacheHit)
}

func TestRoute_IdleSession_EPPFallback(t *testing.T) {
	scheme := sessionScheme(t)
	session := idleSession("sess-idle", "default")

	r := &SessionRouter{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(session).
			WithStatusSubresource(session).
			Build(),
	}

	// Idle session — should not route via session affinity
	decision, err := r.Route(context.Background(), "llama3", "sess-idle", "default")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "no-session-affinity-epp-routing", decision.Reason)
	assert.False(t, decision.CacheHit)
}

func TestRoute_ActiveSessionNoBoundEndpoint_EPPFallback(t *testing.T) {
	scheme := sessionScheme(t)
	session := activeSessionNoBound("sess-unbound", "default")

	r := &SessionRouter{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(session).
			WithStatusSubresource(session).
			Build(),
	}

	decision, err := r.Route(context.Background(), "llama3", "sess-unbound", "default")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "no-session-affinity-epp-routing", decision.Reason)
}

// ---- routeBySession (white-box) -------------------------------------------

func TestRouteBySession_Active_ReturnsDecision(t *testing.T) {
	scheme := sessionScheme(t)
	session := activeSession("s1", "ns1", "192.168.1.10:8080")

	r := &SessionRouter{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(session).
			WithStatusSubresource(session).
			Build(),
	}

	decision, err := r.routeBySession(context.Background(), "s1", "ns1")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "192.168.1.10:8080", decision.Endpoint)
	assert.True(t, decision.CacheHit)
}

func TestRouteBySession_NotFound_Error(t *testing.T) {
	scheme := sessionScheme(t)
	r := &SessionRouter{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	decision, err := r.routeBySession(context.Background(), "missing", "ns1")
	require.Error(t, err)
	assert.Nil(t, decision)
}

func TestRouteBySession_IdlePhase_NilDecision(t *testing.T) {
	scheme := sessionScheme(t)
	session := idleSession("s2", "ns1")

	r := &SessionRouter{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(session).
			WithStatusSubresource(session).
			Build(),
	}

	decision, err := r.routeBySession(context.Background(), "s2", "ns1")
	require.NoError(t, err)
	assert.Nil(t, decision)
}

func TestRouteBySession_Draining_NilDecision(t *testing.T) {
	scheme := sessionScheme(t)
	sess := &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{Name: "s3", Namespace: "ns1"},
		Status: servingv1alpha2.InferenceSessionStatus{
			Phase:         servingv1alpha2.SessionPhaseDraining,
			BoundEndpoint: "10.0.0.9:8080",
		},
	}

	r := &SessionRouter{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sess).
			WithStatusSubresource(sess).
			Build(),
	}

	decision, err := r.routeBySession(context.Background(), "s3", "ns1")
	require.NoError(t, err)
	assert.Nil(t, decision)
}

// ---- RecordActivity --------------------------------------------------------

func TestRecordActivity_UpdatesTurnAndTokenCount(t *testing.T) {
	scheme := sessionScheme(t)
	session := activeSession("sess-rec", "default", "10.0.0.2:8080")
	session.Status.TurnCount = 5
	session.Status.TokenCount = 100

	r := &SessionRouter{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(session).
			WithStatusSubresource(session).
			Build(),
	}

	require.NoError(t, r.RecordActivity(context.Background(), "sess-rec", "default", 50))

	var updated servingv1alpha2.InferenceSession
	require.NoError(t, r.Get(context.Background(),
		client_key("sess-rec", "default"), &updated))

	assert.Equal(t, int32(6), updated.Status.TurnCount)
	assert.Equal(t, int64(150), updated.Status.TokenCount)
	assert.Equal(t, servingv1alpha2.SessionPhaseActive, updated.Status.Phase)
	assert.NotNil(t, updated.Status.LastActivityTime)
}

func TestRecordActivity_SessionNotFound_Error(t *testing.T) {
	scheme := sessionScheme(t)
	r := &SessionRouter{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	err := r.RecordActivity(context.Background(), "missing", "default", 10)
	require.Error(t, err)
}

func TestRecordActivity_ZeroTokens_TurnCountStillIncremented(t *testing.T) {
	scheme := sessionScheme(t)
	session := activeSession("sess-z", "default", "10.0.0.3:8080")

	r := &SessionRouter{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(session).
			WithStatusSubresource(session).
			Build(),
	}

	require.NoError(t, r.RecordActivity(context.Background(), "sess-z", "default", 0))

	var updated servingv1alpha2.InferenceSession
	require.NoError(t, r.Get(context.Background(),
		client_key("sess-z", "default"), &updated))
	assert.Equal(t, int32(1), updated.Status.TurnCount)
	assert.Equal(t, int64(0), updated.Status.TokenCount)
}
