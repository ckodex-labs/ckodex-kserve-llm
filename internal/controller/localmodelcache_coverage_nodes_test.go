package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestLocalModelCacheCoverageNodeSelectionMergesSources(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"accelerator": "gpu"}}
	lmc := &servingv1alpha2.LocalModelCache{Spec: servingv1alpha2.LocalModelCacheSpec{
		NodeGroup: &servingv1alpha2.NodeGroupSpec{LabelSelector: selector}, WarmNodes: []string{"gpu-1", "missing"},
	}}
	objects := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu-1", Labels: map[string]string{"accelerator": "gpu"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu-2", Labels: map[string]string{"accelerator": "gpu"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cpu-1"}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	r := &LocalModelCacheReconciler{Client: cl, APIReader: cl}
	nodes, err := r.resolveTargetNodes(context.Background(), lmc)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(nodes), 2; got != want {
		t.Fatalf("selected %d nodes, want %d: %v", got, want, nodes)
	}
	if nodes[0] != "gpu-1" || nodes[1] != "gpu-2" {
		t.Fatalf("nodes = %v", nodes)
	}
}

func TestLocalModelCacheCoverageWarmNodesSkipMissingAndDeduplicate(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	lmc := &servingv1alpha2.LocalModelCache{Spec: servingv1alpha2.LocalModelCacheSpec{WarmNodes: []string{"node-a", "node-a", "missing"}}}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &LocalModelCacheReconciler{Client: cl}
	nodes, err := r.resolveTargetNodes(context.Background(), lmc)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0] != "node-a" {
		t.Fatalf("nodes = %v", nodes)
	}
}

func TestLocalModelCacheCoverageNamespaceAccessAndStatusHelpers(t *testing.T) {
	lmc := makeLocalModelCache("cache")
	if !IsNamespaceAllowed(lmc, "any") {
		t.Fatal("empty allow list should allow")
	}
	lmc.Spec.AllowedNamespaces = []string{"team-a"}
	if !IsNamespaceAllowed(lmc, "team-a") || IsNamespaceAllowed(lmc, "team-b") {
		t.Fatal("namespace access mismatch")
	}
	lmc.Spec.AllowedNamespaces = []string{"*"}
	if !IsNamespaceAllowed(lmc, "team-b") {
		t.Fatal("wildcard should allow")
	}

	if got := availableCacheSpace(lmc, resource.MustParse("1Gi")); got != "" {
		t.Fatalf("unset max size = %q", got)
	}
	lmc.Spec.MaxCacheSize = "10Gi"
	if got := availableCacheSpace(lmc, resource.MustParse("4Gi")); got != "6Gi" {
		t.Fatalf("available = %q", got)
	}
	if got := availableCacheSpace(lmc, resource.MustParse("11Gi")); got != "0" {
		t.Fatalf("clamped available = %q", got)
	}
}

func TestLocalModelCacheCoverageMergeAndCachedModelStatus(t *testing.T) {
	old := metav1.Now()
	lmc := makeLocalModelCache("cache")
	previous := []servingv1alpha2.NodeCacheStatus{{NodeName: "stale", Phase: "Ready"}, {NodeName: "node-a", Phase: "Downloading"}}
	current := []servingv1alpha2.NodeCacheStatus{{NodeName: "node-a", Phase: "Ready", SizeBytes: 5, LastUsed: &old}, {NodeName: "node-b", Phase: "Pending"}}
	merged := mergeNodeCacheStatuses(previous, current, []string{"node-a", "node-b"}, context.Background())
	if len(merged) != 2 || merged[0].Phase != "Ready" {
		t.Fatalf("merged = %#v", merged)
	}
	models, total := (&LocalModelCacheReconciler{}).buildCachedModelsStatus(lmc, current)
	if len(models) != 1 || models[0].NodeNames[0] != "node-a" || total.Value() != 5 {
		t.Fatalf("models=%#v total=%s", models, total.String())
	}
	if cachedModelStatus(lmc, nil, 0, nil) != nil {
		t.Fatal("empty cached model status should be nil")
	}
}
