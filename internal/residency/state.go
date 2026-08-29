/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package residency defines the model-residency control-plane state machine.
// It returns declarative actions; cache, runtime, session, and route owners
// execute those actions at their existing ownership boundaries.
package residency

import (
	"errors"
	"time"
)

// State is the durable residency phase observed by a reconciler.
type State string

const (
	StateCold     State = "cold"
	StateCached   State = "cached"
	StateLoading  State = "loading"
	StateWarming  State = "warming"
	StateReady    State = "ready"
	StateDraining State = "draining"
	StateEvicting State = "evicting"
)

// Action is a requested effect for the owning controller.
type Action string

const (
	ActionCacheArtifact Action = "cache-artifact"
	ActionLoadRuntime   Action = "load-runtime"
	ActionWarmRuntime   Action = "warm-runtime"
	ActionAttachRoute   Action = "attach-route"
	ActionDetachRoute   Action = "detach-route"
	ActionWaitForDrain  Action = "wait-for-drain"
	ActionStopRuntime   Action = "stop-runtime"
	ActionEvictArtifact Action = "evict-artifact"
)

var (
	ErrInvalidPlan       = errors.New("invalid residency plan")
	ErrInvariantViolated = errors.New("residency invariant violated")
	ErrTransitionFailed  = errors.New("residency transition failed")
	ErrTransitionTimeout = errors.New("residency transition timed out")
)

// Plan is desired state compiled by policy.
type Plan struct {
	Target         State
	RequirePrefill bool
	Timeout        time.Duration
}

// Snapshot is the persisted transaction cursor.
type Snapshot struct {
	State          State
	RollbackTarget State
	StartedAt      time.Time
	Deadline       time.Time
}

// Observation is the bounded evidence used to advance a transaction.
type Observation struct {
	ArtifactCached    bool
	RuntimeLoaded     bool
	RuntimeReady      bool
	PrefillReady      bool
	RouteAttached     bool
	AcceptingRequests bool
	ActiveRequests    int64
	Failure           string
}

// Policy is the request/route posture derived from observed state.
type Policy struct {
	AllowRoute        bool
	AcceptNewRequests bool
	RetainRuntime     bool
	RetainArtifact    bool
}

// Status is the reverse projection from observed state to operator status.
type Status struct {
	State       State
	Ready       bool
	Progressing bool
	Reason      string
}

// Decision is one atomic state update plus the effects required after it.
type Decision struct {
	Snapshot Snapshot
	Actions  []Action
	Policy   Policy
	Status   Status
}
