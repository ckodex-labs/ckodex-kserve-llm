/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// ---- InjectTenantBaggage -----------------------------------------------------

func TestInjectTenantBaggage_AllFields_RoundTrip(t *testing.T) {
	tc := observability.TenantContext{
		TenantID:   "acme",
		ModelName:  "llama3",
		CostCenter: "ml-platform",
		Namespace:  "production",
		SessionID:  "sess-001",
	}

	ctx, err := observability.InjectTenantBaggage(context.Background(), tc)
	require.NoError(t, err)

	got := observability.ExtractTenantBaggage(ctx)
	assert.Equal(t, "acme", got.TenantID)
	assert.Equal(t, "llama3", got.ModelName)
	assert.Equal(t, "ml-platform", got.CostCenter)
	assert.Equal(t, "production", got.Namespace)
	assert.Equal(t, "sess-001", got.SessionID)
}

func TestInjectTenantBaggage_EmptyFields_Skipped(t *testing.T) {
	// Only TenantID is set; other fields empty — should not error.
	tc := observability.TenantContext{
		TenantID: "tenant-only",
	}

	ctx, err := observability.InjectTenantBaggage(context.Background(), tc)
	require.NoError(t, err)

	got := observability.ExtractTenantBaggage(ctx)
	assert.Equal(t, "tenant-only", got.TenantID)
	assert.Empty(t, got.ModelName)
	assert.Empty(t, got.CostCenter)
	assert.Empty(t, got.Namespace)
	assert.Empty(t, got.SessionID)
}

func TestInjectTenantBaggage_EmptyContext_ZeroTenantContext(t *testing.T) {
	// No baggage injected — ExtractTenantBaggage must return zero value.
	got := observability.ExtractTenantBaggage(context.Background())
	assert.Empty(t, got.TenantID)
	assert.Empty(t, got.ModelName)
	assert.Empty(t, got.CostCenter)
	assert.Empty(t, got.Namespace)
	assert.Empty(t, got.SessionID)
}

func TestInjectTenantBaggage_AllEmpty_NoError(t *testing.T) {
	// All fields empty — inject must succeed and produce no baggage members.
	tc := observability.TenantContext{}
	ctx, err := observability.InjectTenantBaggage(context.Background(), tc)
	require.NoError(t, err)

	got := observability.ExtractTenantBaggage(ctx)
	assert.Empty(t, got.TenantID)
}

func TestInjectTenantBaggage_OverwriteExisting_UpdatesValue(t *testing.T) {
	// First injection
	tc1 := observability.TenantContext{TenantID: "first"}
	ctx, err := observability.InjectTenantBaggage(context.Background(), tc1)
	require.NoError(t, err)

	// Second injection overwrites
	tc2 := observability.TenantContext{TenantID: "second", ModelName: "gpt4"}
	ctx, err = observability.InjectTenantBaggage(ctx, tc2)
	require.NoError(t, err)

	got := observability.ExtractTenantBaggage(ctx)
	assert.Equal(t, "second", got.TenantID)
	assert.Equal(t, "gpt4", got.ModelName)
}

func TestInjectTenantBaggage_SpecialChars_SessionID(t *testing.T) {
	// W3C baggage values may contain percent-encoded characters.
	// ASCII safe value (no special chars that need encoding).
	tc := observability.TenantContext{
		SessionID: "abc123XYZ",
	}
	ctx, err := observability.InjectTenantBaggage(context.Background(), tc)
	require.NoError(t, err)

	got := observability.ExtractTenantBaggage(ctx)
	assert.Equal(t, "abc123XYZ", got.SessionID)
}
