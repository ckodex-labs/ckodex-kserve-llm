/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package lifecycle implements the pure Semantic Kernel lifecycle state machine
// for AI workloads, model artifacts, and AIPacks per CKODEX specifications.
//
// Invariants:
// - Zero external dependencies (no k8s, http, or cloud SDKs).
// - Vector state representation (no boolean flags).
// - Strict state transition verification.
package lifecycle

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// State represents the lifecycle state of a governed AI workload or artifact.
type State string

const (
	StateQuarantined State = "QUARANTINED"
	StateObserved    State = "OBSERVED"
	StateBenchmarked State = "BENCHMARKED"
	StateGoverned    State = "GOVERNED"
	StatePromoted    State = "PROMOTED"
	StateWarming     State = "WARMING"
	StateActive      State = "ACTIVE"
	StateDraining    State = "DRAINING"
	StateRetired     State = "RETIRED"
)

// ConformanceAxis represents the compliance and verification status dimension.
type ConformanceAxis string

const (
	ConformanceUnverified ConformanceAxis = "UNVERIFIED"
	ConformancePending    ConformanceAxis = "PENDING"
	ConformanceConformant ConformanceAxis = "CONFORMANT"
	ConformanceDegraded   ConformanceAxis = "DEGRADED"
	ConformanceBreached   ConformanceAxis = "BREACHED"
)

// ReadinessAxis represents the operational serving posture dimension.
type ReadinessAxis string

const (
	ReadinessCold     ReadinessAxis = "COLD"
	ReadinessWarming  ReadinessAxis = "WARMING"
	ReadinessReady    ReadinessAxis = "READY"
	ReadinessDraining ReadinessAxis = "DRAINING"
	ReadinessHalted   ReadinessAxis = "HALTED"
)

var (
	// ErrInvalidState is returned when an unrecognized lifecycle state is encountered.
	ErrInvalidState = errors.New("invalid lifecycle state")

	// ErrIllegalTransition is returned when a state transition violates edge rules.
	ErrIllegalTransition = errors.New("illegal lifecycle state transition")

	// ErrInvalidIntent is returned when transition parameters are incomplete or invalid.
	ErrInvalidIntent = errors.New("invalid transition intent")
)

var validStates = map[State]struct{}{
	StateQuarantined: {},
	StateObserved:    {},
	StateBenchmarked: {},
	StateGoverned:    {},
	StatePromoted:    {},
	StateWarming:     {},
	StateActive:      {},
	StateDraining:    {},
	StateRetired:     {},
}

// legalTransitions defines the edge graph of permissible lifecycle transitions.
var legalTransitions = map[State]map[State]struct{}{
	StateQuarantined: {
		StateObserved: {},
		StateRetired:  {},
	},
	StateObserved: {
		StateBenchmarked: {},
		StateQuarantined: {},
		StateRetired:     {},
	},
	StateBenchmarked: {
		StateGoverned:    {},
		StateObserved:    {},
		StateQuarantined: {},
		StateRetired:     {},
	},
	StateGoverned: {
		StatePromoted:    {},
		StateObserved:    {},
		StateQuarantined: {},
		StateRetired:     {},
	},
	StatePromoted: {
		StateWarming:     {},
		StateActive:      {},
		StateDraining:    {},
		StateQuarantined: {},
		StateRetired:     {},
	},
	StateWarming: {
		StateActive:      {},
		StateDraining:    {},
		StateQuarantined: {},
		StateRetired:     {},
	},
	StateActive: {
		StateDraining:    {},
		StateWarming:     {},
		StateQuarantined: {},
	},
	StateDraining: {
		StateRetired:     {},
		StateActive:      {},
		StateQuarantined: {},
	},
	StateRetired: {
		StateQuarantined: {},
	},
}

// StateVector encapsulates the composite state of a subject across orthogonal axes.
// Adheres strictly to the "vector state, not booleans" rule.
type StateVector struct {
	CurrentState    State           `json:"currentState"`
	PreviousState   State           `json:"previousState"`
	Epoch           uint64          `json:"epoch"`
	TransitionCount uint64          `json:"transitionCount"`
	Conformance     ConformanceAxis `json:"conformance"`
	Readiness       ReadinessAxis   `json:"readiness"`
	LastTransition  time.Time       `json:"lastTransition"`
	EvidenceDigest  string          `json:"evidenceDigest,omitempty"`
	Reason          string          `json:"reason,omitempty"`
}

// TransitionIntent describes a requested state transition.
type TransitionIntent struct {
	TargetState    State           `json:"targetState"`
	Reason         string          `json:"reason"`
	EvidenceDigest string          `json:"evidenceDigest,omitempty"`
	Conformance    ConformanceAxis `json:"conformance,omitempty"`
	Readiness      ReadinessAxis   `json:"readiness,omitempty"`
	Timestamp      time.Time       `json:"timestamp,omitempty"`
}

// ValidateState checks whether the provided state is a recognized lifecycle state.
func ValidateState(s State) error {
	if _, ok := validStates[s]; !ok {
		return fmt.Errorf("%w: %q", ErrInvalidState, s)
	}
	return nil
}

// CanTransition returns true if moving from the source state to target state is legal.
func CanTransition(from, to State) bool {
	targets, ok := legalTransitions[from]
	if !ok {
		return false
	}
	_, legal := targets[to]
	return legal
}

// LegalTransitions returns a sorted list of legal destination states from the given state.
func LegalTransitions(from State) []State {
	targets, ok := legalTransitions[from]
	if !ok {
		return nil
	}
	res := make([]State, 0, len(targets))
	for t := range targets {
		res = append(res, t)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i] < res[j]
	})
	return res
}

// NewStateVector initializes a validated StateVector.
func NewStateVector(
	initial State,
	epoch uint64,
	conformance ConformanceAxis,
	readiness ReadinessAxis,
) (StateVector, error) {
	if err := ValidateState(initial); err != nil {
		return StateVector{}, fmt.Errorf("failed to initialize StateVector: %w", err)
	}
	if conformance == "" {
		conformance = defaultConformanceForState(initial)
	}
	if readiness == "" {
		readiness = defaultReadinessForState(initial)
	}

	return StateVector{
		CurrentState:    initial,
		PreviousState:   "",
		Epoch:           epoch,
		TransitionCount: 0,
		Conformance:     conformance,
		Readiness:       readiness,
		LastTransition:  time.Now().UTC(),
		Reason:          "initialization",
	}, nil
}

// Transition executes a validated state transition, producing a new StateVector.
func (v StateVector) Transition(intent TransitionIntent) (StateVector, error) {
	if err := ValidateState(intent.TargetState); err != nil {
		return v, fmt.Errorf("transition target validation failed: %w", err)
	}
	if intent.Reason == "" {
		return v, fmt.Errorf("%w: reason must not be empty", ErrInvalidIntent)
	}
	if !CanTransition(v.CurrentState, intent.TargetState) {
		return v, fmt.Errorf(
			"%w: transition from %s to %s is prohibited",
			ErrIllegalTransition,
			v.CurrentState,
			intent.TargetState,
		)
	}

	conf, ready, ev, ts := v.resolveTransitionDimensions(intent)
	return StateVector{
		CurrentState:    intent.TargetState,
		PreviousState:   v.CurrentState,
		Epoch:           v.Epoch + 1,
		TransitionCount: v.TransitionCount + 1,
		Conformance:     conf,
		Readiness:       ready,
		LastTransition:  ts,
		EvidenceDigest:  ev,
		Reason:          intent.Reason,
	}, nil
}

func (v StateVector) resolveTransitionDimensions(intent TransitionIntent) (
	ConformanceAxis,
	ReadinessAxis,
	string,
	time.Time,
) {
	ts := intent.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	conf := intent.Conformance
	if conf == "" {
		conf = defaultConformanceForState(intent.TargetState)
	}
	ready := intent.Readiness
	if ready == "" {
		ready = defaultReadinessForState(intent.TargetState)
	}
	ev := intent.EvidenceDigest
	if ev == "" {
		ev = v.EvidenceDigest
	}
	return conf, ready, ev, ts
}

// CanTransitionTo returns true if the current vector can advance to the target state.
func (v StateVector) CanTransitionTo(target State) bool {
	return CanTransition(v.CurrentState, target)
}

// LegalTargets returns permissible destination states from the current vector state.
func (v StateVector) LegalTargets() []State {
	return LegalTransitions(v.CurrentState)
}

// IsTerminal returns true if the subject has reached terminal lifecycle.
func (v StateVector) IsTerminal() bool {
	return v.CurrentState == StateRetired
}

func defaultConformanceForState(s State) ConformanceAxis {
	switch s {
	case StateQuarantined:
		return ConformanceBreached
	case StateObserved, StateBenchmarked:
		return ConformancePending
	case StateGoverned, StatePromoted, StateWarming, StateActive:
		return ConformanceConformant
	case StateDraining, StateRetired:
		return ConformanceConformant
	default:
		return ConformanceUnverified
	}
}

func defaultReadinessForState(s State) ReadinessAxis {
	switch s {
	case StateQuarantined:
		return ReadinessHalted
	case StateObserved, StateBenchmarked, StateGoverned, StatePromoted:
		return ReadinessCold
	case StateWarming:
		return ReadinessWarming
	case StateActive:
		return ReadinessReady
	case StateDraining:
		return ReadinessDraining
	case StateRetired:
		return ReadinessHalted
	default:
		return ReadinessCold
	}
}
