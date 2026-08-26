package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestLocalModelCacheCoverageEvictionBudgetAndOrdering(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-a", Namespace: "default"}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: warmupJobName("hash", "node-a"), Namespace: "default"}},
	).Build()
	lmc := makeLocalModelCache("cache")
	r := &LocalModelCacheReconciler{Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(5)}
	statuses := []servingv1alpha2.NodeCacheStatus{
		{NodeName: "node-a", Phase: "Ready", PVCName: "pvc-a", ModelURIHash: "hash", SizeBytes: 2, LastUsed: ptrTime(time.Now().Add(-time.Hour))},
		{NodeName: "node-b", Phase: "Pending", SizeBytes: 100},
	}
	if err := r.evictLRU(context.Background(), lmc, statuses); err != nil {
		t.Fatal(err)
	}
	if statuses[0].Phase != "Ready" {
		t.Fatalf("unset budget evicted cache: %#v", statuses)
	}

	lmc.Spec.MaxCacheSize = "1"
	if err := r.evictLRU(context.Background(), lmc, statuses); err != nil {
		t.Fatal(err)
	}
	if statuses[0].Phase != "Pending" {
		t.Fatalf("over-budget cache not evicted: %#v", statuses)
	}
	if got := cacheEntries([]servingv1alpha2.NodeCacheStatus{{Phase: "Pending"}}); len(got) != 0 {
		t.Fatalf("pending entry = %#v", got)
	}
	if got := cacheEntriesSize(nil); got.Cmp(resource.MustParse("0")) != 0 {
		t.Fatalf("empty size = %s", got.String())
	}
}

func TestLocalModelCacheCoverageDeleteMissingResourcesAndCachedModel(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	r := &LocalModelCacheReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme, Recorder: record.NewFakeRecorder(5)}
	if err := r.deleteCachePVC(context.Background(), "default", "missing"); err != nil {
		t.Fatal(err)
	}
	if err := r.deleteCacheJob(context.Background(), "default", "missing"); err != nil {
		t.Fatal(err)
	}
	lmc := makeLocalModelCache("cache")
	used := metav1.Now()
	model := cachedModelStatus(lmc, []string{"node-a", "node-b"}, 12, &used)
	if model == nil || model[0].PVCName == "" || model[0].LastUsed == nil {
		t.Fatalf("cached model = %#v", model)
	}
}

func TestLocalModelCacheCoverageEvictionReportsDeleteFailure(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Namespace: "default"}}).Build()
	recorder := record.NewFakeRecorder(5)
	r := &LocalModelCacheReconciler{Client: localModelCacheDeleteErrorClient{Client: base}, Scheme: scheme, Recorder: recorder}
	lmc := makeLocalModelCache("cache")
	lmc.Spec.MaxCacheSize = "1"
	status := servingv1alpha2.NodeCacheStatus{NodeName: "node", Phase: "Ready", PVCName: "pvc", ModelURIHash: "hash", SizeBytes: 2}
	r.evictAndReport(context.Background(), lmc, []servingv1alpha2.NodeCacheStatus{status})
	select {
	case event := <-recorder.Events:
		if event == "" {
			t.Fatal("empty eviction failure event")
		}
	default:
		t.Fatal("eviction failure was not reported")
	}
}

type localModelCacheDeleteErrorClient struct{ client.Client }

func (localModelCacheDeleteErrorClient) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	return fmt.Errorf("delete denied")
}

func ptrTime(value time.Time) *metav1.Time { result := metav1.NewTime(value); return &result }
