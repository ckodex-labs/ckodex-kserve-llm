package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestLocalModelCacheReconcileRejectsInvalidQuantitiesBeforeCreatingResources(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	cache := makeLocalModelCache("invalid-size")
	cache.Spec.ModelSize = "not-a-quantity"
	cache.Spec.WarmNodes = []string{"node-a"}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cache, node).WithStatusSubresource(cache).Build()
	r := &LocalModelCacheReconciler{Client: cl, APIReader: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(4)}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cache)})
	require.ErrorContains(t, err, "invalid modelSize")

	var pvcs corev1.PersistentVolumeClaimList
	require.NoError(t, cl.List(context.Background(), &pvcs))
	assert.Empty(t, pvcs.Items)
	var jobs batchv1.JobList
	require.NoError(t, cl.List(context.Background(), &jobs))
	assert.Empty(t, jobs.Items)
}

func TestLocalModelCacheReconcileDeletesResourcesBeforeDroppingStaleStatus(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	cache := makeLocalModelCache("cache")
	modelHash := ModelURIHash(cache.Spec.SourceModelURI)
	status := servingv1alpha2.NodeCacheStatus{NodeName: "node-old", PVCName: PVCNameForNode(modelHash, "node-old"), ModelURIHash: modelHash, Phase: "Ready"}
	cache.Status.NodeStatuses = []servingv1alpha2.NodeCacheStatus{status}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: status.PVCName, Namespace: defaultCacheNamespace}}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: warmupJobName(modelHash, status.NodeName), Namespace: defaultCacheNamespace}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cache, pvc, job).WithStatusSubresource(cache).Build()
	r := &LocalModelCacheReconciler{Client: cl, APIReader: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(4)}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cache)})
	require.NoError(t, err)
	require.True(t, apierrors.IsNotFound(cl.Get(context.Background(), client.ObjectKeyFromObject(pvc), &corev1.PersistentVolumeClaim{})))
	require.True(t, apierrors.IsNotFound(cl.Get(context.Background(), client.ObjectKeyFromObject(job), &batchv1.Job{})))

	var updated servingv1alpha2.LocalModelCache
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(cache), &updated))
	assert.Empty(t, updated.Status.NodeStatuses)
}

func TestLocalModelCacheReconcileRetainsStaleStatusWhenCleanupFails(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	cache := makeLocalModelCache("cache-cleanup-failure")
	modelHash := ModelURIHash(cache.Spec.SourceModelURI)
	status := servingv1alpha2.NodeCacheStatus{NodeName: "node-old", PVCName: PVCNameForNode(modelHash, "node-old"), ModelURIHash: modelHash, Phase: "Ready"}
	cache.Status.NodeStatuses = []servingv1alpha2.NodeCacheStatus{status}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: status.PVCName, Namespace: defaultCacheNamespace}}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cache, pvc).WithStatusSubresource(cache).Build()
	r := &LocalModelCacheReconciler{
		Client:    localModelCacheDeleteErrorClient{Client: base},
		APIReader: base,
		Scheme:    scheme,
		Recorder:  record.NewFakeRecorder(4),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cache)})
	require.ErrorContains(t, err, "cleaning stale node")
	var updated servingv1alpha2.LocalModelCache
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(cache), &updated))
	require.Len(t, updated.Status.NodeStatuses, 1)
	assert.Equal(t, status.NodeName, updated.Status.NodeStatuses[0].NodeName)
}
