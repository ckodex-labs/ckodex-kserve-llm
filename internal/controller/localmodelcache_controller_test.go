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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func buildLocalModelCacheScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, batchv1.AddToScheme(s))
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	return s
}

func makeLocalModelCache(name string) *servingv1alpha2.LocalModelCache {
	return &servingv1alpha2.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       k8stypes.UID("lmc-uid-" + name),
		},
		Spec: servingv1alpha2.LocalModelCacheSpec{
			SourceModelURI: "hf://org/test-model",
			ModelSize:      "10Gi",
		},
	}
}

func TestCacheWorkloadNamespace(t *testing.T) {
	lmc := makeLocalModelCache("my-cache")
	assert.Equal(t, defaultCacheNamespace, cacheWorkloadNamespace(lmc))

	lmc.Annotations = map[string]string{cacheWorkloadNamespaceAnnotation: "tenant-a"}
	assert.Equal(t, "tenant-a", cacheWorkloadNamespace(lmc))
}

// ---- Unit Tests (Hashing & Naming) -------------------------------------------

func TestModelURIHash_Deterministic(t *testing.T) {
	uri := "hf://meta-llama/Llama-3.2-8B-Instruct"
	h1 := ModelURIHash(uri)
	h2 := ModelURIHash(uri)
	assert.Equal(t, h1, h2)
	assert.Len(t, h1, 16)
}

func TestModelURIHash_DifferentURIs(t *testing.T) {
	h1 := ModelURIHash("hf://org/model-a")
	h2 := ModelURIHash("hf://org/model-b")
	assert.NotEqual(t, h1, h2)
}

func TestPVCNameForNode(t *testing.T) {
	hash := ModelURIHash("hf://test/model")
	name := PVCNameForNode(hash, "node-1")
	assert.NotEmpty(t, name)
	assert.Equal(t, "lmc-", name[:4])
	assert.Equal(t, name, PVCNameForNode(hash, "node-1"))
	assert.NotEqual(t, name, PVCNameForNode(hash, "node-2"))
	assert.LessOrEqual(t, len(name), 63)
}

// ---- Reconcile Tests --------------------------------------------------------

func TestLocalModelCacheReconcile_NotFound(t *testing.T) {
	s := buildLocalModelCacheScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &LocalModelCacheReconciler{
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

func TestLocalModelCacheReconcile_NoNodes(t *testing.T) {
	s := buildLocalModelCacheScheme(t)
	lmc := makeLocalModelCache("my-cache")
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lmc).WithStatusSubresource(lmc).Build()
	r := &LocalModelCacheReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-cache", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(30), int64(result.RequeueAfter.Seconds()))
}

func TestLocalModelCacheReconcile_WithWarmNode(t *testing.T) {
	s := buildLocalModelCacheScheme(t)
	lmc := makeLocalModelCache("my-cache")
	lmc.Spec.WarmNodes = []string{"node-1"}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lmc, node).WithStatusSubresource(lmc).Build()
	r := &LocalModelCacheReconciler{
		Client:    cl,
		Scheme:    s,
		Recorder:  record.NewFakeRecorder(10),
		APIReader: cl,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-cache", Namespace: "default"},
	})
	require.NoError(t, err)

	// Second pass: Job creation
	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-cache", Namespace: "default"},
	})
	require.NoError(t, err)

	// Verify PVC creation
	pvcList := &corev1.PersistentVolumeClaimList{}
	require.NoError(t, cl.List(context.Background(), pvcList))
	assert.Len(t, pvcList.Items, 1)

	// Verify Job creation
	jobList := &batchv1.JobList{}
	require.NoError(t, cl.List(context.Background(), jobList))
	assert.Len(t, jobList.Items, 1)
}

// ---- Eviction Tests ---------------------------------------------------------

func TestEvictLRU_OverBudget_EvictsOldest(t *testing.T) {
	s := buildLocalModelCacheScheme(t)
	pvc1 := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-node1", Namespace: "default"}}
	pvc2 := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-node2", Namespace: "default"}}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc1, pvc2).Build()
	r := &LocalModelCacheReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	lmc := makeLocalModelCache("test")
	lmc.Spec.MaxCacheSize = "15Gi"

	oldTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	newTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	bytesPerNode := int64(10 * 1024 * 1024 * 1024)

	nodeStatuses := []servingv1alpha2.NodeCacheStatus{
		{NodeName: "node-1", Phase: "Ready", PVCName: "pvc-node1", SizeBytes: bytesPerNode, LastUsed: &oldTime, ModelURIHash: "abc"},
		{NodeName: "node-2", Phase: "Ready", PVCName: "pvc-node2", SizeBytes: bytesPerNode, LastUsed: &newTime, ModelURIHash: "abc"},
	}

	err := r.evictLRU(context.Background(), lmc, nodeStatuses)
	require.NoError(t, err)
	assert.Equal(t, "Pending", nodeStatuses[0].Phase)
	assert.Equal(t, "Ready", nodeStatuses[1].Phase)
}

// ---- Internal Logic Tests ---------------------------------------------------

func TestResolveTargetNodes_UnschedulableNodes(t *testing.T) {
	s := buildLocalModelCacheScheme(t)
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-ok"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-drain"}, Spec: corev1.NodeSpec{Unschedulable: true}},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(&nodes[0], &nodes[1]).Build()
	r := &LocalModelCacheReconciler{
		Client:    cl,
		Scheme:    s,
		Recorder:  record.NewFakeRecorder(10),
		APIReader: cl,
	}

	lmc := &servingv1alpha2.LocalModelCache{Spec: servingv1alpha2.LocalModelCacheSpec{SourceModelURI: "hf://org/model"}}
	result, err := r.resolveTargetNodes(context.Background(), lmc)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "node-ok", result[0])
}

func TestBuildCachePVC(t *testing.T) {
	r := &LocalModelCacheReconciler{Recorder: record.NewFakeRecorder(10)}
	lmc := makeLocalModelCache("my-cache")
	pvc := r.buildCachePVC(lmc, "pvc-name", "default", "node-1", "abc123")

	assert.Equal(t, "pvc-name", pvc.Name)
	assert.Equal(t, "abc123", pvc.Labels[labelModelHash])
}

func TestBuildWarmupJob(t *testing.T) {
	r := &LocalModelCacheReconciler{Recorder: record.NewFakeRecorder(10)}
	lmc := makeLocalModelCache("my-cache")
	lmc.Spec.Storage = &servingv1alpha2.LocalModelStorageSpec{
		SecretName:         "storage-secret",
		ServiceAccountName: "cache-sa",
	}
	job := r.buildWarmupJob(lmc, "warmup-job", "pvc-name", "default", "node-1")

	assert.Equal(t, "warmup-job", job.Name)
	assert.Equal(t, "node-1", job.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"])
	assert.Equal(t, "cache-sa", job.Spec.Template.Spec.ServiceAccountName)
	require.Len(t, job.Spec.Template.Spec.Containers[0].EnvFrom, 1)
	assert.Equal(t, "storage-secret", job.Spec.Template.Spec.Containers[0].EnvFrom[0].SecretRef.Name)
}
