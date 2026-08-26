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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	josev4 "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// First call fetches JWKS
	_, err = v.Verify(context.Background(), signRSATestToken(t, privKey, kid, "cached-api"))
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Second call uses cache
	_, err = v.Verify(context.Background(), signRSATestToken(t, privKey, kid, "cached-api"))
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "JWKS should be cached and not refetched")
}

func signRSATestToken(t *testing.T, privKey *rsa.PrivateKey, kid, audience string) string {
	t.Helper()
	claims := &InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{audience}},
		Scope:            "inference",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(privKey)
	require.NoError(t, err)
	return signed
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
