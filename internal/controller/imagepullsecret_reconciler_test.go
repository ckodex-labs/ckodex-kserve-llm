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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func buildImagePullScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

func makeSourceSecret(ns, name string, dockerConfig []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfig,
		},
	}
}

// TestImagePullSecret_ReconcileNonTenantNamespace does nothing for namespaces
// without the tenant label.
func TestImagePullSecret_ReconcileNonTenantNamespace(t *testing.T) {
	s := buildImagePullScheme(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "plain-ns"}}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(ns).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s, OperatorNamespace: "ckodex-system"}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "plain-ns"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	var secrets corev1.SecretList
	require.NoError(t, cl.List(context.Background(), &secrets))
	assert.Empty(t, secrets.Items)
}

// TestImagePullSecret_ReconcileNoAnnotation does nothing when annotation is missing.
func TestImagePullSecret_ReconcileNoAnnotation(t *testing.T) {
	s := buildImagePullScheme(t)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "tenant-ns",
			Labels: map[string]string{LabelTenantID: "tenant-abc"},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(ns).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s, OperatorNamespace: "ckodex-system"}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "tenant-ns"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestImagePullSecret_ReconcileCreatesSecret distributes the pull secret when
// the namespace has the tenant label and the registry-secret annotation.
func TestImagePullSecret_ReconcileCreatesSecret(t *testing.T) {
	s := buildImagePullScheme(t)
	dockerCfg := []byte(`{"auths":{"ghcr.io":{"auth":"dGVzdA=="}}}`)

	sourceNS := "ckodex-system"
	sourceSecret := makeSourceSecret(sourceNS, "ghcr-pull-secret", dockerCfg)
	tenantNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "tenant-ns",
			Labels: map[string]string{LabelTenantID: "tenant-abc"},
			Annotations: map[string]string{
				AnnotationRegistrySecret: sourceNS + "/ghcr-pull-secret",
			},
		},
	}
	defaultSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountDefault,
			Namespace: "tenant-ns",
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(tenantNS, sourceSecret, defaultSA).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s, OperatorNamespace: sourceNS}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "tenant-ns"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify the pull secret was created in the tenant namespace.
	var pullSecret corev1.Secret
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name:      pullSecretName,
		Namespace: "tenant-ns",
	}, &pullSecret))
	assert.Equal(t, corev1.SecretTypeDockerConfigJson, pullSecret.Type)
	assert.Equal(t, dockerCfg, pullSecret.Data[corev1.DockerConfigJsonKey])
}

// TestImagePullSecret_ReconcileNotFound returns no error for missing namespace.
func TestImagePullSecret_ReconcileNotFound(t *testing.T) {
	s := buildImagePullScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s, OperatorNamespace: "ckodex-system"}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "missing-ns"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestImagePullSecret_ReconcileMalformedAnnotation ignores malformed secret refs.
func TestImagePullSecret_ReconcileMalformedAnnotation(t *testing.T) {
	s := buildImagePullScheme(t)
	tenantNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "tenant-ns",
			Labels: map[string]string{LabelTenantID: "tenant-abc"},
			Annotations: map[string]string{
				AnnotationRegistrySecret: "bad-format-no-slash",
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(tenantNS).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s, OperatorNamespace: "ckodex-system"}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "tenant-ns"},
	})
	require.NoError(t, err) // malformed annotation = log + skip, not error
	assert.Equal(t, ctrl.Result{}, result)
}

// TestEnsurePullSecret_UpdatesWhenDockerConfigChanges exercises the update path.
func TestEnsurePullSecret_UpdatesWhenDockerConfigChanges(t *testing.T) {
	s := buildImagePullScheme(t)

	oldCfg := []byte(`{"auths":{"old.io":{}}}`)
	newCfg := []byte(`{"auths":{"new.io":{}}}`)

	sourceSecret := makeSourceSecret("ckodex-system", "ghcr-pull-secret", newCfg)
	existingPullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: pullSecretName, Namespace: "tenant-ns"},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: oldCfg,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(existingPullSecret).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s}

	err := r.ensurePullSecret(context.Background(), "tenant-ns", sourceSecret)
	require.NoError(t, err)

	var updated corev1.Secret
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: pullSecretName, Namespace: "tenant-ns",
	}, &updated))
	assert.Equal(t, newCfg, updated.Data[corev1.DockerConfigJsonKey])
}

// TestEnsurePullSecret_IdempotentWhenUnchanged does not update if data is the same.
func TestEnsurePullSecret_IdempotentWhenUnchanged(t *testing.T) {
	s := buildImagePullScheme(t)

	dockerCfg := []byte(`{"auths":{"ghcr.io":{}}}`)
	sourceSecret := makeSourceSecret("ckodex-system", "ghcr-pull-secret", dockerCfg)
	existingPullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: pullSecretName, Namespace: "tenant-ns"},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerCfg,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(existingPullSecret).Build()
	r := &ImagePullSecretReconciler{Client: cl, Scheme: s}

	err := r.ensurePullSecret(context.Background(), "tenant-ns", sourceSecret)
	require.NoError(t, err)
}

// TestParseSecretRef validates the secret ref parsing logic.
func TestParseSecretRef(t *testing.T) {
	tests := []struct {
		input   string
		wantNS  string
		wantNm  string
		wantErr bool
	}{
		{"ckodex-system/ghcr-pull-secret", "ckodex-system", "ghcr-pull-secret", false},
		{"ns/name", "ns", "name", false},
		{"bad-format", "", "", true},
		{"/name", "", "", true},
		{"ns/", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ns, name, err := parseSecretRef(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantNS, ns)
				assert.Equal(t, tt.wantNm, name)
			}
		})
	}
}
