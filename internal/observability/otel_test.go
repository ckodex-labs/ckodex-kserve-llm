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
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// setupTestMeterProvider registers a no-op SDK meter provider globally and
// restores the previous provider after the test.
func setupTestMeterProvider(t *testing.T) {
	t.Helper()
	prev := otel.GetMeterProvider()
	mp := sdkmetric.NewMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		_ = mp.Shutdown(context.Background())
		otel.SetMeterProvider(prev)
	})
}

// ---- NewInstrumentation -------------------------------------------------------

func TestNewInstrumentation_NoError(t *testing.T) {
	setupTestMeterProvider(t)
	inst, err := observability.NewInstrumentation()
	require.NoError(t, err)
	assert.NotNil(t, inst)
	assert.NotNil(t, inst.Tracer)
	assert.NotNil(t, inst.Meter)
	assert.NotNil(t, inst.ReconcileDuration)
	assert.NotNil(t, inst.ReconcileCount)
	assert.NotNil(t, inst.ActiveModels)
	assert.NotNil(t, inst.InferenceRequests)
	assert.NotNil(t, inst.TokensPerSecond)
	assert.NotNil(t, inst.QueueDepth)
	assert.NotNil(t, inst.GPUUtilization)
	assert.NotNil(t, inst.TokensConsumed)
	assert.NotNil(t, inst.ActiveGPUSeconds)
}

// ---- RecordReconcile ---------------------------------------------------------

func TestRecordReconcile_Success_NoPanic(t *testing.T) {
	setupTestMeterProvider(t)
	inst, err := observability.NewInstrumentation()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		inst.RecordReconcile(context.Background(), "llama3", 0.15, true)
	})
}

func TestRecordReconcile_Failure_NoPanic(t *testing.T) {
	setupTestMeterProvider(t)
	inst, err := observability.NewInstrumentation()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		inst.RecordReconcile(context.Background(), "llama3", 0.42, false)
	})
}

// ---- RecordTokensConsumed ----------------------------------------------------

func TestRecordTokensConsumed_NoPanic(t *testing.T) {
	setupTestMeterProvider(t)
	inst, err := observability.NewInstrumentation()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		inst.RecordTokensConsumed(context.Background(), "acme", "llama3", 100, 200,
			map[string]string{"team": "ml-platform", "project": "chatbot"})
	})
}

func TestRecordTokensConsumed_EmptyCostTags_NoPanic(t *testing.T) {
	setupTestMeterProvider(t)
	inst, err := observability.NewInstrumentation()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		inst.RecordTokensConsumed(context.Background(), "acme", "llama3", 50, 50, nil)
	})
}

// ---- RecordGPUUtilization ---------------------------------------------------

func TestRecordGPUUtilization_NoPanic(t *testing.T) {
	setupTestMeterProvider(t)
	inst, err := observability.NewInstrumentation()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		inst.RecordGPUUtilization(context.Background(), "acme", "llama3", 0.75,
			map[string]string{"cost_center": "ml"})
	})
}

func TestRecordGPUUtilization_Boundaries_NoPanic(t *testing.T) {
	setupTestMeterProvider(t)
	inst, err := observability.NewInstrumentation()
	require.NoError(t, err)

	for _, util := range []float64{0.0, 0.5, 1.0} {
		assert.NotPanics(t, func() {
			inst.RecordGPUUtilization(context.Background(), "t1", "m1", util, nil)
		})
	}
}

// ---- RecordActiveGPUSeconds -------------------------------------------------

func TestRecordActiveGPUSeconds_NoPanic(t *testing.T) {
	setupTestMeterProvider(t)
	inst, err := observability.NewInstrumentation()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		inst.RecordActiveGPUSeconds(context.Background(), "acme", "llama3", 15.0,
			map[string]string{"project": "search"})
	})
}
