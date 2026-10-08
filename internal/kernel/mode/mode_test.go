/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package mode_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/kernel/mode"
)

func TestValidateMode(t *testing.T) {
	tests := []struct {
		m       mode.Mode
		wantErr bool
	}{
		{mode.ModeNormal, false},
		{mode.ModeDegraded, false},
		{mode.ModeSafeHold, false},
		{mode.ModeQuarantined, false},
		{mode.ModeRecovering, false},
		{mode.ModeFailed, false},
		{mode.Mode("UNKNOWN"), true},
		{mode.Mode(""), true},
	}

	for _, tt := range tests {
		err := mode.ValidateMode(tt.m)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateMode(%q) error = %v, wantErr = %v", tt.m, err, tt.wantErr)
		}
		if tt.wantErr && !errors.Is(err, mode.ErrInvalidMode) {
			t.Errorf("expected ErrInvalidMode, got %v", err)
		}
	}
}

func TestGetContract(t *testing.T) {
	c, err := mode.GetContract(mode.ModeNormal)
	if err != nil || c.Traffic != mode.TrafficAdmitAll || c.Mutation != mode.MutationAllowed {
		t.Fatalf("normal contract mismatch: %+v, err: %v", c, err)
	}

	cq, err := mode.GetContract(mode.ModeQuarantined)
	if err != nil || cq.Traffic != mode.TrafficBlocked || cq.Mutation != mode.MutationFrozen {
		t.Fatalf("quarantined contract mismatch: %+v, err: %v", cq, err)
	}

	cs, err := mode.GetContract(mode.ModeSafeHold)
	if err != nil || cs.Traffic != mode.TrafficPaused || cs.SLA != mode.SLASuspended {
		t.Fatalf("safe hold contract mismatch: %+v, err: %v", cs, err)
	}

	_, err = mode.GetContract("NON_EXISTENT")
	if !errors.Is(err, mode.ErrInvalidMode) {
		t.Fatalf("expected ErrInvalidMode, got %v", err)
	}
}

func TestNewModeVector(t *testing.T) {
	t.Run("valid initialization", func(t *testing.T) {
		vec, err := mode.NewModeVector(mode.ModeNormal, mode.SeverityNone, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vec.CurrentMode != mode.ModeNormal {
			t.Errorf("got current mode %s, want %s", vec.CurrentMode, mode.ModeNormal)
		}
		if vec.Epoch != 5 {
			t.Errorf("got epoch %d, want 5", vec.Epoch)
		}
		if vec.Severity != mode.SeverityNone {
			t.Errorf("got severity %s, want %s", vec.Severity, mode.SeverityNone)
		}
		if vec.Contract.Traffic != mode.TrafficAdmitAll {
			t.Errorf("got contract traffic %s, want %s", vec.Contract.Traffic, mode.TrafficAdmitAll)
		}
	})

	t.Run("default severity initialization", func(t *testing.T) {
		vec, err := mode.NewModeVector(mode.ModeSafeHold, "", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vec.Severity != mode.SeverityHigh {
			t.Errorf("got default severity %s, want %s", vec.Severity, mode.SeverityHigh)
		}
	})

	t.Run("invalid mode initialization", func(t *testing.T) {
		_, err := mode.NewModeVector("INVALID_MODE", "", 0)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})
}

func stepOperational(
	t *testing.T,
	v mode.ModeVector,
	target mode.Mode,
	sev mode.SeverityLevel,
	reason, ev string,
	dims map[string]string,
) mode.ModeVector {
	t.Helper()
	prev := v.CurrentMode
	next, err := v.Transition(mode.TransitionIntent{
		TargetMode:     target,
		Severity:       sev,
		Reason:         reason,
		EvidenceDigest: ev,
		Dimensions:     dims,
		Timestamp:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("transition from %s to %s failed: %v", prev, target, err)
	}
	if next.CurrentMode != target || next.PreviousMode != prev {
		t.Fatalf("expected mode %s (prev %s), got %s (prev %s)", target, prev, next.CurrentMode, next.PreviousMode)
	}
	if next.Severity != sev || next.Contract.Mode != target {
		t.Fatalf("unexpected severity %s or contract mode %s", next.Severity, next.Contract.Mode)
	}
	return next
}

func TestValidOperationalTransitions(t *testing.T) {
	v, err := mode.NewModeVector(mode.ModeNormal, mode.SeverityNone, 0)
	if err != nil {
		t.Fatalf("failed init: %v", err)
	}

	v = stepOperational(t, v, mode.ModeDegraded, mode.SeverityLow, "p99 latency breach", "sha256:p99-01", map[string]string{"latency_p99_ms": "450"})
	v = stepOperational(t, v, mode.ModeSafeHold, mode.SeverityHigh, "circuit breaker tripped", "sha256:drift-02", map[string]string{"circuit_breaker": "tripped"})
	v = stepOperational(t, v, mode.ModeRecovering, mode.SeverityLow, "remediation active", "sha256:recon-03", map[string]string{"remediation": "task-42"})
	v = stepOperational(t, v, mode.ModeNormal, mode.SeverityNone, "all probes healthy", "sha256:probe-04", map[string]string{"health": "ok"})

	if v.TransitionCount != 4 {
		t.Errorf("expected 4 transitions, got %d", v.TransitionCount)
	}
}

func allExpectedModeEdges() [][2]mode.Mode {
	return [][2]mode.Mode{
		{mode.ModeNormal, mode.ModeDegraded},
		{mode.ModeNormal, mode.ModeSafeHold},
		{mode.ModeNormal, mode.ModeQuarantined},
		{mode.ModeNormal, mode.ModeFailed},
		{mode.ModeDegraded, mode.ModeNormal},
		{mode.ModeDegraded, mode.ModeSafeHold},
		{mode.ModeDegraded, mode.ModeQuarantined},
		{mode.ModeDegraded, mode.ModeRecovering},
		{mode.ModeDegraded, mode.ModeFailed},
		{mode.ModeSafeHold, mode.ModeNormal},
		{mode.ModeSafeHold, mode.ModeRecovering},
		{mode.ModeSafeHold, mode.ModeQuarantined},
		{mode.ModeSafeHold, mode.ModeFailed},
		{mode.ModeQuarantined, mode.ModeRecovering},
		{mode.ModeQuarantined, mode.ModeFailed},
		{mode.ModeRecovering, mode.ModeNormal},
		{mode.ModeRecovering, mode.ModeDegraded},
		{mode.ModeRecovering, mode.ModeSafeHold},
		{mode.ModeRecovering, mode.ModeQuarantined},
		{mode.ModeRecovering, mode.ModeFailed},
		{mode.ModeFailed, mode.ModeRecovering},
	}
}

func TestLegalTransitionsAllEdges(t *testing.T) {
	for _, e := range allExpectedModeEdges() {
		if !mode.CanTransition(e[0], e[1]) {
			t.Errorf("expected CanTransition(%s, %s) to be true", e[0], e[1])
		}
	}
}

func TestProhibitedTransitions(t *testing.T) {
	illegalPairs := []struct {
		from mode.Mode
		to   mode.Mode
	}{
		{mode.ModeNormal, mode.ModeRecovering},    // cannot recover from normal
		{mode.ModeFailed, mode.ModeNormal},        // cannot jump directly from failed to normal
		{mode.ModeFailed, mode.ModeDegraded},      // must recover first
		{mode.ModeFailed, mode.ModeSafeHold},      // must recover first
		{mode.ModeQuarantined, mode.ModeNormal},   // must recover first
		{mode.ModeQuarantined, mode.ModeDegraded}, // must recover first
		{mode.ModeQuarantined, mode.ModeSafeHold}, // must recover first
	}

	for _, p := range illegalPairs {
		if mode.CanTransition(p.from, p.to) {
			t.Errorf("CanTransition(%s, %s) unexpectedly true", p.from, p.to)
		}

		vec, err := mode.NewModeVector(p.from, "", 0)
		if err != nil {
			t.Fatalf("failed init: %v", err)
		}
		_, err = vec.Transition(mode.TransitionIntent{
			TargetMode: p.to,
			Reason:     "attempting illegal mode jump",
		})
		if err == nil {
			t.Errorf("expected error transitioning from %s to %s, got nil", p.from, p.to)
		}
		if !errors.Is(err, mode.ErrIllegalTransition) {
			t.Errorf("expected ErrIllegalTransition from %s to %s, got %v", p.from, p.to, err)
		}
	}
}

func TestTransitionIntentValidation(t *testing.T) {
	vec, err := mode.NewModeVector(mode.ModeNormal, "", 1)
	if err != nil {
		t.Fatalf("failed init: %v", err)
	}

	t.Run("empty reason rejection", func(t *testing.T) {
		_, err := vec.Transition(mode.TransitionIntent{
			TargetMode: mode.ModeDegraded,
			Reason:     "",
		})
		if !errors.Is(err, mode.ErrInvalidIntent) {
			t.Errorf("expected ErrInvalidIntent for empty reason, got %v", err)
		}
	})

	t.Run("invalid target mode rejection", func(t *testing.T) {
		_, err := vec.Transition(mode.TransitionIntent{
			TargetMode: "BOGUS_MODE",
			Reason:     "valid reason",
		})
		if !errors.Is(err, mode.ErrInvalidMode) {
			t.Errorf("expected ErrInvalidMode for bogus target, got %v", err)
		}
	})
}

func TestLegalTargets(t *testing.T) {
	vec, err := mode.NewModeVector(mode.ModeQuarantined, "", 1)
	if err != nil {
		t.Fatalf("failed init: %v", err)
	}

	targets := vec.LegalTargets()
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets from QUARANTINED, got %d", len(targets))
	}

	expectedMap := map[mode.Mode]bool{
		mode.ModeRecovering: true,
		mode.ModeFailed:     true,
	}

	for _, tgt := range targets {
		if !expectedMap[tgt] {
			t.Errorf("unexpected target %s from QUARANTINED", tgt)
		}
	}
}

func TestCanTransitionTo(t *testing.T) {
	vec, err := mode.NewModeVector(mode.ModeNormal, "", 1)
	if err != nil {
		t.Fatalf("failed init: %v", err)
	}

	if !vec.CanTransitionTo(mode.ModeDegraded) {
		t.Errorf("expected CanTransitionTo(DEGRADED) to be true")
	}
	if vec.CanTransitionTo(mode.ModeRecovering) {
		t.Errorf("expected CanTransitionTo(RECOVERING) to be false")
	}
}

func TestUnknownModeEdgeQueries(t *testing.T) {
	if mode.CanTransition("UNKNOWN", mode.ModeNormal) {
		t.Errorf("expected false for unknown mode")
	}
	if targets := mode.LegalTransitions("UNKNOWN"); targets != nil {
		t.Errorf("expected nil targets for unknown mode, got %v", targets)
	}
}

func TestAllModeContracts(t *testing.T) {
	modes := []mode.Mode{
		mode.ModeNormal,
		mode.ModeDegraded,
		mode.ModeSafeHold,
		mode.ModeQuarantined,
		mode.ModeRecovering,
		mode.ModeFailed,
	}

	for _, m := range modes {
		c, err := mode.GetContract(m)
		if err != nil {
			t.Errorf("GetContract(%s) error = %v", m, err)
		}
		if c.Mode != m {
			t.Errorf("contract mode mismatch: got %s, want %s", c.Mode, m)
		}
	}
}
