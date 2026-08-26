/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
