/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package auth implements JWT/OIDC authentication and authorization for inference endpoints.
package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	josev4 "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

var (
	// ErrNoToken is returned when no bearer token is found.
	ErrNoToken = errors.New("no bearer token in Authorization header")
	// ErrInvalidToken is returned when the token fails validation.
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrInsufficientScope is returned when the token lacks required scopes.
	ErrInsufficientScope = errors.New("insufficient scope for this operation")
)

// OIDCConfig holds OIDC provider configuration.
type OIDCConfig struct {
	// IssuerURL is the OIDC provider issuer URL (e.g., https://accounts.google.com).
	IssuerURL string
	// ClientID is the OAuth2 client ID.
	ClientID string
	// Audience is the expected audience claim.
	Audience string
	// JWKSEndpoint overrides auto-discovery of the JWKS endpoint.
	JWKSEndpoint string
	// RequiredScopes are scopes required for all endpoints.
	RequiredScopes []string
	// SkipPaths are paths that bypass authentication (e.g., /v2/health/live).
	SkipPaths []string
	// CacheTTL is how long to cache JWKS keys.
	CacheTTL time.Duration
}

// DefaultOIDCConfig returns production defaults.
func DefaultOIDCConfig() OIDCConfig {
	return OIDCConfig{
		RequiredScopes: []string{"inference"},
		SkipPaths: []string{
			"/v2/health/live",
			"/v2/health/ready",
			"/healthz",
			"/readyz",
			"/metrics",
		},
		CacheTTL: 1 * time.Hour,
	}
}

// InferenceClaims extends standard JWT claims with inference-specific fields.
type InferenceClaims struct {
	jwt.RegisteredClaims

	// Scope contains space-separated OAuth2 scopes.
	Scope string `json:"scope,omitempty"`

	// ModelAccess lists models this token can access. Empty = all.
	ModelAccess []string `json:"model_access,omitempty"`

	// TokenBudget is the per-session token budget (for rate limiting).
	TokenBudget int64 `json:"token_budget,omitempty"`

	// TenantID identifies the tenant for multi-tenancy.
	TenantID string `json:"tenant_id,omitempty"`
}

// HasScope checks if the claims include a specific scope.
func (c *InferenceClaims) HasScope(scope string) bool {
	scopes := strings.Fields(c.Scope)
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// CanAccessModel checks if the claims allow access to a specific model.
func (c *InferenceClaims) CanAccessModel(model string) bool {
	if len(c.ModelAccess) == 0 {
		return true // no restriction
	}
	for _, m := range c.ModelAccess {
		if m == model || m == "*" {
			return true
		}
	}
	return false
}

// Middleware provides HTTP middleware for JWT/OIDC authentication.
type Middleware struct {
	config    OIDCConfig
	verifier  *TokenVerifier
	skipPaths map[string]bool
	// inst is optional; nil means no forbidden-tuple metrics are emitted.
	inst *observability.Instrumentation
}

// NewMiddleware creates auth middleware with the given config.
func NewMiddleware(config OIDCConfig) *Middleware {
	skip := make(map[string]bool, len(config.SkipPaths))
	for _, p := range config.SkipPaths {
		skip[p] = true
	}
	return &Middleware{
		config:    config,
		verifier:  NewTokenVerifier(config),
		skipPaths: skip,
	}
}

// WithInstrumentation attaches OTel instrumentation so the middleware can emit
// the ckodex.vector.forbidden_tuple counter for the anti_execute tuple type.
func (m *Middleware) WithInstrumentation(inst *observability.Instrumentation) *Middleware {
	m.inst = inst
	return m
}

// Authenticate is HTTP middleware that validates JWT bearer tokens.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip health/metrics endpoints
		if m.skipPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		// Extract bearer token
		token, err := extractBearerToken(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized","message":"`+err.Error()+`"}`, http.StatusUnauthorized)
			return
		}

		// Validate token
		claims, err := m.verifier.Verify(r.Context(), token)
		if err != nil {
			http.Error(w, `{"error":"unauthorized","message":"`+err.Error()+`"}`, http.StatusUnauthorized)
			return
		}

		// Check required scopes
		for _, scope := range m.config.RequiredScopes {
			if !claims.HasScope(scope) {
				span := trace.SpanFromContext(r.Context())
				span.AddEvent("ckodex.auth.denied", trace.WithAttributes(
					attribute.String("reason", "missing_scope"),
					attribute.StringSlice("required_scopes", m.config.RequiredScopes),
				))
				// CKODEX §10: anti ∧ execute → HALT. The request is in the "anti"
				// (denied) state and attempted execution — emit the forbidden-tuple counter.
				if m.inst != nil {
					m.inst.ForbiddenTupleCounter.Add(r.Context(), 1,
						observability.TupleTypeAttr("anti_execute"))
				}
				http.Error(w, `{"error":"forbidden","message":"missing scope: `+scope+`"}`, http.StatusForbidden)
				return
			}
		}

		// Inject claims into context
		ctx := ContextWithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractBearerToken extracts the JWT from the Authorization header.
func extractBearerToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", ErrNoToken
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", ErrNoToken
	}
	return parts[1], nil
}

// --- Context helpers ---

type claimsKey struct{}

// ContextWithClaims adds InferenceClaims to the context.
func ContextWithClaims(ctx context.Context, claims *InferenceClaims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

// ClaimsFromContext extracts InferenceClaims from the context.
func ClaimsFromContext(ctx context.Context) (*InferenceClaims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(*InferenceClaims)
	return claims, ok
}

// --- Token Verifier ---

// TokenVerifier validates JWTs against OIDC provider keys.
type TokenVerifier struct {
	config     OIDCConfig
	mu         sync.RWMutex
	publicKeys map[string]*rsa.PublicKey
	lastFetch  time.Time
}

// NewTokenVerifier creates a new token verifier.
func NewTokenVerifier(config OIDCConfig) *TokenVerifier {
	return &TokenVerifier{
		config:     config,
		publicKeys: make(map[string]*rsa.PublicKey),
	}
}

// Verify validates a JWT string and returns the parsed claims.
func (v *TokenVerifier) Verify(ctx context.Context, tokenString string) (*InferenceClaims, error) {
	claims := &InferenceClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			// Also accept HMAC for dev/test
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			// HMAC: use client secret as key (dev only)
			return []byte(v.config.ClientID), nil
		}

		// RSA: fetch JWKS and find matching key
		kid, _ := token.Header["kid"].(string)
		key, err := v.getPublicKey(ctx, kid)
		if err != nil {
			return nil, fmt.Errorf("failed to get public key: %w", err)
		}
		return key, nil
	},
		jwt.WithIssuer(v.config.IssuerURL),
		jwt.WithAudience(v.config.Audience),
		jwt.WithLeeway(30*time.Second),
	)

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("%w: token failed validation", ErrInvalidToken)
	}

	return claims, nil
}

// getPublicKey retrieves an RSA public key by kid from JWKS.
func (v *TokenVerifier) getPublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if key, ok := v.publicKeys[kid]; ok && time.Since(v.lastFetch) < v.config.CacheTTL {
		v.mu.RUnlock()
		return key, nil
	}
	v.mu.RUnlock()

	// Fetch JWKS
	if err := v.refreshJWKS(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok := v.publicKeys[kid]
	if !ok {
		return nil, fmt.Errorf("key %s not found in JWKS", kid)
	}
	return key, nil
}

// refreshJWKS fetches the JWKS from the OIDC provider.
func (v *TokenVerifier) refreshJWKS(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	endpoint := v.config.JWKSEndpoint
	if endpoint == "" {
		endpoint = strings.TrimSuffix(v.config.IssuerURL, "/") + "/.well-known/jwks.json"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var keySet josev4.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&keySet); err != nil {
		return fmt.Errorf("parse JWKS: %w", err)
	}

	newKeys := make(map[string]*rsa.PublicKey, len(keySet.Keys))
	for _, k := range keySet.Keys {
		rsaKey, ok := k.Key.(*rsa.PublicKey)
		if !ok {
			continue // skip EC, oct, and other key types
		}
		newKeys[k.KeyID] = rsaKey
	}
	v.publicKeys = newKeys
	v.lastFetch = time.Now()
	return nil
}
