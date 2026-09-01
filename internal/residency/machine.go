/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package residency

import (
	"fmt"
	"time"
)

const defaultTransitionTimeout = 5 * time.Minute

// Advance returns the next transaction cursor and declarative owner actions.
func Advance(now time.Time, current Snapshot, plan Plan, observed Observation) (Decision, error) {
	if err := validatePlan(current, plan); err != nil {
		return decision(current, nil, observed, plan.RequirePrefill), err
	}
	if observed.Failure != "" {
		return rollback(current, observed, plan, fmt.Errorf("%w: %s", ErrTransitionFailed, observed.Failure))
	}
	if isTransitional(current.State) && !current.Deadline.IsZero() && !now.Before(current.Deadline) {
		return rollback(current, observed, plan, ErrTransitionTimeout)
	}
	if _, _, err := Project(current.State, observed, plan.RequirePrefill); err != nil {
		return invariantDecision(current, failClosedActions(observed), observed, plan.RequirePrefill), err
	}
	switch plan.Target {
	case StateReady:
		return advanceToReady(now, current, plan, observed)
	case StateCached, StateCold:
		return advanceToLowerState(now, current, plan, observed)
	default:
		return decision(current, nil, observed, plan.RequirePrefill), ErrInvalidPlan
	}
}

func validatePlan(current Snapshot, plan Plan) error {
	if !isKnownState(current.State) {
		return fmt.Errorf("%w: unknown current state %q", ErrInvalidPlan, current.State)
	}
	if plan.Target != StateReady && plan.Target != StateCached && plan.Target != StateCold {
		return fmt.Errorf("%w: target %q is not stable", ErrInvalidPlan, plan.Target)
	}
	return nil
}

func advanceToReady(now time.Time, current Snapshot, plan Plan, observed Observation) (Decision, error) {
	switch current.State {
	case StateCold:
		if observed.ArtifactCached {
			return decision(settle(current, StateCached), nil, observed, plan.RequirePrefill), nil
		}
		return decision(current, []Action{ActionCacheArtifact}, observed, plan.RequirePrefill), nil
	case StateCached:
		if !observed.ArtifactCached {
			return decision(settle(current, StateCold), []Action{ActionCacheArtifact}, observed, plan.RequirePrefill), nil
		}
		return decision(begin(now, current, StateLoading, plan.Timeout), []Action{ActionLoadRuntime}, observed, plan.RequirePrefill), nil
	case StateLoading:
		if observed.RuntimeLoaded {
			return decision(move(current, StateWarming), []Action{ActionWarmRuntime}, observed, plan.RequirePrefill), nil
		}
		return decision(current, []Action{ActionLoadRuntime}, observed, plan.RequirePrefill), nil
	case StateWarming:
		if observed.RuntimeReady && (!plan.RequirePrefill || observed.PrefillReady) {
			return decision(settle(current, StateReady), []Action{ActionAttachRoute}, observed, plan.RequirePrefill), nil
		}
		return decision(current, []Action{ActionWarmRuntime}, observed, plan.RequirePrefill), nil
	case StateReady:
		return recoverReady(current, plan, observed), nil
	case StateDraining, StateEvicting:
		return resumeReady(now, current, plan, observed), nil
	default:
		return decision(current, nil, observed, plan.RequirePrefill), ErrInvalidPlan
	}
}

func advanceToLowerState(now time.Time, current Snapshot, plan Plan, observed Observation) (Decision, error) {
	if current.State == plan.Target {
		return decision(settle(current, current.State), nil, observed, plan.RequirePrefill), nil
	}
	switch current.State {
	case StateReady:
		return decision(begin(now, current, StateDraining, plan.Timeout), []Action{ActionDetachRoute}, observed, plan.RequirePrefill), nil
	case StateLoading, StateWarming:
		actions := append(failClosedActions(observed), ActionStopRuntime)
		return decision(begin(now, current, StateEvicting, plan.Timeout), actions, observed, plan.RequirePrefill), nil
	case StateDraining:
		return advanceDrain(current, plan, observed), nil
	case StateEvicting:
		return advanceEviction(current, plan, observed), nil
	case StateCached:
		return decision(begin(now, current, StateEvicting, plan.Timeout), []Action{ActionEvictArtifact}, observed, plan.RequirePrefill), nil
	case StateCold:
		return decision(current, []Action{ActionCacheArtifact}, observed, plan.RequirePrefill), nil
	default:
		return decision(current, nil, observed, plan.RequirePrefill), ErrInvalidPlan
	}
}

func advanceDrain(current Snapshot, plan Plan, observed Observation) Decision {
	if observed.RouteAttached {
		return decision(current, []Action{ActionDetachRoute}, observed, plan.RequirePrefill)
	}
	if observed.ActiveRequests > 0 {
		return decision(current, []Action{ActionWaitForDrain}, observed, plan.RequirePrefill)
	}
	return decision(move(current, StateEvicting), []Action{ActionStopRuntime}, observed, plan.RequirePrefill)
}

func advanceEviction(current Snapshot, plan Plan, observed Observation) Decision {
	if observed.RuntimeLoaded {
		return decision(current, []Action{ActionStopRuntime}, observed, plan.RequirePrefill)
	}
	if plan.Target == StateCached && observed.ArtifactCached {
		return decision(settle(current, StateCached), nil, observed, plan.RequirePrefill)
	}
	if observed.ArtifactCached {
		return decision(current, []Action{ActionEvictArtifact}, observed, plan.RequirePrefill)
	}
	return decision(settle(current, StateCold), nil, observed, plan.RequirePrefill)
}

func recoverReady(current Snapshot, plan Plan, observed Observation) Decision {
	if observed.RuntimeReady && (!plan.RequirePrefill || observed.PrefillReady) {
		actions := []Action(nil)
		if !observed.RouteAttached {
			actions = []Action{ActionAttachRoute}
		}
		return decision(settle(current, StateReady), actions, observed, plan.RequirePrefill)
	}
	return decision(move(current, StateWarming), []Action{ActionWarmRuntime}, observed, plan.RequirePrefill)
}

func resumeReady(now time.Time, current Snapshot, plan Plan, observed Observation) Decision {
	if observed.RuntimeReady && (!plan.RequirePrefill || observed.PrefillReady) {
		return decision(settle(current, StateReady), []Action{ActionAttachRoute}, observed, plan.RequirePrefill)
	}
	return decision(begin(now, current, StateWarming, plan.Timeout), []Action{ActionWarmRuntime}, observed, plan.RequirePrefill)
}

func rollback(current Snapshot, observed Observation, plan Plan, cause error) (Decision, error) {
	actions := failClosedActions(observed)
	if observed.ActiveRequests > 0 {
		return decision(move(current, StateDraining), append(actions, ActionWaitForDrain), observed, plan.RequirePrefill), cause
	}
	if observed.RuntimeLoaded {
		return decision(move(current, StateEvicting), append(actions, ActionStopRuntime), observed, plan.RequirePrefill), cause
	}
	target := current.RollbackTarget
	if target == StateCached && observed.ArtifactCached {
		return decision(settle(current, StateCached), actions, observed, plan.RequirePrefill), cause
	}
	if observed.ArtifactCached {
		return decision(move(current, StateEvicting), append(actions, ActionEvictArtifact), observed, plan.RequirePrefill), cause
	}
	return decision(settle(current, StateCold), actions, observed, plan.RequirePrefill), cause
}

func failClosedActions(observed Observation) []Action {
	actions := make([]Action, 0, 2)
	if observed.RouteAttached || observed.AcceptingRequests {
		actions = append(actions, ActionDetachRoute)
	}
	return actions
}

func decision(snapshot Snapshot, actions []Action, observed Observation, requirePrefill bool) Decision {
	status := statusFor(snapshot.State, observed, requirePrefill)
	policy := policyFor(snapshot.State, observed, requirePrefill)
	return Decision{Snapshot: snapshot, Actions: actions, Policy: policy, Status: status}
}

func invariantDecision(snapshot Snapshot, actions []Action, observed Observation, requirePrefill bool) Decision {
	decision := decision(snapshot, actions, observed, requirePrefill)
	decision.Status.Ready = false
	decision.Status.Reason = "InvariantViolation"
	decision.Policy.AllowRoute = false
	decision.Policy.AcceptNewRequests = false
	return decision
}

func begin(now time.Time, current Snapshot, state State, timeout time.Duration) Snapshot {
	if timeout <= 0 {
		timeout = defaultTransitionTimeout
	}
	rollbackTarget := current.State
	if isTransitional(rollbackTarget) {
		rollbackTarget = current.RollbackTarget
	}
	return Snapshot{State: state, RollbackTarget: rollbackTarget, StartedAt: now, Deadline: now.Add(timeout)}
}

func move(current Snapshot, state State) Snapshot {
	current.State = state
	return current
}

func settle(current Snapshot, state State) Snapshot {
	return Snapshot{State: state, RollbackTarget: state}
}

func isKnownState(state State) bool {
	switch state {
	case StateCold, StateCached, StateLoading, StateWarming, StateReady, StateDraining, StateEvicting:
		return true
	default:
		return false
	}
}
