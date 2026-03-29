/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func buildTenantScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

func makeTenantNamespace(name string, labels map[string]string, annotations map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

// TestDefaultTenantQuota verifies the default values are sane.
func TestDefaultTenantQuota(t *testing.T) {
	q := DefaultTenantQuota()
	assert.Equal(t, int64(5), q.MaxLLMInferenceServices)
	assert.Equal(t, int64(8), q.MaxGPUs)
	assert.Equal(t, "64", q.MaxCPU)
	assert.Equal(t, "256Gi", q.MaxMemory)
}

// TestTenantQuotaReconcile_NonTenantNamespace should return early without creating quota.
func TestTenantQuotaReconcile_NonTenantNamespace(t *testing.T) {
	s := buildTenantScheme(t)
	ns := makeTenantNamespace("no-label-ns", nil, nil)

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(ns).Build()
	r := &TenantQuotaReconciler{
		Client:   cl,
		Scheme:   s,
		Defaults: DefaultTenantQuota(),
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "no-label-ns"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify no ResourceQuota was created.
	var rqList corev1.ResourceQuotaList
	require.NoError(t, cl.List(context.Background(), &rqList))
	assert.Empty(t, rqList.Items)
}

// TestTenantQuotaReconcile_CreatesQuotaAndLimitRange checks that reconciling a tenant
// namespace creates both a ResourceQuota and a LimitRange.
func TestTenantQuotaReconcile_CreatesQuotaAndLimitRange(t *testing.T) {
	s := buildTenantScheme(t)
	ns := makeTenantNamespace("tenant-ns", map[string]string{
		LabelTenantID: "tenant-abc",
	}, nil)

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(ns).Build()
	r := &TenantQuotaReconciler{
		Client:   cl,
		Scheme:   s,
		Defaults: DefaultTenantQuota(),
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "tenant-ns"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// ResourceQuota must exist.
	var rq corev1.ResourceQuota
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: quotaName, Namespace: "tenant-ns",
	}, &rq))
	assert.Equal(t, "tenant-abc", rq.Labels[LabelTenantID])

	// LimitRange must exist.
	var lr corev1.LimitRange
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: limitRangeName, Namespace: "tenant-ns",
	}, &lr))
}

// TestTenantQuotaReconcile_NamespaceNotFound returns no error (not-found scenario).
func TestTenantQuotaReconcile_NamespaceNotFound(t *testing.T) {
	s := buildTenantScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &TenantQuotaReconciler{Client: cl, Scheme: s, Defaults: DefaultTenantQuota()}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "missing-ns"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestTenantQuotaReconcile_UpdatesExistingQuota verifies idempotency and update path.
func TestTenantQuotaReconcile_UpdatesExistingQuota(t *testing.T) {
	s := buildTenantScheme(t)
	ns := makeTenantNamespace("tenant-ns", map[string]string{
		LabelTenantID: "tenant-xyz",
	}, nil)

	// Pre-create a ResourceQuota with different spec to force update.
	existingRQ := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaName,
			Namespace: "tenant-ns",
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"),
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(ns, existingRQ).Build()
	r := &TenantQuotaReconciler{
		Client:   cl,
		Scheme:   s,
		Defaults: DefaultTenantQuota(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "tenant-ns"},
	})
	require.NoError(t, err)

	// ResourceQuota should have been updated with new CPU value.
	var rq corev1.ResourceQuota
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: quotaName, Namespace: "tenant-ns",
	}, &rq))
	assert.Equal(t, resource.MustParse("64"), rq.Spec.Hard[corev1.ResourceCPU])
}

// TestTenantQuotaReconcile_EmptyTenantID returns early if label value is empty.
func TestTenantQuotaReconcile_EmptyTenantID(t *testing.T) {
	s := buildTenantScheme(t)
	ns := makeTenantNamespace("tenant-ns", map[string]string{
		LabelTenantID: "", // empty value
	}, nil)

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(ns).Build()
	r := &TenantQuotaReconciler{Client: cl, Scheme: s, Defaults: DefaultTenantQuota()}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "tenant-ns"},
	})
	require.NoError(t, err)

	// No quota should be created.
	var rqList corev1.ResourceQuotaList
	require.NoError(t, cl.List(context.Background(), &rqList))
	assert.Empty(t, rqList.Items)
}

// TestReconcileResourceQuota_IdempotentWhenUnchanged exercises the no-update path.
func TestReconcileResourceQuota_IdempotentWhenUnchanged(t *testing.T) {
	s := buildTenantScheme(t)

	defaults := DefaultTenantQuota()
	existingRQ := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaName,
			Namespace: "tenant-ns",
			Labels: map[string]string{
				LabelTenantID:                  "tenant-abc",
				"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceName("requests.nvidia.com/gpu"): resource.MustParse("8"),
				corev1.ResourceCPU:    resource.MustParse("64"),
				corev1.ResourceMemory: resource.MustParse("256Gi"),
				corev1.ResourceName("count/llminferenceservices.serving.ckodex.com"): resource.MustParse("5"),
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(existingRQ).Build()
	r := &TenantQuotaReconciler{Client: cl, Scheme: s, Defaults: defaults}

	err := r.reconcileResourceQuota(context.Background(), "tenant-ns", "tenant-abc")
	require.NoError(t, err)
}

// TestReconcileLimitRange_UpdatesExisting exercises the LimitRange update path.
func TestReconcileLimitRange_UpdatesExisting(t *testing.T) {
	s := buildTenantScheme(t)

	existingLR := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      limitRangeName,
			Namespace: "tenant-ns",
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"), // different from desired
					},
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(existingLR).Build()
	r := &TenantQuotaReconciler{Client: cl, Scheme: s, Defaults: DefaultTenantQuota()}

	err := r.reconcileLimitRange(context.Background(), "tenant-ns")
	require.NoError(t, err)

	// LimitRange should have the new defaults.
	var lr corev1.LimitRange
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: limitRangeName, Namespace: "tenant-ns",
	}, &lr))
	assert.Equal(t, resource.MustParse("2"), lr.Spec.Limits[0].Default[corev1.ResourceCPU])
}
