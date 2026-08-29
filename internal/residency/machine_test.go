/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package residency

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvanceDesiredReadyMovesColdThroughResidencyPhases(t *testing.T) {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	plan := Plan{Target: StateReady, RequirePrefill: true, Timeout: time.Minute}
	current := Snapshot{State: StateCold, RollbackTarget: StateCold}

	decision, err := Advance(now, current, plan, Observation{})
	require.NoError(t, err)
	assert.Equal(t, StateCold, decision.Snapshot.State)
	assert.Equal(t, []Action{ActionCacheArtifact}, decision.Actions)

	decision, err = Advance(now, current, plan, Observation{ArtifactCached: true})
	require.NoError(t, err)
	assert.Equal(t, StateCached, decision.Snapshot.State)

	decision, err = Advance(now, decision.Snapshot, plan, Observation{ArtifactCached: true})
	require.NoError(t, err)
	assert.Equal(t, StateLoading, decision.Snapshot.State)
	assert.Equal(t, []Action{ActionLoadRuntime}, decision.Actions)

	loaded := Observation{ArtifactCached: true, RuntimeLoaded: true}
	decision, err = Advance(now.Add(time.Second), decision.Snapshot, plan, loaded)
	require.NoError(t, err)
	assert.Equal(t, StateWarming, decision.Snapshot.State)

	warming := Observation{ArtifactCached: true, RuntimeLoaded: true, RuntimeReady: true}
	decision, err = Advance(now.Add(2*time.Second), decision.Snapshot, plan, warming)
	require.NoError(t, err)
	assert.Equal(t, StateWarming, decision.Snapshot.State, "prefill readiness is a separate P/D gate")

	warming.PrefillReady = true
	decision, err = Advance(now.Add(3*time.Second), decision.Snapshot, plan, warming)
	require.NoError(t, err)
	assert.Equal(t, StateReady, decision.Snapshot.State)
	assert.Equal(t, []Action{ActionAttachRoute}, decision.Actions)
	assert.True(t, decision.Status.Ready)
}

func TestAdvanceDrainPreventsOrphanRequestsBeforeEviction(t *testing.T) {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	plan := Plan{Target: StateCold, Timeout: time.Minute}
	current := Snapshot{State: StateReady, RollbackTarget: StateReady}
	ready := Observation{ArtifactCached: true, RuntimeLoaded: true, RuntimeReady: true, RouteAttached: true, AcceptingRequests: true}

	decision, err := Advance(now, current, plan, ready)
	require.NoError(t, err)
	assert.Equal(t, StateDraining, decision.Snapshot.State)
	assert.Equal(t, []Action{ActionDetachRoute}, decision.Actions)
	assert.False(t, decision.Policy.AcceptNewRequests)
	assert.Equal(t, "RequestsDraining", decision.Status.Reason)

	draining := Observation{ArtifactCached: true, RuntimeLoaded: true, RuntimeReady: true, ActiveRequests: 2}
	decision, err = Advance(now.Add(time.Second), decision.Snapshot, plan, draining)
	require.NoError(t, err)
	assert.Equal(t, StateDraining, decision.Snapshot.State)
	assert.Equal(t, []Action{ActionWaitForDrain}, decision.Actions)

	draining.ActiveRequests = 0
	decision, err = Advance(now.Add(2*time.Second), decision.Snapshot, plan, draining)
	require.NoError(t, err)
	assert.Equal(t, StateEvicting, decision.Snapshot.State)
	assert.Equal(t, []Action{ActionStopRuntime}, decision.Actions)
}

func TestAdvanceTimeoutCompensatesLoadingTransaction(t *testing.T) {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	current := Snapshot{
		State: StateLoading, RollbackTarget: StateCached,
		StartedAt: now.Add(-time.Minute), Deadline: now,
	}
	observed := Observation{ArtifactCached: true, RuntimeLoaded: true}

	decision, err := Advance(now, current, Plan{Target: StateReady}, observed)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTransitionTimeout)
	assert.Equal(t, StateEvicting, decision.Snapshot.State)
	assert.Equal(t, []Action{ActionStopRuntime}, decision.Actions)
	assert.False(t, decision.Policy.AllowRoute)
}

func TestAdvanceRecoversInterruptedTransactionsFromObservedState(t *testing.T) {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	plan := Plan{Target: StateReady, RequirePrefill: true}
	interruptedLoad := Snapshot{State: StateLoading, RollbackTarget: StateCached, Deadline: now.Add(time.Minute)}
	loaded := Observation{ArtifactCached: true, RuntimeLoaded: true}

	decision, err := Advance(now, interruptedLoad, plan, loaded)
	require.NoError(t, err)
	assert.Equal(t, StateWarming, decision.Snapshot.State)

	interruptedDrain := Snapshot{State: StateDraining, RollbackTarget: StateReady, Deadline: now.Add(time.Minute)}
	drained := Observation{ArtifactCached: true, RuntimeLoaded: true, RuntimeReady: true}
	decision, err = Advance(now, interruptedDrain, Plan{Target: StateCold}, drained)
	require.NoError(t, err)
	assert.Equal(t, StateEvicting, decision.Snapshot.State)
	assert.Equal(t, []Action{ActionStopRuntime}, decision.Actions)
}

func TestAdvanceFailureRollsBackToCachedAfterRuntimeStops(t *testing.T) {
	current := Snapshot{State: StateWarming, RollbackTarget: StateCached}
	observed := Observation{ArtifactCached: true, Failure: "readiness probe failed"}

	decision, err := Advance(time.Now(), current, Plan{Target: StateReady}, observed)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTransitionFailed))
	assert.Equal(t, StateCached, decision.Snapshot.State)
	assert.Empty(t, decision.Actions)
}

func TestAdvanceInterruptedLoadIsCancelledBeforeEviction(t *testing.T) {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	current := Snapshot{State: StateLoading, RollbackTarget: StateCached}

	decision, err := Advance(now, current, Plan{Target: StateCold}, Observation{ArtifactCached: true})
	require.NoError(t, err)
	assert.Equal(t, StateEvicting, decision.Snapshot.State)
	assert.Equal(t, []Action{ActionStopRuntime}, decision.Actions)
}
