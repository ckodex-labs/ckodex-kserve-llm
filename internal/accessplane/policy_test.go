/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package accessplane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluator_EvaluateContextHonorsCallerDeadline(t *testing.T) {
	evaluator := newTestEvaluator(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	decision, err := evaluator.EvaluateContext(
		ctx,
		Intent{TenantID: "tenant-a", Route: "balanced"},
		RuntimeObservation{},
	)

	assert.Equal(t, Decision{}, decision)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEvaluator_ForwardPolicySelectsFirstEligibleTarget(t *testing.T) {
	evaluator := newTestEvaluator(t)
	observed := RuntimeObservation{
		Tenant: TenantObservation{},
		Models: map[string]ModelObservation{
			"light-model": {Ready: true, InFlight: 2},
			"large-model": {Ready: true, InFlight: 1},
		},
	}

	decision := evaluator.Evaluate(Intent{TenantID: "tenant-a", Route: "balanced"}, observed)

	assert.Equal(t, DispositionAdmit, decision.Disposition)
	assert.Equal(t, "large-model", decision.Model)
	assert.Equal(t, "target-eligible", decision.Reason)
}

func TestEvaluator_ReverseRuntimeObservationChangesAdmission(t *testing.T) {
	evaluator := newTestEvaluator(t)
	intent := Intent{TenantID: "tenant-a", Route: "balanced"}
	observed := RuntimeObservation{
		Tenant: TenantObservation{InFlight: 1, QueueDepth: 0},
		Models: map[string]ModelObservation{
			"light-model": {Ready: true, InFlight: 0},
			"large-model": {Ready: true, InFlight: 0},
		},
	}
	original := observed

	admitted := evaluator.Evaluate(intent, observed)
	assert.Equal(t, DispositionAdmit, admitted.Disposition)

	observed.Tenant = TenantObservation{InFlight: 3, QueueDepth: 1}
	beforeBackpressure := observed
	backpressured := evaluator.Evaluate(intent, observed)
	assert.Equal(t, DispositionBackpressure, backpressured.Disposition)
	assert.Equal(t, "tenant-concurrency-limit", backpressured.Reason)
	assert.Equal(t, 1, backpressured.QueueCapacityRemaining)
	assert.Equal(t, beforeBackpressure, observed, "evaluation must not claim queue ownership by mutating observations")

	observed.Tenant.QueueDepth = 2
	rejected := evaluator.Evaluate(intent, observed)
	assert.Equal(t, DispositionReject, rejected.Disposition)
	assert.Equal(t, "queue-capacity-exhausted", rejected.Reason)
	assert.Equal(t, original.Models, observed.Models, "evaluation must not mutate model observations")
}

func TestEvaluator_BackpressuresWhenNoTargetIsEligible(t *testing.T) {
	evaluator := newTestEvaluator(t)
	observed := RuntimeObservation{
		Tenant: TenantObservation{QueueDepth: 0},
		Models: map[string]ModelObservation{
			"light-model": {Ready: false},
			"large-model": {Ready: true, InFlight: 4},
		},
	}

	decision := evaluator.Evaluate(Intent{TenantID: "tenant-a", Route: "balanced"}, observed)

	assert.Equal(t, DispositionBackpressure, decision.Disposition)
	assert.Equal(t, "no-eligible-target", decision.Reason)
	assert.Equal(t, 2, decision.QueueCapacityRemaining)
}

func TestEvaluator_RejectsRequestsOutsideTenantBoundary(t *testing.T) {
	evaluator := newTestEvaluator(t)

	unknownTenant := evaluator.Evaluate(Intent{TenantID: "tenant-b", Route: "balanced"}, RuntimeObservation{})
	unknownRoute := evaluator.Evaluate(Intent{TenantID: "tenant-a", Route: "restricted"}, RuntimeObservation{})

	assert.Equal(t, Decision{Disposition: DispositionReject, Reason: "tenant-not-admitted"}, unknownTenant)
	assert.Equal(t, Decision{Disposition: DispositionReject, Reason: "route-not-admitted"}, unknownRoute)
}

func TestEvaluator_RejectsInvalidRuntimeObservation(t *testing.T) {
	evaluator := newTestEvaluator(t)
	observed := RuntimeObservation{Tenant: TenantObservation{QueueDepth: -1}}

	decision := evaluator.Evaluate(Intent{TenantID: "tenant-a", Route: "balanced"}, observed)

	assert.Equal(t, Decision{Disposition: DispositionReject, Reason: "invalid-runtime-observation"}, decision)
}

func TestNewEvaluator_ValidatesPolicyAndCopiesRoutes(t *testing.T) {
	policy := testTenantPolicy()
	evaluator, err := NewEvaluator([]TenantPolicy{policy})
	require.NoError(t, err)

	policy.Routes["balanced"] = RoutePolicy{Targets: []TargetPolicy{{Model: "mutated", MaxInFlight: 1}}}
	decision := evaluator.Evaluate(Intent{TenantID: "tenant-a", Route: "balanced"}, RuntimeObservation{
		Models: map[string]ModelObservation{"light-model": {Ready: true}},
	})
	assert.Equal(t, "light-model", decision.Model)

	_, err = NewEvaluator([]TenantPolicy{testTenantPolicy(), testTenantPolicy()})
	assert.EqualError(t, err, `duplicate tenant policy "tenant-a"`)
}

func newTestEvaluator(t *testing.T) *Evaluator {
	t.Helper()
	evaluator, err := NewEvaluator([]TenantPolicy{testTenantPolicy()})
	require.NoError(t, err)
	return evaluator
}

func testTenantPolicy() TenantPolicy {
	return TenantPolicy{
		TenantID:      "tenant-a",
		MaxInFlight:   3,
		MaxQueueDepth: 2,
		Routes: map[string]RoutePolicy{
			"balanced": {
				Targets: []TargetPolicy{
					{Model: "light-model", MaxInFlight: 2},
					{Model: "large-model", MaxInFlight: 4},
				},
			},
		},
	}
}
