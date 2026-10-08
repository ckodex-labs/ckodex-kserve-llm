/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package mode implements the pure Semantic Kernel operational degraded modes
// and operational contract state machine per CKODEX specifications.
//
// Invariants:
// - Zero external dependencies (no k8s, http, or cloud SDKs).
// - Vector state representation (no boolean flags).
// - Strict operational contract and transition verification.
package mode

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Mode represents an operational degraded mode for an AI workload.
type Mode string

const (
	ModeNormal      Mode = "NORMAL"
	ModeDegraded    Mode = "DEGRADED"
	ModeSafeHold    Mode = "SAFE_HOLD"
	ModeQuarantined Mode = "QUARANTINED"
	ModeRecovering  Mode = "RECOVERING"
	ModeFailed      Mode = "FAILED"
)

// SeverityLevel captures the incident or degradation severity along an ordinal axis.
type SeverityLevel string

const (
	SeverityNone     SeverityLevel = "NONE"
	SeverityLow      SeverityLevel = "LOW"
	SeverityMedium   SeverityLevel = "MEDIUM"
	SeverityHigh     SeverityLevel = "HIGH"
	SeverityCritical SeverityLevel = "CRITICAL"
)

// TrafficPosture defines the traffic admissibility posture of the contract.
type TrafficPosture string

const (
	TrafficAdmitAll    TrafficPosture = "ADMIT_ALL"
	TrafficConstrained TrafficPosture = "CONSTRAINED"
	TrafficPaused      TrafficPosture = "PAUSED"
	TrafficBlocked     TrafficPosture = "BLOCKED"
	TrafficProbingOnly TrafficPosture = "PROBING_ONLY"
	TrafficTerminated  TrafficPosture = "TERMINATED"
)

// MutationPosture defines whether configuration and state updates are permissible.
type MutationPosture string

const (
	MutationAllowed         MutationPosture = "ALLOWED"
	MutationRestricted      MutationPosture = "RESTRICTED"
	MutationFrozen          MutationPosture = "FROZEN"
	MutationRemediationOnly MutationPosture = "REMEDIATION_ONLY"
	MutationDisallowed      MutationPosture = "DISALLOWED"
)

// IsolationLevel defines the degree of execution and communication confinement.
type IsolationLevel string

const (
	IsolationStandard  IsolationLevel = "STANDARD"
	IsolationEnhanced  IsolationLevel = "ENHANCED"
	IsolationFrozen    IsolationLevel = "FROZEN"
	IsolationComplete  IsolationLevel = "COMPLETE"
	IsolationSandboxed IsolationLevel = "SANDBOXED"
	IsolationTerminal  IsolationLevel = "TERMINAL"
)

// SLAPosture defines service-level agreement commitments under the current mode.
type SLAPosture string

const (
	SLAStrict    SLAPosture = "STRICT"
	SLARelaxed   SLAPosture = "RELAXED"
	SLASuspended SLAPosture = "SUSPENDED"
	SLAVoided    SLAPosture = "VOIDED"
	SLAWarming   SLAPosture = "WARMING"
	SLABreached  SLAPosture = "BREACHED"
)

// HealingPosture defines autonomous self-healing obligations under the contract.
type HealingPosture string

const (
	HealingPassive                HealingPosture = "PASSIVE"
	HealingActive                 HealingPosture = "ACTIVE"
	HealingManualIntervention     HealingPosture = "MANUAL_INTERVENTION"
	HealingForensicLocked         HealingPosture = "FORENSIC_LOCKED"
	HealingInFlight               HealingPosture = "IN_FLIGHT"
	HealingSupervisorIntervention HealingPosture = "SUPERVISOR_INTERVENTION"
)

var (
	// ErrInvalidMode is returned when an unrecognized mode is supplied.
	ErrInvalidMode = errors.New("invalid degraded mode")

	// ErrIllegalTransition is returned when an operational mode transition is prohibited.
	ErrIllegalTransition = errors.New("illegal mode transition")

	// ErrInvalidIntent is returned when transition parameters are incomplete.
	ErrInvalidIntent = errors.New("invalid transition intent")
)

var validModes = map[Mode]struct{}{
	ModeNormal:      {},
	ModeDegraded:    {},
	ModeSafeHold:    {},
	ModeQuarantined: {},
	ModeRecovering:  {},
	ModeFailed:      {},
}

// legalTransitions maps permitted operational mode transitions.
var legalTransitions = map[Mode]map[Mode]struct{}{
	ModeNormal: {
		ModeDegraded:    {},
		ModeSafeHold:    {},
		ModeQuarantined: {},
		ModeFailed:      {},
	},
	ModeDegraded: {
		ModeNormal:      {},
		ModeSafeHold:    {},
		ModeQuarantined: {},
		ModeRecovering:  {},
		ModeFailed:      {},
	},
	ModeSafeHold: {
		ModeNormal:      {},
		ModeRecovering:  {},
		ModeQuarantined: {},
		ModeFailed:      {},
	},
	ModeQuarantined: {
		ModeRecovering: {},
		ModeFailed:     {},
	},
	ModeRecovering: {
		ModeNormal:      {},
		ModeDegraded:    {},
		ModeSafeHold:    {},
		ModeQuarantined: {},
		ModeFailed:      {},
	},
	ModeFailed: {
		ModeRecovering: {},
	},
}

// Contract encapsulates the operational guarantees and constraints mandated for a mode.
// Follows vector state principles by utilizing typed discrete postures instead of booleans.
type Contract struct {
	Mode        Mode            `json:"mode"`
	Traffic     TrafficPosture  `json:"traffic"`
	Mutation    MutationPosture `json:"mutation"`
	Isolation   IsolationLevel  `json:"isolation"`
	SLA         SLAPosture      `json:"sla"`
	SelfHealing HealingPosture  `json:"selfHealing"`
}

// modeContracts defines the normative operational degraded mode contract.
var modeContracts = map[Mode]Contract{
	ModeNormal: {
		Mode:        ModeNormal,
		Traffic:     TrafficAdmitAll,
		Mutation:    MutationAllowed,
		Isolation:   IsolationStandard,
		SLA:         SLAStrict,
		SelfHealing: HealingPassive,
	},
	ModeDegraded: {
		Mode:        ModeDegraded,
		Traffic:     TrafficConstrained,
		Mutation:    MutationRestricted,
		Isolation:   IsolationEnhanced,
		SLA:         SLARelaxed,
		SelfHealing: HealingActive,
	},
	ModeSafeHold: {
		Mode:        ModeSafeHold,
		Traffic:     TrafficPaused,
		Mutation:    MutationFrozen,
		Isolation:   IsolationFrozen,
		SLA:         SLASuspended,
		SelfHealing: HealingManualIntervention,
	},
	ModeQuarantined: {
		Mode:        ModeQuarantined,
		Traffic:     TrafficBlocked,
		Mutation:    MutationFrozen,
		Isolation:   IsolationComplete,
		SLA:         SLAVoided,
		SelfHealing: HealingForensicLocked,
	},
	ModeRecovering: {
		Mode:        ModeRecovering,
		Traffic:     TrafficProbingOnly,
		Mutation:    MutationRemediationOnly,
		Isolation:   IsolationSandboxed,
		SLA:         SLAWarming,
		SelfHealing: HealingInFlight,
	},
	ModeFailed: {
		Mode:        ModeFailed,
		Traffic:     TrafficTerminated,
		Mutation:    MutationDisallowed,
		Isolation:   IsolationTerminal,
		SLA:         SLABreached,
		SelfHealing: HealingSupervisorIntervention,
	},
}

// ModeVector represents the multi-dimensional operational state.
type ModeVector struct {
	CurrentMode     Mode              `json:"currentMode"`
	PreviousMode    Mode              `json:"previousMode"`
	Severity        SeverityLevel     `json:"severity"`
	Epoch           uint64            `json:"epoch"`
	TransitionCount uint64            `json:"transitionCount"`
	LastTransition  time.Time         `json:"lastTransition"`
	Contract        Contract          `json:"contract"`
	Dimensions      map[string]string `json:"dimensions,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	EvidenceDigest  string            `json:"evidenceDigest,omitempty"`
}

// TransitionIntent describes an operational mode transition request.
type TransitionIntent struct {
	TargetMode     Mode              `json:"targetMode"`
	Severity       SeverityLevel     `json:"severity"`
	Reason         string            `json:"reason"`
	EvidenceDigest string            `json:"evidenceDigest,omitempty"`
	Dimensions     map[string]string `json:"dimensions,omitempty"`
	Timestamp      time.Time         `json:"timestamp,omitempty"`
}

// ValidateMode confirms if the given mode is recognized.
func ValidateMode(m Mode) error {
	if _, ok := validModes[m]; !ok {
		return fmt.Errorf("%w: %q", ErrInvalidMode, m)
	}
	return nil
}

// GetContract returns the operational degraded mode contract for the given mode.
func GetContract(m Mode) (Contract, error) {
	if err := ValidateMode(m); err != nil {
		return Contract{}, fmt.Errorf("contract retrieval failed: %w", err)
	}
	return modeContracts[m], nil
}

// CanTransition returns true if the edge from -> to is valid.
func CanTransition(from, to Mode) bool {
	targets, ok := legalTransitions[from]
	if !ok {
		return false
	}
	_, legal := targets[to]
	return legal
}

// LegalTransitions returns a sorted list of valid destination modes from the source mode.
func LegalTransitions(from Mode) []Mode {
	targets, ok := legalTransitions[from]
	if !ok {
		return nil
	}
	res := make([]Mode, 0, len(targets))
	for t := range targets {
		res = append(res, t)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i] < res[j]
	})
	return res
}

// NewModeVector initializes a validated ModeVector.
func NewModeVector(
	initial Mode,
	severity SeverityLevel,
	epoch uint64,
) (ModeVector, error) {
	if err := ValidateMode(initial); err != nil {
		return ModeVector{}, fmt.Errorf("failed to initialize ModeVector: %w", err)
	}
	contract, err := GetContract(initial)
	if err != nil {
		return ModeVector{}, fmt.Errorf("failed to resolve initial contract: %w", err)
	}
	if severity == "" {
		severity = defaultSeverityForMode(initial)
	}

	return ModeVector{
		CurrentMode:     initial,
		PreviousMode:    "",
		Severity:        severity,
		Epoch:           epoch,
		TransitionCount: 0,
		LastTransition:  time.Now().UTC(),
		Contract:        contract,
		Dimensions:      make(map[string]string),
		Reason:          "initialization",
	}, nil
}

// Transition performs a validated operational mode transition.
func (v ModeVector) Transition(intent TransitionIntent) (ModeVector, error) {
	if err := ValidateMode(intent.TargetMode); err != nil {
		return v, fmt.Errorf("transition target validation failed: %w", err)
	}
	if intent.Reason == "" {
		return v, fmt.Errorf("%w: reason must not be empty", ErrInvalidIntent)
	}
	if !CanTransition(v.CurrentMode, intent.TargetMode) {
		return v, fmt.Errorf(
			"%w: transition from %s to %s is prohibited",
			ErrIllegalTransition,
			v.CurrentMode,
			intent.TargetMode,
		)
	}

	contract := modeContracts[intent.TargetMode]
	sev, dims, ev, ts := v.resolveTransitionDimensions(intent)

	return ModeVector{
		CurrentMode:     intent.TargetMode,
		PreviousMode:    v.CurrentMode,
		Severity:        sev,
		Epoch:           v.Epoch + 1,
		TransitionCount: v.TransitionCount + 1,
		LastTransition:  ts,
		Contract:        contract,
		Dimensions:      dims,
		Reason:          intent.Reason,
		EvidenceDigest:  ev,
	}, nil
}

func (v ModeVector) resolveTransitionDimensions(intent TransitionIntent) (
	SeverityLevel,
	map[string]string,
	string,
	time.Time,
) {
	ts := intent.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	sev := intent.Severity
	if sev == "" {
		sev = defaultSeverityForMode(intent.TargetMode)
	}

	dims := make(map[string]string, len(v.Dimensions)+len(intent.Dimensions))
	for k, val := range v.Dimensions {
		dims[k] = val
	}
	for k, val := range intent.Dimensions {
		dims[k] = val
	}

	ev := intent.EvidenceDigest
	if ev == "" {
		ev = v.EvidenceDigest
	}

	return sev, dims, ev, ts
}

// CanTransitionTo returns true if transition to target mode is legal.
func (v ModeVector) CanTransitionTo(target Mode) bool {
	return CanTransition(v.CurrentMode, target)
}

// LegalTargets returns permissible destination modes from current vector mode.
func (v ModeVector) LegalTargets() []Mode {
	return LegalTransitions(v.CurrentMode)
}

func defaultSeverityForMode(m Mode) SeverityLevel {
	switch m {
	case ModeNormal:
		return SeverityNone
	case ModeDegraded:
		return SeverityMedium
	case ModeSafeHold:
		return SeverityHigh
	case ModeQuarantined:
		return SeverityCritical
	case ModeRecovering:
		return SeverityLow
	case ModeFailed:
		return SeverityCritical
	default:
		return SeverityNone
	}
}
