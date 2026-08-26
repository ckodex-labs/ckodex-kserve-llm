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
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ---- scheme helpers --------------------------------------------------------

func TestSPIFFEIDForService_Format(t *testing.T) {
	r := &SPIREReconciler{}
	id := r.SPIFFEIDForService("prod", "vllm-sa", "llama3")
	assert.Equal(t, "spiffe://ckodex.com/ns/prod/sa/vllm-sa/model/llama3", id)
}

func TestSPIFFEIDForService_TrustDomain(t *testing.T) {
	r := &SPIREReconciler{}
	id := r.SPIFFEIDForService("any", "sa", "model")
	assert.Contains(t, id, "spiffe://"+SPIFFETrustDomain+"/")
}

func TestSPIFFEIDForService_DifferentInputs(t *testing.T) {
	r := &SPIREReconciler{}
	a := r.SPIFFEIDForService("ns1", "sa1", "m1")
	b := r.SPIFFEIDForService("ns2", "sa2", "m2")
	assert.NotEqual(t, a, b)
}

// ---- SPIREReconciler.ReconcileSPIRE ----------------------------------------

func TestReconcileSPIRE_CreatesStatefulSet(t *testing.T) {
	scheme := secScheme(t)
	r := &SPIREReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileSPIRE(context.Background(), "spire"))

	var ss appsv1.StatefulSet
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "spire-server", Namespace: "spire"}, &ss))

	assert.Equal(t, SPIREServerImage, ss.Spec.Template.Spec.Containers[0].Image)
}

func TestReconcileSPIRE_CreatesDaemonSet(t *testing.T) {
	scheme := secScheme(t)
	r := &SPIREReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileSPIRE(context.Background(), "spire"))

	var ds appsv1.DaemonSet
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "spire-agent", Namespace: "spire"}, &ds))

	assert.Equal(t, SPIREAgentImage, ds.Spec.Template.Spec.Containers[0].Image)
	assert.True(t, ds.Spec.Template.Spec.HostNetwork, "SPIRE agent requires HostNetwork for node attestation")
}

func TestReconcileSPIRE_ServerSecurityContext(t *testing.T) {
	scheme := secScheme(t)
	r := &SPIREReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileSPIRE(context.Background(), "default"))

	var ss appsv1.StatefulSet
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "spire-server", Namespace: "default"}, &ss))

	sc := ss.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc)
	assert.True(t, *sc.RunAsNonRoot)
	assert.False(t, *sc.AllowPrivilegeEscalation)
}

func TestReconcileSPIRE_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	r := &SPIREReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileSPIRE(context.Background(), "spire"))
	require.NoError(t, r.ReconcileSPIRE(context.Background(), "spire"))
}

// ---- SPIREReconciler.InjectSidecar -----------------------------------------
