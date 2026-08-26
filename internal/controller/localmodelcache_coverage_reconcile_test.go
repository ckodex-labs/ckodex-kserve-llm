package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

type localModelCacheErrorClient struct {
	client.Client
	err error
}

func (c localModelCacheErrorClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return c.err
}

func TestLocalModelCacheCoverageStatusAndErrorPaths(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	lmc := makeLocalModelCache("cache")
	r := &LocalModelCacheReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(lmc).WithStatusSubresource(lmc).Build(), Scheme: scheme}
	if _, err := r.loadLocalModelCache(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "absent", Namespace: "default"}}); err != nil {
		t.Fatal(err)
	}

	status := pendingNodeCacheStatus("node", "pvc", "hash")
	result := mergeNodeCacheStatuses(nil, []servingv1alpha2.NodeCacheStatus{status}, []string{"node"}, context.Background())
	if len(result) != 1 {
		t.Fatalf("merged new status = %#v", result)
	}
	if err := updateLocalModelCacheStatus(context.Background(), r, lmc, result, 0, nil, resource.MustParse("1Gi")); err != nil {
		t.Fatal(err)
	}
	if lmc.Status.TotalCacheSize != "1Gi" || lmc.Status.AvailableSpace != "" {
		t.Fatalf("status = %#v", lmc.Status)
	}

	errClient := &LocalModelCacheReconciler{Client: localModelCacheErrorClient{Client: r.Client, err: errors.New("reader failure")}, Scheme: scheme}
	if _, err := errClient.loadLocalModelCache(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "cache", Namespace: "default"}}); err == nil {
		t.Fatal("expected load error")
	}
}

func TestLocalModelCacheCoverageNodeFailureAndMap(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	lmc := makeLocalModelCache("cache")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(lmc).Build()
	recorder := record.NewFakeRecorder(5)
	r := &LocalModelCacheReconciler{Client: cl, Scheme: scheme, Recorder: recorder}
	r.reportNodeCacheFailure(context.Background(), lmc, "node-a", errors.New("boom"))
	requests := r.mapNodeToLMC(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}})
	if len(requests) != 1 || requests[0].Name != "cache" {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestLocalModelCacheCoverageReconcileTargetFailure(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	lmc := makeLocalModelCache("cache")
	recorder := record.NewFakeRecorder(5)
	r := &LocalModelCacheReconciler{Scheme: scheme, Recorder: recorder, APIReader: localModelCacheErrorReader{err: errors.New("cache lookup failed")}}
	statuses, ready := r.reconcileTargetCaches(context.Background(), lmc, []string{"node-a"}, "hash")
	if len(statuses) != 0 || ready != 0 {
		t.Fatalf("failure statuses=%#v ready=%d", statuses, ready)
	}
}

type localModelCacheErrorReader struct{ err error }

func (r localModelCacheErrorReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return r.err
}

func (r localModelCacheErrorReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return r.err
}
