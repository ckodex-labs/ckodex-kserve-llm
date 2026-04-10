/*
Copyright 2026 CKodex Authors.
*/

package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

type mockESClient struct {
	client.Client
	scheme *runtime.Scheme
	objects map[string]client.Object
}

func (m *mockESClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	m.objects[obj.GetName()] = obj
	return nil
}

func (m *mockESClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	m.objects[obj.GetName()] = obj
	return nil
}

func (m *mockESClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if found, ok := m.objects[key.Name]; ok {
		// Very simplified copy for test
		if u, ok := obj.(*unstructured.Unstructured); ok {
			if fu, ok := found.(*unstructured.Unstructured); ok {
				u.Object = fu.Object
				return nil
			}
		}
		if s, ok := obj.(*corev1.Secret); ok {
			if fs, ok := found.(*corev1.Secret); ok {
				s.Data = fs.Data
				return nil
			}
		}
	}
	return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
}

func (m *mockESClient) Scheme() *runtime.Scheme {
	return m.scheme
}

func (m *mockESClient) RESTMapper() meta.RESTMapper {
	// Return nil to trigger the Scheme-based fallback in our reconciler
	return nil
}

func TestManagedSecret_LatentSyncResilience(t *testing.T) {
	scheme := secScheme(t)
	// Explicitly register the GVK in the scheme for the fallback check
	scheme.AddKnownTypeWithName(ExternalSecretGVK, &unstructured.Unstructured{})

	svc := minimalLLMSvc("latent-secret", "default")
	svc.Spec.Model.Storage = &servingv1alpha2.StorageSpec{
		ExternalSecret: &servingv1alpha2.ExternalSecretSpec{
			SecretStoreRef: servingv1alpha2.SecretStoreRef{
				Name: "vault-backend",
			},
			Data: []servingv1alpha2.ExternalSecretData{
				{
					SecretKey: "token",
					RemoteRef: servingv1alpha2.ExternalSecretRemoteRef{
						Key: "secret/data/hf",
					},
				},
			},
		},
	}

	// Setup reconciler with our mock client
	mcl := &mockESClient{
		scheme:  scheme,
		objects: make(map[string]client.Object),
	}
	
	r := &ExternalSecretReconciler{
		Client: mcl,
		Scheme: scheme,
	}

	// 1. Perform reconciliation
	err := r.ReconcileExternalSecret(context.Background(), svc)
	require.NoError(t, err)

	// 2. Verify ExternalSecret was created
	es := &unstructured.Unstructured{}
	es.SetGroupVersionKind(ExternalSecretGVK)
	require.NoError(t, mcl.Get(context.Background(), 
		client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, es))

	// 3. Verify target secret name
	targetSecretName := svc.Name + "-external-secret"
	specMap, ok := es.Object["spec"].(map[string]interface{})
	require.True(t, ok, "spec should be a map")
	targetMap, ok := specMap["target"].(map[string]interface{})
	require.True(t, ok, "target should be a map")
	assert.Equal(t, targetSecretName, targetMap["name"])
	
	// 4. Simulate the secret appearing
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetSecretName,
			Namespace: svc.Namespace,
		},
		Data: map[string][]byte{"token": []byte("ready")},
	}
	require.NoError(t, mcl.Create(context.Background(), secret))

	// 5. Verify that fetching the secret now succeeds
	var found corev1.Secret
	require.NoError(t, mcl.Get(context.Background(), 
		client.ObjectKey{Name: targetSecretName, Namespace: svc.Namespace}, &found))
	assert.Equal(t, []byte("ready"), found.Data["token"])
}

func TestManagedSecret_UpdateIdempotency(t *testing.T) {
	scheme := secScheme(t)
	scheme.AddKnownTypeWithName(ExternalSecretGVK, &unstructured.Unstructured{})

	svc := minimalLLMSvc("update-test", "default")
	svc.Spec.Model.Storage = &servingv1alpha2.StorageSpec{
		ExternalSecret: &servingv1alpha2.ExternalSecretSpec{
			SecretStoreRef: servingv1alpha2.SecretStoreRef{Name: "old-store"},
		},
	}

	mcl := &mockESClient{
		scheme:  scheme,
		objects: make(map[string]client.Object),
	}
	r := &ExternalSecretReconciler{
		Client: mcl,
		Scheme: scheme,
	}

	// 1. Initial creation
	require.NoError(t, r.ReconcileExternalSecret(context.Background(), svc))
	
	es := &unstructured.Unstructured{}
	es.SetGroupVersionKind(ExternalSecretGVK)
	require.NoError(t, mcl.Get(context.Background(), client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, es))
	
	oldSpec := es.Object["spec"].(map[string]interface{})
	assert.Equal(t, "old-store", oldSpec["secretStoreRef"].(map[string]interface{})["name"])

	// 2. Update spec
	svc.Spec.Model.Storage.ExternalSecret.SecretStoreRef.Name = "new-store"
	require.NoError(t, r.ReconcileExternalSecret(context.Background(), svc))

	// 3. Verify update
	require.NoError(t, mcl.Get(context.Background(), client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, es))
	newSpec := es.Object["spec"].(map[string]interface{})
	assert.Equal(t, "new-store", newSpec["secretStoreRef"].(map[string]interface{})["name"])
}

func TestManagedSecret_GVKAbsenceResilience(t *testing.T) {
	scheme := runtime.NewScheme() // Empty scheme, does not recognize ExternalSecret
	svc := minimalLLMSvc("no-crd", "default")
	svc.Spec.Model.Storage = &servingv1alpha2.StorageSpec{
		ExternalSecret: &servingv1alpha2.ExternalSecretSpec{
			SecretStoreRef: servingv1alpha2.SecretStoreRef{Name: "vault"},
		},
	}

	mcl := &mockESClient{
		scheme:  scheme,
		objects: make(map[string]client.Object),
	}
	r := &ExternalSecretReconciler{
		Client: mcl,
		Scheme: scheme,
	}

	// This should NOT return an error, but skip (professionally non-blocking)
	err := r.ReconcileExternalSecret(context.Background(), svc)
	assert.NoError(t, err, "GVK absence must not cause reconciliation error")
	
	// Verify nothing was created
	assert.Empty(t, mcl.objects, "No objects should be created when GVK is missing")
}
