/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package vectorstate provides bidirectional conformance tests for the CKODEX
// vector state forbidden-tuple invariants defined in CKODEX §10.
//
// Spec reference:
//
//	Forbidden tuples (immediate HALT):
//	  anti_execute              — anti ∧ execute
//	  active_untrusted          — Active ∧ trust < Verified
//	  negative_escalation_skipped — negative ∧ escalation_skipped
//	  empty_high_dal            — empty ∧ DAL ≥ 3
//
// Each test verifies one of:
//  1. The tuple is structurally unreachable (spec_to_runtime direction).
//  2. The tuple is detected and the ForbiddenTupleCounter is incremented
//     with the correct attribute value (runtime_to_spec direction).
package vectorstate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/ckodex-labs/kserve-llm-operator/internal/auth"
	"github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// tupleType values must match those documented in otel.go and CKODEX §10.
const (
	tupleAntiExecute               = "anti_execute"
	tupleActiveTrusted             = "active_untrusted"
	tupleNegativeEscalationSkipped = "negative_escalation_skipped"
	tupleEmptyHighDAL              = "empty_high_dal"
)

// allForbiddenTuples is the canonical list from CKODEX §10.
// Any addition to the spec must be reflected here — this is the
// runtime_to_spec direction of the conformance vector.
var allForbiddenTuples = []string{
	tupleAntiExecute,
	tupleActiveTrusted,
	tupleNegativeEscalationSkipped,
	tupleEmptyHighDAL,
}

// newTestInstrumentation creates an Instrumentation backed by an in-process
// OTel metric reader so counter values can be asserted in tests.
func newTestInstrumentation(t *testing.T) (*observability.Instrumentation, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	// Register the global provider so NewInstrumentation picks it up.
	// Restore the original on cleanup.
	orig := observability.SetMeterProvider(provider)
	t.Cleanup(func() { observability.SetMeterProvider(orig) })

	inst, err := observability.NewInstrumentation()
	require.NoError(t, err)
	return inst, reader
}

// collectCounterValue reads the current sum of the named counter from the
// ManualReader, filtered to a specific tuple_type attribute value.
func collectCounterValue(t *testing.T, reader *sdkmetric.ManualReader, tupleType string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "ckodex.vector.forbidden_tuple" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				v, ok := dp.Attributes.Value(attribute.Key("tuple_type"))
				if ok && v.AsString() == tupleType {
					return dp.Value
				}
			}
		}
	}
	return 0
}

// --- Spec-to-runtime: ForbiddenTupleCounter covers all four tuple types ---

// TestForbiddenTupleCounter_AllTupleTypesAreCountable verifies that for every
// tuple type in the spec, the counter can be incremented (attribute routing
// works). This is the spec_to_runtime direction: the spec lists four types;
// the runtime must be able to record each.
func TestForbiddenTupleCounter_AllTupleTypesAreCountable(t *testing.T) {
	inst, reader := newTestInstrumentation(t)

	for _, tt := range allForbiddenTuples {
		t.Run(tt, func(t *testing.T) {
			inst.ForbiddenTupleCounter.Add(context.Background(), 1,
				observability.TupleTypeAttr(tt))

			got := collectCounterValue(t, reader, tt)
			assert.GreaterOrEqual(t, got, int64(1),
				"counter for tuple_type=%q must be ≥ 1 after Add", tt)
		})
	}
}

// --- Runtime-to-spec: counter name and attribute key match the spec ---

// TestForbiddenTupleCounter_MetricNameMatchesSpec verifies the OTel metric
// name is exactly "ckodex.vector.forbidden_tuple" as specified in §10 and
// otel.go. A rename would break dashboards and alert rules.
func TestForbiddenTupleCounter_MetricNameMatchesSpec(t *testing.T) {
	inst, reader := newTestInstrumentation(t)

	inst.ForbiddenTupleCounter.Add(context.Background(), 1,
		observability.TupleTypeAttr(tupleAntiExecute))

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "ckodex.vector.forbidden_tuple" {
				found = true
			}
		}
	}
	assert.True(t, found, "metric 'ckodex.vector.forbidden_tuple' must be present in collected metrics")
}

// TestForbiddenTupleCounter_AttributeKeyIsTupleType verifies the attribute key
// is "tuple_type". Dashboards and alert rules depend on this key name.
func TestForbiddenTupleCounter_AttributeKeyIsTupleType(t *testing.T) {
	inst, reader := newTestInstrumentation(t)

	inst.ForbiddenTupleCounter.Add(context.Background(), 3,
		observability.TupleTypeAttr(tupleEmptyHighDAL))

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "ckodex.vector.forbidden_tuple" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, dp := range sum.DataPoints {
				_, keyPresent := dp.Attributes.Value(attribute.Key("tuple_type"))
				assert.True(t, keyPresent, "data point must carry 'tuple_type' attribute")
				return
			}
		}
	}
	t.Fatal("no data points found for ckodex.vector.forbidden_tuple")
}

// --- Structural unreachability proofs ---

// TestForbiddenTuple_AntiExecute_Unreachable_ViaAuthMiddleware verifies that
// the auth middleware (oidc_middleware.go) always denies the request before
// reaching the handler when a scope check fails — i.e., execute never happens
// in the anti (denied) state. This is a structural proof: the middleware
// returns 403 and does NOT call next.ServeHTTP.
func TestForbiddenTuple_AntiExecute_Unreachable_ViaAuthMiddleware(t *testing.T) {
	inst, reader := newTestInstrumentation(t)
	cfg := auth.OIDCConfig{
		ClientID:       "conformance-secret",
		Audience:       "conformance",
		RequiredScopes: []string{"admin"},
		CacheTTL:       time.Hour,
	}
	claims := &auth.InferenceClaims{
		RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"conformance"}},
		Scope:            "inference",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(cfg.ClientID))
	require.NoError(t, err)

	called := false
	handler := auth.NewMiddleware(cfg).WithInstrumentation(inst).Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	assert.False(t, called, "a denied request must not reach execution")
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, int64(1), collectCounterValue(t, reader, tupleAntiExecute))
}

// TestForbiddenTuple_EmptyHighDAL_Unreachable_ViaStartupValidation verifies
// that the operator exits at startup if required credentials are absent,
// ensuring DAL ≥ 3 operations (cross-boundary storage pulls) cannot proceed
// in an empty (unconfigured) state.
func TestForbiddenTuple_EmptyHighDAL_Unreachable_ViaStartupValidation(t *testing.T) {
	t.Setenv("VAULT_PATH", "secret/data/models")
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")
	require.Error(t, config.ValidateStorageCredentials(&config.OperatorConfig{}))
}
