/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package auth — enforcement.go wires InferenceClaims fields into request handling.
// TokenBudget and ModelAccess are parsed from JWTs in oidc_middleware.go but were
// never enforced. This file closes that gap.
package auth

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TokenBudgetEnforcer tracks token consumption against the per-session budget
// declared in InferenceClaims.TokenBudget. Budget state is persisted on
// InferenceSession annotations so it survives controller restarts.
//
// When TokenBudget == 0 the claim is absent and enforcement is skipped (open access).
type TokenBudgetEnforcer struct {
	mu      sync.Mutex
	budgets map[string]int64 // subject → tokens remaining
}

// NewTokenBudgetEnforcer creates an enforcer backed by an in-memory map.
// For production with EnableDapr=true the map will be replaced by a Dapr state
// store (Phase 12). For now it restores from InferenceSession annotations on
// first access via RestoreFromAnnotation.
func NewTokenBudgetEnforcer() *TokenBudgetEnforcer {
	return &TokenBudgetEnforcer{
		budgets: make(map[string]int64),
	}
}

// RestoreFromAnnotation seeds the remaining budget for a subject from a persisted
// annotation value. Call this during session reconciliation.
func (e *TokenBudgetEnforcer) RestoreFromAnnotation(subject string, remaining int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.budgets[subject] = remaining
}

// Remaining returns the number of tokens left for the subject.
// Returns -1 if no budget is being tracked (budget was never set).
func (e *TokenBudgetEnforcer) Remaining(subject string) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	v, ok := e.budgets[subject]
	if !ok {
		return -1
	}
	return v
}

// Consume deducts n tokens from the subject's budget.
// Returns an error if the budget would go negative.
func (e *TokenBudgetEnforcer) Consume(subject string, n int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	remaining, ok := e.budgets[subject]
	if !ok {
		return nil // no budget configured — pass through
	}
	if remaining < n {
		return fmt.Errorf("token budget exhausted: %d requested, %d remaining", n, remaining)
	}
	e.budgets[subject] = remaining - n
	return nil
}

// EnforceBudget is HTTP middleware that gates requests against the token budget
// declared in the JWT. It must run after Authenticate (claims must be in ctx).
//
// The middleware only blocks if:
//  1. Claims are present in context, AND
//  2. TokenBudget > 0 (budget was declared), AND
//  3. The subject has exhausted their budget.
//
// Token consumption is recorded by the inference handler itself (which knows
// actual token counts). This middleware enforces the gate on entry.
func (e *TokenBudgetEnforcer) EnforceBudget(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok || claims.TokenBudget == 0 {
			// No claims or no budget declared — pass through.
			next.ServeHTTP(w, r)
			return
		}

		subject := claims.Subject
		if subject == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Initialise budget on first request for this subject.
		e.mu.Lock()
		if _, exists := e.budgets[subject]; !exists {
			e.budgets[subject] = claims.TokenBudget
		}
		remaining := e.budgets[subject]
		e.mu.Unlock()

		if remaining <= 0 {
			span := trace.SpanFromContext(r.Context())
			span.AddEvent("ckodex.budget.exhausted", trace.WithAttributes(
				attribute.String("subject", subject),
				attribute.Int64("tokens_remaining", remaining),
			))
			http.Error(w, `{"error":"rate_limited","message":"token budget exhausted"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RecordConsumption deducts actual token usage after a completed inference request.
// Intended to be called by the inference proxy handler once it knows the token count.
func (e *TokenBudgetEnforcer) RecordConsumption(ctx context.Context, tokensUsed int64) {
	claims, ok := ClaimsFromContext(ctx)
	if !ok || claims.TokenBudget == 0 || claims.Subject == "" {
		return
	}
	_ = e.Consume(claims.Subject, tokensUsed) // best-effort; over-spend logged elsewhere
}

// --- Model Access Enforcer ---

// EnforceModelAccess is HTTP middleware that gates requests to a specific model
// against the ModelAccess list in the JWT claims.
//
// modelExtractor is a function that returns the model name from the request
// (e.g. from the URL path segment or JSON body). If it returns "" enforcement
// is skipped.
func EnforceModelAccess(modelExtractor func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				// No claims — auth middleware should have already rejected this.
				next.ServeHTTP(w, r)
				return
			}

			model := modelExtractor(r)
			if model == "" {
				// Cannot determine model; pass through to let the backend reject.
				next.ServeHTTP(w, r)
				return
			}

			if !claims.CanAccessModel(model) {
				http.Error(w, `{"error":"forbidden","message":"model access denied: `+model+`"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
