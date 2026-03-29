/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- DegradationController ---------------------------------------------------

func TestDegradationController_NewController_LevelNone(t *testing.T) {
	dc := NewDegradationController()
	assert.Equal(t, DegradationNone, dc.Level())
}

func TestDegradationController_Evaluate_HighQueue_SetsLevel(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(200, 1*time.Second) // depth=200 > Light threshold (100)
	assert.Equal(t, DegradationLight, dc.Level())
}

func TestDegradationController_Evaluate_ModerateQueue_SetsModerate(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(600, 1*time.Second) // depth=600 > Moderate threshold (500)
	assert.Equal(t, DegradationModerate, dc.Level())
}

func TestDegradationController_Evaluate_SevereQueue_SetsSevere(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(1500, 1*time.Second) // depth=1500 > Severe threshold (1000)
	assert.Equal(t, DegradationSevere, dc.Level())
}

func TestDegradationController_Evaluate_HighP99_SetsLevel(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(0, 10*time.Second) // p99=10s > Light threshold (5s)
	assert.Equal(t, DegradationLight, dc.Level())
}

func TestDegradationController_Evaluate_NormalLoad_LevelNone(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(10, 100*time.Millisecond) // below all thresholds
	assert.Equal(t, DegradationNone, dc.Level())
}

func TestDegradationController_ActiveRule_NoneLevel_ReturnsNil(t *testing.T) {
	dc := NewDegradationController()
	rule := dc.ActiveRule()
	assert.Nil(t, rule)
}

func TestDegradationController_ActiveRule_LightLevel_ReturnsRule(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(200, 1*time.Second)
	rule := dc.ActiveRule()
	require.NotNil(t, rule)
	assert.Equal(t, DegradationLight, rule.Level)
	assert.Equal(t, int32(2048), rule.MaxTokensOverride)
}

func TestDegradationController_ActiveRule_SevereLevel_ReturnsRule(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(1500, 1*time.Second)
	rule := dc.ActiveRule()
	require.NotNil(t, rule)
	assert.Equal(t, DegradationSevere, rule.Level)
	assert.True(t, rule.RejectBatch)
}

func TestDegradationController_ClampTokens_NoRule_ReturnsRequested(t *testing.T) {
	dc := NewDegradationController()
	// Level=None → no active rule
	clamped := dc.ClampTokens(4096)
	assert.Equal(t, int32(4096), clamped)
}

func TestDegradationController_ClampTokens_LightLevel_ClampsHigh(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(200, 1*time.Second) // Light level, MaxTokensOverride=2048
	clamped := dc.ClampTokens(4096)
	assert.Equal(t, int32(2048), clamped)
}

func TestDegradationController_ClampTokens_LightLevel_BelowOverride_Unchanged(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(200, 1*time.Second) // Light: MaxTokensOverride=2048
	clamped := dc.ClampTokens(512)
	assert.Equal(t, int32(512), clamped)
}

func TestDegradationController_ShouldRejectBatch_NoneLevel_False(t *testing.T) {
	dc := NewDegradationController()
	assert.False(t, dc.ShouldRejectBatch())
}

func TestDegradationController_ShouldRejectBatch_LightLevel_False(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(200, 1*time.Second) // Light: RejectBatch=false
	assert.False(t, dc.ShouldRejectBatch())
}

func TestDegradationController_ShouldRejectBatch_ModerateLevel_True(t *testing.T) {
	dc := NewDegradationController()
	dc.Evaluate(600, 1*time.Second) // Moderate: RejectBatch=true
	assert.True(t, dc.ShouldRejectBatch())
}

// ---- AdaptiveTimeout: P95/P99 ------------------------------------------------

func TestAdaptiveTimeout_P95_P99_AfterSamples(t *testing.T) {
	a := NewAdaptiveTimeout(200, 50*time.Millisecond, 5*time.Second)
	// Load 200 samples (triggers recompute every 100)
	for i := 0; i < 200; i++ {
		a.Record(time.Duration(i+1) * time.Millisecond)
	}
	p95 := a.P95()
	p99 := a.P99()
	assert.True(t, p95 > 0, "P95 should be positive after samples")
	assert.True(t, p99 >= p95, "P99 should be >= P95")
}
