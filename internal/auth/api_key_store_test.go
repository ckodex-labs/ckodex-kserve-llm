/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGenerateRawKey_HasPrefix(t *testing.T) {
	key, err := generateRawKey()
	require.NoError(t, err)
	assert.True(t, len(key) > len(keyPrefix))
	assert.Equal(t, keyPrefix, key[:len(keyPrefix)])
}

func TestGenerateRawKey_IsUnique(t *testing.T) {
	k1, _ := generateRawKey()
	k2, _ := generateRawKey()
	assert.NotEqual(t, k1, k2)
}

// ---- APIKeyStore (with fake k8s client) ---------------------------------------

// newFakeAPIKeyStore creates an APIKeyStore backed by a fake k8s client with
// the "type" field index registered so List with MatchingFields works.
func newFakeAPIKeyStore(objs ...client.Object) *APIKeyStore {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithIndex(&corev1.Secret{}, "type", func(obj client.Object) []string {
			s, ok := obj.(*corev1.Secret)
			if !ok {
				return nil
			}
			return []string{string(s.Type)}
		}).
		Build()
	return NewAPIKeyStore(fc, "default")
}

func TestAPIKeyStore_NewAPIKeyStore_Constructed(t *testing.T) {
	store := newFakeAPIKeyStore()
	assert.NotNil(t, store)
}

func TestAPIKeyStore_GenerateKey_CreatesSecret(t *testing.T) {
	store := newFakeAPIKeyStore()

	rawKey, secretName, err := store.GenerateKey(context.Background(), "mykey", APIKeyOptions{
		TenantID:    "acme",
		ModelAccess: []string{"llama3", "mistral"},
		Scopes:      "inference",
	})
	require.NoError(t, err)
	assert.Equal(t, "mykey", secretName)
	assert.True(t, len(rawKey) > len(keyPrefix))
	assert.Equal(t, keyPrefix, rawKey[:len(keyPrefix)])
}

func TestAPIKeyStore_GenerateKey_AlreadyExists_ReturnsError(t *testing.T) {
	store := newFakeAPIKeyStore()

	_, _, err := store.GenerateKey(context.Background(), "dupkey", APIKeyOptions{})
	require.NoError(t, err)

	_, _, err = store.GenerateKey(context.Background(), "dupkey", APIKeyOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestAPIKeyStore_GenerateKey_WithRotateAfter(t *testing.T) {
	store := newFakeAPIKeyStore()

	rotateAfter := time.Now().Add(24 * time.Hour)
	rawKey, _, err := store.GenerateKey(context.Background(), "rotating-key", APIKeyOptions{
		RotateAfter: rotateAfter,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, rawKey)
}

func TestAPIKeyStore_Validate_WrongPrefix_ReturnsErrInvalidToken(t *testing.T) {
	store := newFakeAPIKeyStore()
	_, err := store.Validate(context.Background(), "sk-invalid-prefix-key")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidToken))
}

func TestAPIKeyStore_Validate_NoSecrets_ReturnsErrInvalidToken(t *testing.T) {
	store := newFakeAPIKeyStore()
	_, err := store.Validate(context.Background(), keyPrefix+"aaaaaaaaaaaaaaaa")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidToken))
}

func TestAPIKeyStore_Validate_ValidKey_ReturnsClaims(t *testing.T) {
	store := newFakeAPIKeyStore()

	rawKey, _, err := store.GenerateKey(context.Background(), "valid-key", APIKeyOptions{
		TenantID:    "test-tenant",
		ModelAccess: []string{"llama3"},
		Scopes:      "inference",
	})
	require.NoError(t, err)

	claims, err := store.Validate(context.Background(), rawKey)
	require.NoError(t, err)
	assert.Equal(t, "test-tenant", claims.TenantID)
	assert.Equal(t, []string{"llama3"}, claims.ModelAccess)
	assert.Equal(t, "inference", claims.Scope)
}

func TestAPIKeyStore_Validate_ExpiredKey_ReturnsErrKeyExpired(t *testing.T) {
	store := newFakeAPIKeyStore()

	rawKey, _, err := store.GenerateKey(context.Background(), "expired-key", APIKeyOptions{
		RotateAfter: time.Now().Add(-1 * time.Hour), // already expired
	})
	require.NoError(t, err)

	_, err = store.Validate(context.Background(), rawKey)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrKeyExpired))
}

func TestAPIKeyStore_Validate_WrongKey_ReturnsErrInvalidToken(t *testing.T) {
	store := newFakeAPIKeyStore()

	_, _, err := store.GenerateKey(context.Background(), "some-key", APIKeyOptions{})
	require.NoError(t, err)

	// Try a different key with the right prefix
	_, err = store.Validate(context.Background(), keyPrefix+"wrongwrongwrongwrongwrongwrongwrong")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidToken))
}

func TestClaimsFromSecret_AllAnnotations(t *testing.T) {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				AnnotationTenantID:    "acme",
				AnnotationScopes:      "inference read",
				AnnotationModelAccess: "llama3,mistral",
			},
		},
	}
	claims := claimsFromSecret(sec)
	assert.Equal(t, "acme", claims.TenantID)
	assert.Equal(t, "inference read", claims.Scope)
	assert.Equal(t, []string{"llama3", "mistral"}, claims.ModelAccess)
}

func TestClaimsFromSecret_NoAnnotations_EmptyClaims(t *testing.T) {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{},
	}
	claims := claimsFromSecret(sec)
	assert.Empty(t, claims.TenantID)
	assert.Empty(t, claims.Scope)
	assert.Nil(t, claims.ModelAccess)
}

func TestClaimsFromSecret_EmptyModelAccess_NotSplit(t *testing.T) {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				AnnotationModelAccess: "",
			},
		},
	}
	claims := claimsFromSecret(sec)
	assert.Nil(t, claims.ModelAccess)
}
