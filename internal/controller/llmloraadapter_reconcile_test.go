/* Copyright 2026 CKodex Authors. Licensed under the Apache License, Version 2.0. */
package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestLLMLoraAdapter_ReconcileNotFound(t *testing.T) {
	s := buildLoraScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "missing", Namespace: "default"}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestLLMLoraAdapter_CreatesLocalModelCache(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("my-lora", "default", "my-llm")
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-lora", Namespace: "default"}})
	require.NoError(t, err)
	assert.Greater(t, result.RequeueAfter, time.Duration(0))
	var list servingv1alpha2.LocalModelCacheList
	require.NoError(t, cl.List(context.Background(), &list))
	require.Len(t, list.Items, 1)
	cache := list.Items[0]
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
	lora := testLora("my-lora", "tenant-a", "missing")
	lora.Finalizers = []string{loraFinalizer}
	lora.DeletionTimestamp = &now
	cache := newLoraCache(lora)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, cache).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: lora.Name, Namespace: lora.Namespace}})
	require.NoError(t, err)
	var deleted servingv1alpha2.LocalModelCache
	err = cl.Get(context.Background(), client.ObjectKey{Name: cache.Name}, &deleted)
	assert.True(t, apierrors.IsNotFound(err), "cluster cache must be deleted before finalizer removal")
}

func TestLoraCacheNameSeparatesNamespaces(t *testing.T) {
	assert.NotEqual(t, loraCacheName("tenant-a", "adapter"), loraCacheName("tenant-b", "adapter"))
	assert.Len(t, loraCacheName("tenant-a", "adapter"), 25)
}

func TestLLMLoraAdapter_WaitsForCache(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("my-lora", "default", "my-llm")
	cache := newLoraCache(lora)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, cache).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-lora", Namespace: "default"}})
	require.NoError(t, err)
	assert.Greater(t, result.RequeueAfter, time.Duration(0))
}

func TestLLMLoraAdapter_TargetServiceNotReady(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("my-lora", "default", "my-llm")
	lora.Status.StatePlanes.Lifecycle = "active"
	lora.Status.EvidenceBundle = servingv1alpha2.EvidenceBundle{SignatureDigest: "sha256:dummy", AttestationURI: "https://dummy/attestation", SBOMDigest: "sha256:dummy-sbom"}
	cache := newLoraCache(lora)
	cache.Status.Conditions = []metav1.Condition{{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue, Reason: "Downloaded", LastTransitionTime: metav1.Now()}}
	target := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, cache, target).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-lora", Namespace: "default"}})
	require.NoError(t, err)
	assert.Greater(t, result.RequeueAfter, time.Duration(0))
}

func TestLLMLoraAdapter_TargetServiceMissing(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("my-lora", "default", "missing-llm")
	cache := newLoraCache(lora)
	cache.Status.Conditions = []metav1.Condition{{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue, Reason: "Downloaded", LastTransitionTime: metav1.Now()}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, cache).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-lora", Namespace: "default"}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}
