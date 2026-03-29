/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// SecretType marks a Secret as a CKodex API key.
	SecretType corev1.SecretType = "ckodex.com/api-key"

	// AnnotationTenantID carries the tenant this key belongs to.
	AnnotationTenantID = "ckodex.com/tenant-id"
	// AnnotationModelAccess is a comma-separated list of allowed models, or "*".
	AnnotationModelAccess = "ckodex.com/model-access"
	// AnnotationTokenBudget holds the per-key token budget as a decimal string.
	AnnotationTokenBudget = "ckodex.com/token-budget"
	// AnnotationRotateAfter holds an RFC3339 timestamp after which the key must be rotated.
	AnnotationRotateAfter = "ckodex.com/rotate-after"
	// AnnotationScopes holds space-separated OAuth2 scopes for the key.
	AnnotationScopes = "ckodex.com/scopes"

	// SecretKeyHash is the data key holding the bcrypt hash of the raw API key.
	SecretKeyHash = "hash"

	// keyPrefix is prepended to raw API keys for easy identification.
	keyPrefix = "ckx_"
	// rawKeyBytes is the number of random bytes before hex-encoding.
	rawKeyBytes = 32
	// bcryptCost is the bcrypt work factor. 12 is a reasonable production default.
	bcryptCost = 12
)

// ErrKeyExpired is returned when the API key has passed its rotation deadline.
var ErrKeyExpired = errors.New("api key rotation deadline exceeded")

// APIKeyStore validates API keys stored as Kubernetes Secrets of type ckodex.com/api-key.
// Keys are bcrypt-hashed — the plaintext is never persisted after creation.
//
// Lookup strategy: the store searches Secrets in the given namespace whose type
// matches SecretType. Because bcrypt hashes cannot be indexed, lookup is O(n) over
// matching secrets in the namespace. In practice a namespace holds few keys so this
// is acceptable; a pre-built index can be added later without API changes.
type APIKeyStore struct {
	client    client.Client
	namespace string
}

// NewAPIKeyStore creates a store backed by the given controller-runtime client.
func NewAPIKeyStore(c client.Client, namespace string) *APIKeyStore {
	return &APIKeyStore{client: c, namespace: namespace}
}

// Validate checks the raw API key against stored bcrypt hashes.
// On success it returns synthesised InferenceClaims built from Secret annotations.
func (s *APIKeyStore) Validate(ctx context.Context, rawKey string) (*InferenceClaims, error) {
	if !strings.HasPrefix(rawKey, keyPrefix) {
		return nil, ErrInvalidToken
	}

	var secrets corev1.SecretList
	if err := s.client.List(ctx, &secrets,
		client.InNamespace(s.namespace),
		client.MatchingFields{"type": string(SecretType)},
	); err != nil {
		return nil, fmt.Errorf("list api-key secrets: %w", err)
	}

	for i := range secrets.Items {
		sec := &secrets.Items[i]
		if sec.Type != SecretType {
			continue
		}

		hash, ok := sec.Data[SecretKeyHash]
		if !ok {
			continue
		}

		if err := bcrypt.CompareHashAndPassword(hash, []byte(rawKey)); err != nil {
			continue // wrong key — try next secret
		}

		// Correct key found — check rotation deadline.
		if ra, ok := sec.Annotations[AnnotationRotateAfter]; ok && ra != "" {
			deadline, err := time.Parse(time.RFC3339, ra)
			if err == nil && time.Now().After(deadline) {
				return nil, ErrKeyExpired
			}
		}

		return claimsFromSecret(sec), nil
	}

	return nil, ErrInvalidToken
}

// claimsFromSecret synthesises InferenceClaims from Secret annotations.
func claimsFromSecret(sec *corev1.Secret) *InferenceClaims {
	claims := &InferenceClaims{}

	if s, ok := sec.Annotations[AnnotationScopes]; ok {
		claims.Scope = s
	}
	if t, ok := sec.Annotations[AnnotationTenantID]; ok {
		claims.TenantID = t
	}
	if ma, ok := sec.Annotations[AnnotationModelAccess]; ok && ma != "" {
		claims.ModelAccess = strings.Split(ma, ",")
	}

	return claims
}

// GenerateKey creates a new random API key and stores its bcrypt hash as a
// Kubernetes Secret. Returns the raw key (shown only once) and the Secret name.
func (s *APIKeyStore) GenerateKey(ctx context.Context, name string, opts APIKeyOptions) (rawKey string, secretName string, err error) {
	raw, err := generateRawKey()
	if err != nil {
		return "", "", fmt.Errorf("generate random key: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(raw), bcryptCost)
	if err != nil {
		return "", "", fmt.Errorf("hash key: %w", err)
	}

	annotations := map[string]string{}
	if opts.TenantID != "" {
		annotations[AnnotationTenantID] = opts.TenantID
	}
	if len(opts.ModelAccess) > 0 {
		annotations[AnnotationModelAccess] = strings.Join(opts.ModelAccess, ",")
	}
	if !opts.RotateAfter.IsZero() {
		annotations[AnnotationRotateAfter] = opts.RotateAfter.UTC().Format(time.RFC3339)
	}
	if opts.Scopes != "" {
		annotations[AnnotationScopes] = opts.Scopes
	}

	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   s.namespace,
			Annotations: annotations,
		},
		Type: SecretType,
		Data: map[string][]byte{
			SecretKeyHash: hash,
		},
	}

	if err := s.client.Create(ctx, sec); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return "", "", fmt.Errorf("api key %q already exists", name)
		}
		return "", "", fmt.Errorf("create api-key secret: %w", err)
	}

	return raw, name, nil
}

// APIKeyOptions configures a new API key at creation time.
type APIKeyOptions struct {
	TenantID    string
	ModelAccess []string
	Scopes      string    // space-separated OAuth2 scopes
	RotateAfter time.Time // zero = no deadline
}

func generateRawKey() (string, error) {
	b := make([]byte, rawKeyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return keyPrefix + hex.EncodeToString(b), nil
}
