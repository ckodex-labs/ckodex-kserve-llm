/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ---- scheme helpers --------------------------------------------------------

func TestReconcileEbpfPolicy_CreatesSecurityPolicy(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	r := &EbpfReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))

	var tp unstructured.Unstructured
	tp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "isovalent.com",
		Version: "v1alpha1",
		Kind:    "TracingPolicy",
	})
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "llama3-security-policy", Namespace: "default"}, &tp))
}

func TestReconcileEbpfPolicy_CreatesNetworkPolicy(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	r := &EbpfReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))

	var tp unstructured.Unstructured
	tp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "isovalent.com",
		Version: "v1alpha1",
		Kind:    "TracingPolicy",
	})
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "llama3-network-policy", Namespace: "default"}, &tp))
}

func TestReconcileEbpfPolicy_SecurityPolicyKprobeIsSysExecve(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("phi3", "default")

	r := &EbpfReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))

	var tp unstructured.Unstructured
	tp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "isovalent.com",
		Version: "v1alpha1",
		Kind:    "TracingPolicy",
	})
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "phi3-security-policy", Namespace: "default"}, &tp))

	kprobes, _, _ := unstructured.NestedSlice(tp.Object, "spec", "kprobes")
	require.Len(t, kprobes, 1)
	kp := kprobes[0].(map[string]interface{})
	assert.Equal(t, "sys_execve", kp["call"])
}

// ---- SPIREReconciler.ReconcileSecurityPolicy (placeholder) -----------------

func TestReconcileSecurityPolicy_NoError(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	require.NoError(t, r.ReconcileSecurityPolicy(context.Background(), svc))
}

// ---- validateSPIFFEID ------------------------------------------------------

func TestReconcileEbpfPolicy_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("gemma", "default")

	r := &EbpfReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))
	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))
}
