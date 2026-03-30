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
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func buildSessionScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, discoveryv1.AddToScheme(s))
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	return s
}

// ---- Unit Tests -------------------------------------------------------------

func TestIsExpired(t *testing.T) {
	r := &SessionReconciler{}
	makeDuration := func(d time.Duration) *metav1.Duration { return &metav1.Duration{Duration: d} }
	makeTime := func(t time.Time) *metav1.Time { mt := metav1.NewTime(t); return &mt }

	tests := []struct {
		name             string
		ttl              *metav1.Duration
		lastActivityTime *metav1.Time
		want             bool
	}{
		{"no TTL → never expires", nil, makeTime(time.Now().Add(-24 * time.Hour)), false},
		{"no last activity → never expires", makeDuration(5 * time.Minute), nil, false},
		{"recently active → not expired", makeDuration(30 * time.Minute), makeTime(time.Now().Add(-5 * time.Minute)), false},
		{"TTL elapsed → expired", makeDuration(10 * time.Minute), makeTime(time.Now().Add(-20 * time.Minute)), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session := &servingv1alpha2.InferenceSession{
				Spec:   servingv1alpha2.InferenceSessionSpec{TTL: tc.ttl},
				Status: servingv1alpha2.InferenceSessionStatus{LastActivityTime: tc.lastActivityTime},
			}
			assert.Equal(t, tc.want, r.isExpired(session))
		})
	}
}

// ---- Reconcile Tests --------------------------------------------------------

func TestSessionReconcile_NotFound(t *testing.T) {
	s := buildSessionScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &SessionReconciler{Client: cl, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "missing", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestActorReconcile_DeactivatesIdleActor(t *testing.T) {
	s := buildSessionScheme(t)
	idleTimeout := metav1.Duration{Duration: 1 * time.Minute}
	lastActivation := metav1.NewTime(time.Now().Add(-5 * time.Hour))

	actor := &servingv1alpha2.InferenceActor{
		ObjectMeta: metav1.ObjectMeta{Name: "idle-actor", Namespace: "default", UID: "uid"},
		Spec: servingv1alpha2.InferenceActorSpec{
			ActorType:   "chat",
			IdleTimeout: &idleTimeout,
		},
		Status: servingv1alpha2.InferenceActorStatus{
			State:              servingv1alpha2.ActorStateActive,
			ActiveSessions:     0,
			LastActivationTime: &lastActivation,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(actor).WithStatusSubresource(actor).Build()
	r := &ActorReconciler{Client: cl, Scheme: s}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "idle-actor", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.InferenceActor
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{Name: "idle-actor", Namespace: "default"}, &updated))
	assert.Equal(t, servingv1alpha2.ActorStateInactive, updated.Status.State)
}

func TestCoactorGroupReconcile_AllMembersReady(t *testing.T) {
	s := buildSessionScheme(t)
	actor := &servingv1alpha2.InferenceActor{
		ObjectMeta: metav1.ObjectMeta{Name: "actor-1", Namespace: "default"},
		Status:     servingv1alpha2.InferenceActorStatus{State: servingv1alpha2.ActorStateActive},
	}
	group := &servingv1alpha2.CoactorGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "my-group", Namespace: "default", UID: "group-uid"},
		Spec: servingv1alpha2.CoactorGroupSpec{
			Pattern: "sequential",
			Members: []servingv1alpha2.CoactorMember{{Name: "agent1", ActorRef: "actor-1", Role: "primary"}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(actor, group).WithStatusSubresource(group).Build()
	r := &CoactorGroupReconciler{Client: cl, Scheme: s}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-group", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.CoactorGroup
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{Name: "my-group", Namespace: "default"}, &updated))
	assert.Equal(t, servingv1alpha2.CoactorGroupReady, updated.Status.Phase)
}
