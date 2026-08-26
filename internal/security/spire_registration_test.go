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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ---- scheme helpers --------------------------------------------------------

func TestValidateSPIFFEID_Valid(t *testing.T) {
	require.NoError(t, validateSPIFFEID("default", "vllm-sa", "llama3"))
}

func TestValidateSPIFFEID_EmptyNamespace_Error(t *testing.T) {
	// An empty namespace produces path /ns//sa/vllm-sa/model/m — invalid SPIFFE path.
	err := validateSPIFFEID("", "vllm-sa", "model")
	require.Error(t, err)
}

func TestValidateSPIFFEID_ValidComplexNames(t *testing.T) {
	// Hyphens and numbers are permitted in K8s names and SPIFFE paths.
	require.NoError(t, validateSPIFFEID("prod-east", "llama3-sa", "llama-3-8b"))
}

// ---- SPIRERegistrationReconciler -------------------------------------------

func TestReconcileRegistrationEntry_CreatesConfigMap(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	r := &SPIRERegistrationReconciler{
		Client:          fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme:          scheme,
		SpireReconciler: &SPIREReconciler{},
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))

	var cm corev1.ConfigMap
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{
			Name:      SPIRERegistrationCMPrefix + "default-llama3",
			Namespace: SPIRERegistrationNamespace,
		}, &cm))

	assert.Contains(t, cm.Data["entry.json"], "spiffe://ckodex.com")
	assert.Equal(t, "true", cm.Labels["spire.ckodex.com/registration-entry"])
}

func TestReconcileRegistrationEntry_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("phi3", "staging")

	r := &SPIRERegistrationReconciler{
		Client:          fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme:          scheme,
		SpireReconciler: &SPIREReconciler{},
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))
	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))
}

func TestReconcileRegistrationEntry_EntryContainsTTL(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("gemma", "prod")

	r := &SPIRERegistrationReconciler{
		Client:          fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme:          scheme,
		SpireReconciler: &SPIREReconciler{},
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))

	var cm corev1.ConfigMap
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{
			Name:      SPIRERegistrationCMPrefix + "prod-gemma",
			Namespace: SPIRERegistrationNamespace,
		}, &cm))

	// TTL of 3600 must be present in the serialised entry
	assert.Contains(t, cm.Data["entry.json"], "3600")
}

func TestReconcileRegistrationEntry_EntryContainsDNSSAN(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("mistral", "default")

	r := &SPIRERegistrationReconciler{
		Client:          fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme:          scheme,
		SpireReconciler: &SPIREReconciler{},
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))

	var cm corev1.ConfigMap
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{
			Name:      SPIRERegistrationCMPrefix + "default-mistral",
			Namespace: SPIRERegistrationNamespace,
		}, &cm))

	// DNS SAN for in-cluster DNS resolution
	assert.Contains(t, cm.Data["entry.json"], "mistral.default.svc.cluster.local")
}

func TestDeleteRegistrationEntry_RemovesConfigMap(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	r := &SPIRERegistrationReconciler{
		Client:          fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme:          scheme,
		SpireReconciler: &SPIREReconciler{},
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))
	require.NoError(t, r.DeleteRegistrationEntry(context.Background(), "default", "llama3"))

	var cm corev1.ConfigMap
	err := r.Get(context.Background(),
		types.NamespacedName{
			Name:      SPIRERegistrationCMPrefix + "default-llama3",
			Namespace: SPIRERegistrationNamespace,
		}, &cm)
	assert.True(t, err != nil, "ConfigMap should be deleted")
}

func TestDeleteRegistrationEntry_NotFound_NoError(t *testing.T) {
	scheme := secScheme(t)

	r := &SPIRERegistrationReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	// Deleting a non-existent entry must be idempotent
	require.NoError(t, r.DeleteRegistrationEntry(context.Background(), "default", "nonexistent"))
}
