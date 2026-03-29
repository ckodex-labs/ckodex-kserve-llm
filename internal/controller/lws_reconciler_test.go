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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func buildLWSScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	// Register LWS types.
	s.AddKnownTypeWithName(schema.GroupVersionKind{
		Group: "leaderworkerset.x-k8s.io", Version: "v1", Kind: "LeaderWorkerSet",
	}, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(schema.GroupVersionKind{
		Group: "leaderworkerset.x-k8s.io", Version: "v1", Kind: "LeaderWorkerSetList",
	}, &unstructured.UnstructuredList{})
	return s
}

func makeLLMSvcWithParallelism(name string, tensorP, dataP int32) *servingv1alpha2.LLMInferenceService {
	tp := tensorP
	dp := dataP
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       k8stypes.UID("lws-uid-" + name),
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://org/big-model",
				Name: "big-model",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "vllm"}},
				},
			},
			Parallelism: &servingv1alpha2.ParallelismSpec{
				Tensor: &tp,
				Data:   &dp,
			},
		},
	}
}

func ptr32(i int32) *int32 { return &i }

// ---- Unit Tests -------------------------------------------------------------

func TestCalculateWorkers(t *testing.T) {
	r := &Reconciler{}
	tests := []struct {
		name string
		p    *servingv1alpha2.ParallelismSpec
		want int32
	}{
		{"nil parallelism", nil, 1},
		{"tensor=2", &servingv1alpha2.ParallelismSpec{Tensor: ptr32(2)}, 2},
		{"data=4", &servingv1alpha2.ParallelismSpec{Data: ptr32(4)}, 4},
		{"tensor=2 data=4", &servingv1alpha2.ParallelismSpec{Tensor: ptr32(2), Data: ptr32(4)}, 8},
		{"expert doubles requirement", &servingv1alpha2.ParallelismSpec{Tensor: ptr32(2), Expert: true}, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, r.calculateWorkers(tc.p))
		})
	}
}

func TestComputeGPUTopology(t *testing.T) {
	tests := []struct {
		name          string
		p             *servingv1alpha2.ParallelismSpec
		wantTotal     int32
		wantPerNode   int32
		wantNodes     int32
		wantPlacement string
	}{
		{"single GPU", &servingv1alpha2.ParallelismSpec{}, 1, 1, 1, "compact"},
		{"tensor=4 on 1 node", &servingv1alpha2.ParallelismSpec{Tensor: ptr32(4)}, 4, 4, 1, "compact"},
		{"tensor=4 data=2 → 8 on 2 nodes", &servingv1alpha2.ParallelismSpec{Tensor: ptr32(4), Data: ptr32(2)}, 8, 4, 2, "compact"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeGPUTopology(tc.p)
			assert.Equal(t, tc.wantTotal, got.TotalGPUs)
			assert.Equal(t, tc.wantPerNode, got.GPUsPerNode)
			assert.Equal(t, tc.wantNodes, got.NodesRequired)
			assert.Equal(t, tc.wantPlacement, got.Placement)
		})
	}
}

// ---- Reconcile Tests --------------------------------------------------------

func TestLWSReconciler_MultiWorker_CreatesLWS(t *testing.T) {
	s := buildLWSScheme(t)
	llmSvc := makeLLMSvcWithParallelism("my-llm", 2, 1)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := &Reconciler{Client: cl, Scheme: s}

	err := r.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	// Verify LWS exists
	lws := &unstructured.Unstructured{}
	lws.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "leaderworkerset.x-k8s.io", Version: "v1", Kind: "LeaderWorkerSet",
	})
	err = cl.Get(context.Background(), k8stypes.NamespacedName{Name: "my-llm-lws", Namespace: "default"}, lws)
	require.NoError(t, err)
}

func TestBuildLWS_ReturnsValidUnstructured(t *testing.T) {
	s := buildLWSScheme(t)
	r := &Reconciler{Scheme: s}
	llmSvc := makeLLMSvcWithParallelism("my-llm", 4, 1)
	obj := r.buildLWS(llmSvc, "my-llm-lws", 4)

	require.NotNil(t, obj)
	assert.Equal(t, "my-llm-lws", obj.GetName())
	assert.Equal(t, "LeaderWorkerSet", obj.GetKind())
}
