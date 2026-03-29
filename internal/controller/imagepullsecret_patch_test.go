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
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func buildPullSecretScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	return s
}

// TestPatchServiceAccount_AddsSecret when SA exists without the pull secret.
func TestPatchServiceAccount_AddsSecret(t *testing.T) {
	s := buildPullSecretScheme(t)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: serviceAccountDefault, Namespace: "default"},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(sa).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s}

	err := r.patchServiceAccount(context.Background(), "default")
	require.NoError(t, err)

	var updated corev1.ServiceAccount
	require.NoError(t, cl.Get(context.Background(),
		k8stypes.NamespacedName{Namespace: "default", Name: serviceAccountDefault}, &updated))

	found := false
	for _, ref := range updated.ImagePullSecrets {
		if ref.Name == pullSecretName {
			found = true
			break
		}
	}
	assert.True(t, found, "expected %q to be patched onto ServiceAccount", pullSecretName)
}

// TestPatchServiceAccount_AlreadyPresent_NoOp returns nil without re-patching.
func TestPatchServiceAccount_AlreadyPresent_NoOp(t *testing.T) {
	s := buildPullSecretScheme(t)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: serviceAccountDefault, Namespace: "default"},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: pullSecretName}, // already present
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(sa).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s}

	err := r.patchServiceAccount(context.Background(), "default")
	// Idempotent — no error when secret already present.
	require.NoError(t, err)
}

// TestPatchServiceAccount_SANotFound returns error when SA does not exist.
func TestPatchServiceAccount_SANotFound(t *testing.T) {
	s := buildPullSecretScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s}

	err := r.patchServiceAccount(context.Background(), "missing-namespace")
	require.Error(t, err)
}
