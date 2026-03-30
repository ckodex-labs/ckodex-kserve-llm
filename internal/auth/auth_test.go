/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	josev4 "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// skipIfNoTCP skips the test if TCP binding is unavailable (sandbox restriction).
func skipIfNoTCP(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("TCP binding unavailable in this environment: %v", err)
	}
	_ = ln.Close()
}

// ---- DefaultOIDCConfig -------------------------------------------------------

func TestDefaultOIDCConfig_ReturnsExpectedDefaults(t *testing.T) {
	cfg := DefaultOIDCConfig()
	assert.Equal(t, []string{"inference"}, cfg.RequiredScopes)
	assert.Contains(t, cfg.SkipPaths, "/healthz")
	assert.Contains(t, cfg.SkipPaths, "/metrics")
	assert.Equal(t, time.Hour, cfg.CacheTTL)
}

// ---- extractBearerToken ------------------------------------------------------

func TestExtractBearerToken_ValidBearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer mytoken123")
	tok, err := extractBearerToken(r)
	require.NoError(t, err)
	assert.Equal(t, "mytoken123", tok)
}

func TestExtractBearerToken_CaseInsensitive(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "bearer mytoken123")
	tok, err := extractBearerToken(r)
	require.NoError(t, err)
	assert.Equal(t, "mytoken123", tok)
}

func TestExtractBearerToken_MissingHeader_ReturnsErrNoToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := extractBearerToken(r)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoToken))
}

func TestExtractBearerToken_NoBearer_ReturnsErrNoToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	_, err := extractBearerToken(r)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoToken))
}

func TestExtractBearerToken_EmptyHeader_ReturnsErrNoToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "")
	_, err := extractBearerToken(r)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoToken))
}

// ---- ContextWithClaims / ClaimsFromContext -----------------------------------

func TestContextWithClaims_RoundTrip(t *testing.T) {
	claims := &InferenceClaims{Scope: "inference", TenantID: "tenant1"}
	ctx := ContextWithClaims(context.Background(), claims)
	got, ok := ClaimsFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, claims, got)
}

func TestClaimsFromContext_Missing_ReturnsFalse(t *testing.T) {
	_, ok := ClaimsFromContext(context.Background())
	assert.False(t, ok)
}

// ---- NewMiddleware / Authenticate -------------------------------------------

func makeHMACToken(t *testing.T, secret string, claims *InferenceClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestMiddleware_SkipPath_PassesThrough(t *testing.T) {
	cfg := DefaultOIDCConfig()
	cfg.ClientID = "test-secret"
	mw := NewMiddleware(cfg)

	called := false
	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMiddleware_NoToken_Returns401(t *testing.T) {
	cfg := DefaultOIDCConfig()
	cfg.ClientID = "test-secret"
	mw := NewMiddleware(cfg)

	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/completions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMiddleware_ValidToken_WithScope_PassesThrough(t *testing.T) {
	claims := &InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"test-audience"},
		},
		Scope: "inference",
	}
	// We need a token that passes Verify — use HMAC and override IssuerURL/Audience
	cfg2 := OIDCConfig{
		ClientID: "test-secret",
		Audience: "test-audience",
		CacheTTL: time.Hour,
	}
	verifier := NewTokenVerifier(cfg2)
	tok := makeHMACToken(t, "test-secret", claims)
	parsed, err := verifier.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
}

func TestMiddleware_ValidToken_MissingScope_Returns403(t *testing.T) {
	cfg := OIDCConfig{
		ClientID:       "test-secret",
		Audience:       "myapp",
		RequiredScopes: []string{"admin"},
		CacheTTL:       time.Hour,
	}
	mw := NewMiddleware(cfg)

	// Create a valid HMAC token that has only "inference" scope, not "admin"
	claims := &InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"myapp"},
		},
		Scope: "inference",
	}
	tok := makeHMACToken(t, "test-secret", claims)

	called := false
	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/completions", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assert.False(t, called)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ---- TokenVerifier.Verify -----------------------------------------------

func TestTokenVerifier_HMAC_ValidToken(t *testing.T) {
	cfg := OIDCConfig{
		ClientID: "mysecret",
		Audience: "test-api",
		CacheTTL: time.Hour,
	}
	v := NewTokenVerifier(cfg)
	claims := &InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"test-api"},
		},
		Scope:    "inference",
		TenantID: "acme",
	}
	tok := makeHMACToken(t, "mysecret", claims)
	got, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "inference", got.Scope)
	assert.Equal(t, "acme", got.TenantID)
}

func TestTokenVerifier_HMAC_InvalidSignature_ReturnsError(t *testing.T) {
	cfg := OIDCConfig{
		ClientID: "mysecret",
		CacheTTL: time.Hour,
	}
	v := NewTokenVerifier(cfg)
	claims := &InferenceClaims{Scope: "inference"}
	tok := makeHMACToken(t, "wrongsecret", claims)
	_, err := v.Verify(context.Background(), tok)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidToken))
}

func TestTokenVerifier_HMAC_ExpiredToken_ReturnsError(t *testing.T) {
	cfg := OIDCConfig{
		ClientID: "mysecret",
		CacheTTL: time.Hour,
	}
	v := NewTokenVerifier(cfg)
	claims := &InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-2 * time.Minute)),
		},
		Scope: "inference",
	}
	tok := makeHMACToken(t, "mysecret", claims)
	_, err := v.Verify(context.Background(), tok)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidToken))
}

func TestTokenVerifier_HMAC_WrongIssuer_ReturnsError(t *testing.T) {
	cfg := OIDCConfig{
		ClientID:  "mysecret",
		IssuerURL: "https://expected-issuer.example.com",
		CacheTTL:  time.Hour,
	}
	v := NewTokenVerifier(cfg)
	claims := &InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "https://other-issuer.example.com",
		},
		Scope: "inference",
	}
	tok := makeHMACToken(t, "mysecret", claims)
	_, err := v.Verify(context.Background(), tok)
	require.Error(t, err)
}

func TestTokenVerifier_InvalidJWT_ReturnsError(t *testing.T) {
	cfg := OIDCConfig{
		ClientID: "mysecret",
		CacheTTL: time.Hour,
	}
	v := NewTokenVerifier(cfg)
	_, err := v.Verify(context.Background(), "not.a.jwt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidToken))
}

func TestTokenVerifier_HMAC_WithAudience_ValidToken(t *testing.T) {
	cfg := OIDCConfig{
		ClientID: "mysecret",
		Audience: "inference-api",
		CacheTTL: time.Hour,
	}
	v := NewTokenVerifier(cfg)
	claims := &InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"inference-api"},
		},
		Scope: "inference",
	}
	tok := makeHMACToken(t, "mysecret", claims)
	got, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.NotNil(t, got)
}

// ---- TokenVerifier with RSA JWKS server ----------------------------------------

func TestTokenVerifier_RSA_ValidToken_WithJWKSServer(t *testing.T) {
	skipIfNoTCP(t)
	// Generate RSA key pair
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	const kid = "test-key-1"

	// Build a JWKS server
	jwk := josev4.JSONWebKey{
		Key:       &privKey.PublicKey,
		KeyID:     kid,
		Algorithm: string(josev4.RS256),
		Use:       "sig",
	}
	keySet := josev4.JSONWebKeySet{Keys: []josev4.JSONWebKey{jwk}}
	jwksJSON, err := json.Marshal(keySet)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	cfg := OIDCConfig{
		JWKSEndpoint: server.URL,
		Audience:     "my-api",
		CacheTTL:     time.Hour,
	}
	v := NewTokenVerifier(cfg)

	claims := &InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"my-api"},
		},
		Scope: "inference",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(privKey)
	require.NoError(t, err)

	got, err := v.Verify(context.Background(), signed)
	require.NoError(t, err)
	assert.Equal(t, "inference", got.Scope)
}

func TestTokenVerifier_JWKS_ServerError_ReturnsError(t *testing.T) {
	skipIfNoTCP(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := OIDCConfig{
		JWKSEndpoint: server.URL,
		CacheTTL:     time.Hour,
	}
	v := NewTokenVerifier(cfg)

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	claims := &InferenceClaims{Scope: "inference"}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "some-kid"
	signed, err := token.SignedString(privKey)
	require.NoError(t, err)

	_, err = v.Verify(context.Background(), signed)
	require.Error(t, err)
}

func TestTokenVerifier_JWKS_KidNotFound_ReturnsError(t *testing.T) {
	skipIfNoTCP(t)
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// JWKS has key with different kid
	jwk := josev4.JSONWebKey{
		Key:       &privKey.PublicKey,
		KeyID:     "different-kid",
		Algorithm: string(josev4.RS256),
		Use:       "sig",
	}
	keySet := josev4.JSONWebKeySet{Keys: []josev4.JSONWebKey{jwk}}
	jwksJSON, err := json.Marshal(keySet)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	cfg := OIDCConfig{
		JWKSEndpoint: server.URL,
		CacheTTL:     time.Hour,
	}
	v := NewTokenVerifier(cfg)

	claims := &InferenceClaims{Scope: "inference"}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "missing-kid"
	signed, err := token.SignedString(privKey)
	require.NoError(t, err)

	_, err = v.Verify(context.Background(), signed)
	require.Error(t, err)
}

func TestTokenVerifier_JWKS_Cached_SecondCallUsesCache(t *testing.T) {
	skipIfNoTCP(t)
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	const kid = "cached-key"
	jwk := josev4.JSONWebKey{
		Key:       &privKey.PublicKey,
		KeyID:     kid,
		Algorithm: string(josev4.RS256),
		Use:       "sig",
	}
	keySet := josev4.JSONWebKeySet{Keys: []josev4.JSONWebKey{jwk}}
	jwksJSON, err := json.Marshal(keySet)
	require.NoError(t, err)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	cfg := OIDCConfig{
		JWKSEndpoint: server.URL,
		Audience:     "cached-api",
		CacheTTL:     time.Hour,
	}
	v := NewTokenVerifier(cfg)

	makeRSAToken := func() string {
		claims := &InferenceClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Audience: jwt.ClaimStrings{"cached-api"},
			},
			Scope: "inference",
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = kid
		signed, err := token.SignedString(privKey)
		require.NoError(t, err)
		return signed
	}

	// First call fetches JWKS
	_, err = v.Verify(context.Background(), makeRSAToken())
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Second call uses cache
	_, err = v.Verify(context.Background(), makeRSAToken())
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "JWKS should be cached and not refetched")
}

func TestTokenVerifier_JWKS_InvalidJSONResponse_ReturnsError(t *testing.T) {
	skipIfNoTCP(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	cfg := OIDCConfig{
		JWKSEndpoint: server.URL,
		CacheTTL:     time.Hour,
	}
	v := NewTokenVerifier(cfg)

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	claims := &InferenceClaims{Scope: "inference"}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "kid1"
	signed, err := token.SignedString(privKey)
	require.NoError(t, err)

	_, err = v.Verify(context.Background(), signed)
	require.Error(t, err)
}

func TestTokenVerifier_JWKS_IssuerURL_BuildsJWKSEndpoint(t *testing.T) {
	skipIfNoTCP(t)
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	const kid = "iss-key"
	jwk := josev4.JSONWebKey{
		Key:   &privKey.PublicKey,
		KeyID: kid,
	}
	keySet := josev4.JSONWebKeySet{Keys: []josev4.JSONWebKey{jwk}}
	jwksJSON, err := json.Marshal(keySet)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's hitting the JWKS path derived from issuer
		if r.URL.Path == "/.well-known/jwks.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(jwksJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Set IssuerURL only — no JWKSEndpoint
	cfg := OIDCConfig{
		IssuerURL: server.URL,
		Audience:  "iss-api",
		CacheTTL:  time.Hour,
	}
	v := NewTokenVerifier(cfg)

	claims := &InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   server.URL,
			Audience: jwt.ClaimStrings{"iss-api"},
		},
		Scope: "inference",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(privKey)
	require.NoError(t, err)

	_, err = v.Verify(context.Background(), signed)
	require.NoError(t, err)
}

// ---- generateRawKey ---------------------------------------------------------

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
