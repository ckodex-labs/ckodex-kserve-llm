/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckodex-labs/kserve-llm-operator/internal/auth"
)

// contextWithClaims injects InferenceClaims into a context for testing.
func contextWithClaims(claims *auth.InferenceClaims) context.Context {
	return auth.ContextWithClaims(context.Background(), claims)
}

// ---- InferenceClaims ---------------------------------------------------------

func TestInferenceClaims_HasScope_Present(t *testing.T) {
	c := &auth.InferenceClaims{Scope: "inference read write"}
	assert.True(t, c.HasScope("inference"))
	assert.True(t, c.HasScope("read"))
	assert.True(t, c.HasScope("write"))
}

func TestInferenceClaims_HasScope_Absent(t *testing.T) {
	c := &auth.InferenceClaims{Scope: "inference read"}
	assert.False(t, c.HasScope("admin"))
	assert.False(t, c.HasScope("write"))
}

func TestInferenceClaims_HasScope_EmptyScope(t *testing.T) {
	c := &auth.InferenceClaims{Scope: ""}
	assert.False(t, c.HasScope("inference"))
}

func TestInferenceClaims_CanAccessModel_NoRestriction(t *testing.T) {
	// Empty ModelAccess = open access
	c := &auth.InferenceClaims{ModelAccess: nil}
	assert.True(t, c.CanAccessModel("llama3"))
	assert.True(t, c.CanAccessModel("any-model"))
}

func TestInferenceClaims_CanAccessModel_Allowed(t *testing.T) {
	c := &auth.InferenceClaims{ModelAccess: []string{"llama3", "mistral"}}
	assert.True(t, c.CanAccessModel("llama3"))
	assert.True(t, c.CanAccessModel("mistral"))
}

func TestInferenceClaims_CanAccessModel_Denied(t *testing.T) {
	c := &auth.InferenceClaims{ModelAccess: []string{"llama3"}}
	assert.False(t, c.CanAccessModel("gpt-4"))
	assert.False(t, c.CanAccessModel(""))
}

func TestInferenceClaims_CanAccessModel_Wildcard(t *testing.T) {
	c := &auth.InferenceClaims{ModelAccess: []string{"*"}}
	assert.True(t, c.CanAccessModel("any-model"))
	assert.True(t, c.CanAccessModel("another-model"))
}

// ---- TokenBudgetEnforcer.Remaining -------------------------------------------

func TestTokenBudgetEnforcer_Remaining_UnknownSubject(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	assert.Equal(t, int64(-1), e.Remaining("nobody"))
}

func TestTokenBudgetEnforcer_Remaining_AfterRestore(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	e.RestoreFromAnnotation("alice", 500)
	assert.Equal(t, int64(500), e.Remaining("alice"))
}

// ---- TokenBudgetEnforcer.Consume ---------------------------------------------

func TestTokenBudgetEnforcer_Consume_UnknownSubject_PassThrough(t *testing.T) {
	// No budget → always ok (open access)
	e := auth.NewTokenBudgetEnforcer()
	require.NoError(t, e.Consume("unknown", 1000))
}

func TestTokenBudgetEnforcer_Consume_WithinBudget(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	e.RestoreFromAnnotation("alice", 1000)
	require.NoError(t, e.Consume("alice", 400))
	assert.Equal(t, int64(600), e.Remaining("alice"))
}

func TestTokenBudgetEnforcer_Consume_ExactBudget(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	e.RestoreFromAnnotation("bob", 100)
	require.NoError(t, e.Consume("bob", 100))
	assert.Equal(t, int64(0), e.Remaining("bob"))
}

func TestTokenBudgetEnforcer_Consume_ExceedsBudget(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	e.RestoreFromAnnotation("carol", 50)
	err := e.Consume("carol", 51)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token budget exhausted")
	// Balance must not have changed
	assert.Equal(t, int64(50), e.Remaining("carol"))
}

func TestTokenBudgetEnforcer_Consume_ZeroTokens(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	e.RestoreFromAnnotation("dave", 100)
	require.NoError(t, e.Consume("dave", 0))
	assert.Equal(t, int64(100), e.Remaining("dave"))
}

// ---- TokenBudgetEnforcer.RestoreFromAnnotation --------------------------------

func TestTokenBudgetEnforcer_RestoreFromAnnotation_Overwrites(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	e.RestoreFromAnnotation("eve", 100)
	e.RestoreFromAnnotation("eve", 999) // override (e.g. session reconciler re-syncs)
	assert.Equal(t, int64(999), e.Remaining("eve"))
}

// ---- TokenBudgetEnforcer.EnforceBudget HTTP middleware -----------------------

func makeRequest(ctx context.Context) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/completions", nil)
	return r.WithContext(ctx)
}

func TestEnforceBudget_NoClaims_PassThrough(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	called := false
	handler := e.EnforceBudget(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, makeRequest(context.Background()))
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceBudget_ZeroBudgetClaim_PassThrough(t *testing.T) {
	// TokenBudget == 0 means "no limit declared"
	e := auth.NewTokenBudgetEnforcer()
	claims := &auth.InferenceClaims{TokenBudget: 0}
	claims.Subject = "frank"

	called := false
	handler := e.EnforceBudget(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, makeRequest(contextWithClaims(claims)))
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceBudget_WithBudget_FirstRequest_Allowed(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	claims := &auth.InferenceClaims{TokenBudget: 1000}
	claims.Subject = "grace"

	called := false
	handler := e.EnforceBudget(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, makeRequest(contextWithClaims(claims)))
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceBudget_ExhaustedBudget_Blocked(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	e.RestoreFromAnnotation("henry", 0) // exhausted

	claims := &auth.InferenceClaims{TokenBudget: 500}
	claims.Subject = "henry"

	called := false
	handler := e.EnforceBudget(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, makeRequest(contextWithClaims(claims)))
	assert.False(t, called)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// ---- TokenBudgetEnforcer.RecordConsumption -----------------------------------

func TestRecordConsumption_NoClaims_NoOp(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	// Must not panic
	e.RecordConsumption(context.Background(), 100)
}

func TestRecordConsumption_ZeroBudgetClaim_NoOp(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	claims := &auth.InferenceClaims{TokenBudget: 0}
	claims.Subject = "iris"
	e.RecordConsumption(contextWithClaims(claims), 100)
	// No budget tracking should occur
	assert.Equal(t, int64(-1), e.Remaining("iris"))
}

func TestRecordConsumption_DeductsFromBudget(t *testing.T) {
	e := auth.NewTokenBudgetEnforcer()
	e.RestoreFromAnnotation("jack", 1000)
	claims := &auth.InferenceClaims{TokenBudget: 1000}
	claims.Subject = "jack"
	e.RecordConsumption(contextWithClaims(claims), 300)
	assert.Equal(t, int64(700), e.Remaining("jack"))
}

// ---- EnforceModelAccess middleware -------------------------------------------

func modelFromPath(r *http.Request) string {
	// Extract last path segment as model name: /v1/models/llama3 → "llama3"
	parts := r.URL.Path
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == '/' {
			return parts[i+1:]
		}
	}
	return ""
}

func TestEnforceModelAccess_NoClaims_PassThrough(t *testing.T) {
	mw := auth.EnforceModelAccess(modelFromPath)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/models/llama3", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assert.True(t, called)
}

func TestEnforceModelAccess_AllowedModel_PassThrough(t *testing.T) {
	mw := auth.EnforceModelAccess(modelFromPath)
	claims := &auth.InferenceClaims{ModelAccess: []string{"llama3", "mistral"}}
	claims.Subject = "kate"

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/models/llama3", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(contextWithClaims(claims)))
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceModelAccess_DeniedModel_Forbidden(t *testing.T) {
	mw := auth.EnforceModelAccess(modelFromPath)
	claims := &auth.InferenceClaims{ModelAccess: []string{"llama3"}}
	claims.Subject = "leo"

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/models/gpt-4", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(contextWithClaims(claims)))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestEnforceModelAccess_EmptyModelName_PassThrough(t *testing.T) {
	// When the extractor returns "" enforcement is skipped.
	mw := auth.EnforceModelAccess(func(*http.Request) string { return "" })
	claims := &auth.InferenceClaims{ModelAccess: []string{"llama3"}}
	claims.Subject = "mike"

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/completions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(contextWithClaims(claims)))
	assert.True(t, called)
}

func TestEnforceModelAccess_WildcardAccess_Allowed(t *testing.T) {
	mw := auth.EnforceModelAccess(modelFromPath)
	claims := &auth.InferenceClaims{ModelAccess: []string{"*"}}
	claims.Subject = "nina"

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	r := httptest.NewRequest(http.MethodPost, "/v1/models/any-model", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(contextWithClaims(claims)))
	assert.True(t, called)
}
