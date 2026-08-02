/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestInferencePoolManagerReconcilesGAContract(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(scheme))
	svc := eppSvc("llama", "inference")
	manager := &InferencePoolManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}
	require.NoError(t, manager.Reconcile(context.Background(), svc))

	pool := &unstructured.Unstructured{}
	pool.SetGroupVersionKind(inferencePoolGVK)
	require.NoError(t, manager.Get(context.Background(), types.NamespacedName{Name: "llama", Namespace: "inference"}, pool))
	port, found, err := unstructured.NestedInt64(pool.Object, "spec", "targetPorts", "0", "number")
	if err != nil || !found {
		ports, _, _ := unstructured.NestedSlice(pool.Object, "spec", "targetPorts")
		require.Len(t, ports, 1)
		port = ports[0].(map[string]interface{})["number"].(int64)
	}
	assert.Equal(t, int64(8000), port)
	failureMode, _, _ := unstructured.NestedString(pool.Object, "spec", "endpointPickerRef", "failureMode")
	assert.Equal(t, "FailClose", failureMode)
}
