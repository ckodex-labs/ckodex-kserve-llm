/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package residency

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectObservedStateToStatusAndPolicy(t *testing.T) {
	tests := []struct {
		name       string
		state      State
		observed   Observation
		wantReason string
		wantReady  bool
		wantRoute  bool
		wantAccept bool
	}{
		{name: "cold", state: StateCold, wantReason: "ArtifactAbsent"},
		{name: "cached", state: StateCached, observed: Observation{ArtifactCached: true}, wantReason: "ArtifactCached"},
		{name: "loading", state: StateLoading, observed: Observation{ArtifactCached: true}, wantReason: "RuntimeLoading"},
		{name: "warming", state: StateWarming, observed: Observation{ArtifactCached: true, RuntimeLoaded: true}, wantReason: "RuntimeWarming"},
		{
			name: "ready", state: StateReady,
			observed:   Observation{ArtifactCached: true, RuntimeLoaded: true, RuntimeReady: true, PrefillReady: true},
			wantReason: "ResidentReady", wantReady: true, wantRoute: true, wantAccept: true,
		},
		{
			name: "draining", state: StateDraining,
			observed:   Observation{ArtifactCached: true, RuntimeLoaded: true, RuntimeReady: true, ActiveRequests: 1},
			wantReason: "RequestsDraining",
		},
		{name: "evicting", state: StateEvicting, observed: Observation{ArtifactCached: true}, wantReason: "ResidencyEvicting"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, policy, err := Project(tt.state, tt.observed, true)
			require.NoError(t, err)
			assert.Equal(t, tt.wantReason, status.Reason)
			assert.Equal(t, tt.wantReady, status.Ready)
			assert.Equal(t, tt.wantRoute, policy.AllowRoute)
			assert.Equal(t, tt.wantAccept, policy.AcceptNewRequests)
		})
	}
}

func TestProjectReadyRequiresObservedPrefillForPD(t *testing.T) {
	status, policy, err := Project(StateReady, Observation{RuntimeLoaded: true, RuntimeReady: true}, true)
	require.NoError(t, err)
	assert.False(t, status.Ready)
	assert.Equal(t, "PrefillUnavailable", status.Reason)
	assert.False(t, policy.AllowRoute)
}

func TestProjectRejectsOrphanRouteAndRequestObservations(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		observed Observation
	}{
		{name: "route without runtime", state: StateWarming, observed: Observation{RouteAttached: true}},
		{name: "request without route", state: StateReady, observed: Observation{RuntimeReady: true, AcceptingRequests: true}},
		{
			name: "request while draining", state: StateDraining,
			observed: Observation{RuntimeReady: true, RouteAttached: true, AcceptingRequests: true},
		},
		{name: "active request while evicting", state: StateEvicting, observed: Observation{ActiveRequests: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, policy, err := Project(tt.state, tt.observed, false)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvariantViolated)
			assert.Equal(t, "InvariantViolation", status.Reason)
			assert.False(t, policy.AcceptNewRequests)
		})
	}
}
