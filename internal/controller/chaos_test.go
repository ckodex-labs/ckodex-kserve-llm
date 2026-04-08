/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

// InterceptingClient wraps a client and injects errors for specific operations.
type InterceptingClient struct {
	client.Client
	FailAt types.NamespacedName
	Op     string
	Error  error
}

func (c *InterceptingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.Error != nil && obj.GetName() == c.FailAt.Name && obj.GetNamespace() == c.FailAt.Namespace && (c.Op == "Update" || c.Op == "*") {
		return c.Error
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *InterceptingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.Error != nil && obj.GetName() == c.FailAt.Name && obj.GetNamespace() == c.FailAt.Namespace && (c.Op == "Create" || c.Op == "*") {
		return c.Error
	}
	return c.Client.Create(ctx, obj, opts...)
}

func newTestReconciler(cl client.Client, s *runtime.Scheme) *LLMInferenceServiceReconciler {
	// Re-using the setup logic from the main controller test but with a custom client
	return setupReconciler(cl, s)
}

func TestChaos_ConflictRetry(t *testing.T) {
	scheme := buildLLMScheme(t)
	namespace := "chaos-ns"
	name := "atomic-llm"

	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "chaos-uid",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://test/model"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(svc).
		Build()

	// Inject a conflict error during Deployment creation
	interceptor := &InterceptingClient{
		Client: fakeClient,
		FailAt: types.NamespacedName{Name: name, Namespace: namespace},
		Op:     "Create",
		Error:  fmt.Errorf("conflict"), // simplified conflict
	}

	r := newTestReconciler(interceptor, scheme)
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}

	// First reconcile should fail due to injected error
	_, err := r.Reconcile(context.Background(), req)
	assert.Error(t, err)

	// Recover and ensure it eventually succeeds
	interceptor.Error = nil
	_, err = r.Reconcile(context.Background(), req)
	assert.NoError(t, err)

	// Verify all parts are there
	var deploy appsv1.Deployment
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, &deploy))
}

func TestChaos_AtomicFinalizer(t *testing.T) {
	scheme := buildLLMScheme(t)
	namespace := "chaos-ns"
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "finalizer-chaos",
			Namespace: namespace,
			UID:       "finalizer-uid",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://test/model"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc).
		WithStatusSubresource(svc).
		Build()
	
	// Inject failure during AddFinalizer Update
	interceptor := &InterceptingClient{
		Client: fakeClient,
		FailAt: types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace},
		Op:     "Update",
		Error:  fmt.Errorf("api server down"),
	}

	r := newTestReconciler(interceptor, scheme)
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}}
	_, err := r.Reconcile(context.Background(), req)
	
	assert.Error(t, err)
	
	// Recover
	interceptor.Error = nil
	_, err = r.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	
	var updatedSvc servingv1alpha2.LLMInferenceService
	require.NoError(t, r.Get(context.Background(), req.NamespacedName, &updatedSvc))
	assert.Contains(t, updatedSvc.Finalizers, api.FinalizerName)
}

func TestChaos_PodRestartResilience(t *testing.T) {
	// Future: Mock HTTPClient for LoRA registration resilience
}
