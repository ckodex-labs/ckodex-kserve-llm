/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package lifecycle_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/kernel/lifecycle"
)

func TestValidateState(t *testing.T) {
	tests := []struct {
		state   lifecycle.State
		wantErr bool
	}{
		{lifecycle.StateQuarantined, false},
		{lifecycle.StateObserved, false},
		{lifecycle.StateBenchmarked, false},
		{lifecycle.StateGoverned, false},
		{lifecycle.StatePromoted, false},
		{lifecycle.StateWarming, false},
		{lifecycle.StateActive, false},
		{lifecycle.StateDraining, false},
		{lifecycle.StateRetired, false},
		{lifecycle.State("UNKNOWN"), true},
		{lifecycle.State(""), true},
	}

	for _, tt := range tests {
		err := lifecycle.ValidateState(tt.state)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateState(%q) error = %v, wantErr = %v", tt.state, err, tt.wantErr)
		}
		if tt.wantErr && !errors.Is(err, lifecycle.ErrInvalidState) {
			t.Errorf("expected ErrInvalidState, got %v", err)
		}
	}
}

func TestNewStateVector(t *testing.T) {
	t.Run("valid initialization", func(t *testing.T) {
		vec, err := lifecycle.NewStateVector(
			lifecycle.StateObserved,
			1,
			lifecycle.ConformancePending,
			lifecycle.ReadinessCold,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vec.CurrentState != lifecycle.StateObserved {
			t.Errorf("got current state %s, want %s", vec.CurrentState, lifecycle.StateObserved)
		}
		if vec.Epoch != 1 {
			t.Errorf("got epoch %d, want 1", vec.Epoch)
		}
		if vec.TransitionCount != 0 {
			t.Errorf("got transition count %d, want 0", vec.TransitionCount)
		}
		if vec.Conformance != lifecycle.ConformancePending {
			t.Errorf("got conformance %s, want %s", vec.Conformance, lifecycle.ConformancePending)
		}
		if vec.Readiness != lifecycle.ReadinessCold {
			t.Errorf("got readiness %s, want %s", vec.Readiness, lifecycle.ReadinessCold)
		}
	})

	t.Run("default dimension derivation", func(t *testing.T) {
		vec, err := lifecycle.NewStateVector(lifecycle.StateActive, 0, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vec.Conformance != lifecycle.ConformanceConformant {
			t.Errorf("expected default ConformanceConformant, got %s", vec.Conformance)
		}
		if vec.Readiness != lifecycle.ReadinessReady {
			t.Errorf("expected default ReadinessReady, got %s", vec.Readiness)
		}
	})

	t.Run("invalid initial state", func(t *testing.T) {
		_, err := lifecycle.NewStateVector("INVALID", 1, "", "")
		if err == nil {
			t.Fatalf("expected error for invalid state, got nil")
		}
	})
}

func stepThrough(t *testing.T, v lifecycle.StateVector, target lifecycle.State, reason, ev string) lifecycle.StateVector {
	t.Helper()
	prev := v.CurrentState
	next, err := v.Transition(lifecycle.TransitionIntent{
		TargetState:    target,
		Reason:         reason,
		EvidenceDigest: ev,
		Timestamp:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("transition from %s to %s failed: %v", prev, target, err)
	}
	if next.CurrentState != target || next.PreviousState != prev {
		t.Fatalf("expected state %s (prev %s), got %s (prev %s)", target, prev, next.CurrentState, next.PreviousState)
	}
	return next
}

func TestValidTransitions(t *testing.T) {
	v, err := lifecycle.NewStateVector(lifecycle.StateQuarantined, 10, "", "")
	if err != nil {
		t.Fatalf("failed init: %v", err)
	}

	v = stepThrough(t, v, lifecycle.StateObserved, "quarantine cleared", "sha256:obs01")
	v = stepThrough(t, v, lifecycle.StateBenchmarked, "baseline latency verified", "sha256:bench01")
	v = stepThrough(t, v, lifecycle.StateGoverned, "attestation approved", "sha256:gov01")
	v = stepThrough(t, v, lifecycle.StatePromoted, "promoted to staging", "sha256:prom01")
	v = stepThrough(t, v, lifecycle.StateWarming, "initializing model weights", "sha256:warm01")
	v = stepThrough(t, v, lifecycle.StateActive, "health probes passing", "sha256:act01")
	v = stepThrough(t, v, lifecycle.StateDraining, "graceful teardown", "sha256:drain01")
	v = stepThrough(t, v, lifecycle.StateRetired, "drained and retired", "sha256:ret01")

	if !v.IsTerminal() {
		t.Errorf("expected retired state to be terminal")
	}
	if v.TransitionCount != 8 {
		t.Errorf("expected 8 transitions, got %d", v.TransitionCount)
	}
}

func allExpectedEdges() [][2]lifecycle.State {
	return [][2]lifecycle.State{
		{lifecycle.StateQuarantined, lifecycle.StateObserved},
		{lifecycle.StateQuarantined, lifecycle.StateRetired},
		{lifecycle.StateObserved, lifecycle.StateBenchmarked},
		{lifecycle.StateObserved, lifecycle.StateQuarantined},
		{lifecycle.StateObserved, lifecycle.StateRetired},
		{lifecycle.StateBenchmarked, lifecycle.StateGoverned},
		{lifecycle.StateBenchmarked, lifecycle.StateObserved},
		{lifecycle.StateBenchmarked, lifecycle.StateQuarantined},
		{lifecycle.StateBenchmarked, lifecycle.StateRetired},
		{lifecycle.StateGoverned, lifecycle.StatePromoted},
		{lifecycle.StateGoverned, lifecycle.StateObserved},
		{lifecycle.StateGoverned, lifecycle.StateQuarantined},
		{lifecycle.StateGoverned, lifecycle.StateRetired},
		{lifecycle.StatePromoted, lifecycle.StateWarming},
		{lifecycle.StatePromoted, lifecycle.StateActive},
		{lifecycle.StatePromoted, lifecycle.StateDraining},
		{lifecycle.StatePromoted, lifecycle.StateQuarantined},
		{lifecycle.StatePromoted, lifecycle.StateRetired},
		{lifecycle.StateWarming, lifecycle.StateActive},
		{lifecycle.StateWarming, lifecycle.StateDraining},
		{lifecycle.StateWarming, lifecycle.StateQuarantined},
		{lifecycle.StateWarming, lifecycle.StateRetired},
		{lifecycle.StateActive, lifecycle.StateDraining},
		{lifecycle.StateActive, lifecycle.StateWarming},
		{lifecycle.StateActive, lifecycle.StateQuarantined},
		{lifecycle.StateDraining, lifecycle.StateRetired},
		{lifecycle.StateDraining, lifecycle.StateActive},
		{lifecycle.StateDraining, lifecycle.StateQuarantined},
		{lifecycle.StateRetired, lifecycle.StateQuarantined},
	}
}

func TestLegalTransitionsAllEdges(t *testing.T) {
	for _, e := range allExpectedEdges() {
		if !lifecycle.CanTransition(e[0], e[1]) {
			t.Errorf("expected CanTransition(%s, %s) to be true", e[0], e[1])
		}
	}
}

func TestProhibitedTransitions(t *testing.T) {
	illegalPairs := []struct {
		from lifecycle.State
		to   lifecycle.State
	}{
		{lifecycle.StateObserved, lifecycle.StateActive},
		{lifecycle.StateObserved, lifecycle.StatePromoted},
		{lifecycle.StateActive, lifecycle.StateObserved},
		{lifecycle.StateActive, lifecycle.StateBenchmarked},
		{lifecycle.StateActive, lifecycle.StateRetired}, // must drain first
		{lifecycle.StateRetired, lifecycle.StateActive},
		{lifecycle.StateRetired, lifecycle.StateObserved},
		{lifecycle.StateQuarantined, lifecycle.StateActive},
		{lifecycle.StateQuarantined, lifecycle.StatePromoted},
		{lifecycle.StateBenchmarked, lifecycle.StateActive},
	}

	for _, p := range illegalPairs {
		if lifecycle.CanTransition(p.from, p.to) {
			t.Errorf("CanTransition(%s, %s) unexpectedly returned true", p.from, p.to)
		}

		vec, err := lifecycle.NewStateVector(p.from, 0, "", "")
		if err != nil {
			t.Fatalf("failed init: %v", err)
		}
		_, err = vec.Transition(lifecycle.TransitionIntent{
			TargetState: p.to,
			Reason:      "attempting prohibited jump",
		})
		if err == nil {
			t.Errorf("expected error transitioning from %s to %s, got nil", p.from, p.to)
		}
		if !errors.Is(err, lifecycle.ErrIllegalTransition) {
			t.Errorf("expected ErrIllegalTransition from %s to %s, got %v", p.from, p.to, err)
		}
	}
}

func TestTransitionIntentValidation(t *testing.T) {
	vec, err := lifecycle.NewStateVector(lifecycle.StateObserved, 1, "", "")
	if err != nil {
		t.Fatalf("failed init: %v", err)
	}

	t.Run("empty reason rejection", func(t *testing.T) {
		_, err := vec.Transition(lifecycle.TransitionIntent{
			TargetState: lifecycle.StateBenchmarked,
			Reason:      "",
		})
		if !errors.Is(err, lifecycle.ErrInvalidIntent) {
			t.Errorf("expected ErrInvalidIntent for empty reason, got %v", err)
		}
	})

	t.Run("invalid target state rejection", func(t *testing.T) {
		_, err := vec.Transition(lifecycle.TransitionIntent{
			TargetState: "BOGUS_STATE",
			Reason:      "valid reason",
		})
		if !errors.Is(err, lifecycle.ErrInvalidState) {
			t.Errorf("expected ErrInvalidState for bogus target, got %v", err)
		}
	})
}

func TestLegalTargetsAndTransitions(t *testing.T) {
	vec, err := lifecycle.NewStateVector(lifecycle.StateActive, 1, "", "")
	if err != nil {
		t.Fatalf("failed init: %v", err)
	}

	targets := vec.LegalTargets()
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets from ACTIVE, got %d", len(targets))
	}

	expectedMap := map[lifecycle.State]bool{
		lifecycle.StateDraining:    true,
		lifecycle.StateWarming:     true,
		lifecycle.StateQuarantined: true,
	}

	for _, tgt := range targets {
		if !expectedMap[tgt] {
			t.Errorf("unexpected target %s from ACTIVE", tgt)
		}
	}
}

func TestCanTransitionTo(t *testing.T) {
	vec, err := lifecycle.NewStateVector(lifecycle.StateActive, 1, "", "")
	if err != nil {
		t.Fatalf("failed init: %v", err)
	}

	if !vec.CanTransitionTo(lifecycle.StateDraining) {
		t.Errorf("expected CanTransitionTo(DRAINING) to be true")
	}
	if vec.CanTransitionTo(lifecycle.StateObserved) {
		t.Errorf("expected CanTransitionTo(OBSERVED) to be false")
	}
}

func TestUnknownStateEdgeQueries(t *testing.T) {
	if lifecycle.CanTransition("UNKNOWN", lifecycle.StateActive) {
		t.Errorf("expected false for unknown source")
	}
	if targets := lifecycle.LegalTransitions("UNKNOWN"); targets != nil {
		t.Errorf("expected nil targets for unknown source, got %v", targets)
	}
}

func TestExplicitDimensionsInTransition(t *testing.T) {
	vec, err := lifecycle.NewStateVector(lifecycle.StatePromoted, 1, "", "")
	if err != nil {
		t.Fatalf("failed init: %v", err)
	}

	next, err := vec.Transition(lifecycle.TransitionIntent{
		TargetState: lifecycle.StateActive,
		Reason:      "direct activation",
		Conformance: lifecycle.ConformanceConformant,
		Readiness:   lifecycle.ReadinessReady,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.Conformance != lifecycle.ConformanceConformant || next.Readiness != lifecycle.ReadinessReady {
		t.Errorf("dimensions mismatch: %+v", next)
	}
}
