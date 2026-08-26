/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// ---- scheme helpers --------------------------------------------------------

func TestInjectSidecar_AppendsCSIVolume(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	spec := &corev1.PodSpec{}

	r.InjectSidecar(spec, svc)

	require.Len(t, spec.Volumes, 1)
	assert.Equal(t, "spiffe-workload-api", spec.Volumes[0].Name)
	require.NotNil(t, spec.Volumes[0].CSI)
	assert.Equal(t, "spiffe.csi.spiffe.io", spec.Volumes[0].CSI.Driver)
}

func TestInjectSidecar_AppendsSidecarContainer(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm", Image: "vllm/vllm:latest"}},
	}

	r.InjectSidecar(spec, svc)

	// Original container + sidecar = 2
	require.Len(t, spec.Containers, 2)
	sidecar := spec.Containers[1]
	assert.Equal(t, "spiffe-sidecar", sidecar.Name)
	assert.Equal(t, SPIFFEHelperImage, sidecar.Image)
}

// CRITICAL: ReadOnlyRootFilesystem must be false — the helper writes cert/key PEM files.
func TestInjectSidecar_SidecarReadOnlyRootFilesystem_False(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm"}},
	}

	r.InjectSidecar(spec, svc)

	sidecar := spec.Containers[1]
	require.NotNil(t, sidecar.SecurityContext)
	require.NotNil(t, sidecar.SecurityContext.ReadOnlyRootFilesystem)
	assert.False(t, *sidecar.SecurityContext.ReadOnlyRootFilesystem,
		"spiffe-helper writes PEM files to /run/spiffe/certs — ReadOnlyRootFilesystem must be false")
}

func TestInjectSidecar_SidecarRunAsNonRoot_True(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm"}},
	}

	r.InjectSidecar(spec, svc)

	sidecar := spec.Containers[1]
	require.NotNil(t, sidecar.SecurityContext)
	assert.True(t, *sidecar.SecurityContext.RunAsNonRoot)
	assert.False(t, *sidecar.SecurityContext.AllowPrivilegeEscalation)
}

func TestInjectSidecar_SPIFFEEndpointSocket_EnvVar(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("phi3", "prod")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm"}},
	}

	r.InjectSidecar(spec, svc)

	// Primary container must have the env var
	primary := spec.Containers[0]
	var found bool
	for _, e := range primary.Env {
		if e.Name == "SPIFFE_ENDPOINT_SOCKET" {
			found = true
			assert.Contains(t, e.Value, SPIFFEWorkloadAPIPath)
		}
	}
	assert.True(t, found, "SPIFFE_ENDPOINT_SOCKET env var must be injected into the primary container")
}

func TestInjectSidecar_SidecarContainsSPIFFEID(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("gemma", "staging")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm"}},
	}

	r.InjectSidecar(spec, svc)

	sidecar := spec.Containers[1]
	var found bool
	for _, e := range sidecar.Env {
		if e.Name == "CKODEX_SPIFFE_ID" {
			found = true
			assert.Contains(t, e.Value, "staging")
			assert.Contains(t, e.Value, "gemma")
		}
	}
	assert.True(t, found, "CKODEX_SPIFFE_ID env var must be injected into the sidecar")
}

func TestInjectSidecar_NoPrimaryContainer_NoCrash(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("phi3", "default")
	spec := &corev1.PodSpec{} // no containers

	// Must not panic
	assert.NotPanics(t, func() {
		r.InjectSidecar(spec, svc)
	})
}

// ---- NetworkPolicyReconciler.ReconcileNetworkPolicies ----------------------
