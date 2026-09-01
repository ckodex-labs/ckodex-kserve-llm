/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package residency

import "fmt"

// Project reconstructs status and request policy from a state observation.
func Project(state State, observed Observation, requirePrefill bool) (Status, Policy, error) {
	policy := policyFor(state, observed, requirePrefill)
	if err := validateObservation(state, observed, requirePrefill); err != nil {
		policy.AllowRoute = false
		policy.AcceptNewRequests = false
		return Status{State: state, Reason: "InvariantViolation"}, policy, err
	}
	status := statusFor(state, observed, requirePrefill)
	return status, policy, nil
}

func validateObservation(state State, observed Observation, requirePrefill bool) error {
	if observed.ActiveRequests < 0 {
		return fmt.Errorf("%w: active request count is negative", ErrInvariantViolated)
	}
	if observed.AcceptingRequests && !observed.RouteAttached {
		return fmt.Errorf("%w: requests accepted without an attached route", ErrInvariantViolated)
	}
	if observed.RouteAttached && !observed.RuntimeReady {
		return fmt.Errorf("%w: route attached without a ready runtime", ErrInvariantViolated)
	}
	if observed.RouteAttached && requirePrefill && !observed.PrefillReady {
		return fmt.Errorf("%w: route attached without ready prefill workers", ErrInvariantViolated)
	}
	if observed.AcceptingRequests && state != StateReady {
		return fmt.Errorf("%w: state %q accepts new requests", ErrInvariantViolated, state)
	}
	if observed.ActiveRequests > 0 && state != StateReady && state != StateDraining {
		return fmt.Errorf("%w: state %q owns active requests", ErrInvariantViolated, state)
	}
	return nil
}

func policyFor(state State, observed Observation, requirePrefill bool) Policy {
	ready := state == StateReady && observed.RuntimeReady && (!requirePrefill || observed.PrefillReady)
	return Policy{
		AllowRoute:        ready,
		AcceptNewRequests: ready,
		RetainRuntime:     state == StateLoading || state == StateWarming || state == StateReady || state == StateDraining,
		RetainArtifact:    state != StateCold && state != StateEvicting,
	}
}

func statusFor(state State, observed Observation, requirePrefill bool) Status {
	status := Status{State: state, Progressing: isTransitional(state), Reason: stateReason(state)}
	if state != StateReady {
		return status
	}
	if !observed.RuntimeReady {
		status.Reason = "RuntimeUnavailable"
		return status
	}
	if requirePrefill && !observed.PrefillReady {
		status.Reason = "PrefillUnavailable"
		return status
	}
	status.Ready = true
	status.Reason = "ResidentReady"
	return status
}

func isTransitional(state State) bool {
	return state == StateLoading || state == StateWarming || state == StateDraining || state == StateEvicting
}

func stateReason(state State) string {
	switch state {
	case StateCold:
		return "ArtifactAbsent"
	case StateCached:
		return "ArtifactCached"
	case StateLoading:
		return "RuntimeLoading"
	case StateWarming:
		return "RuntimeWarming"
	case StateDraining:
		return "RequestsDraining"
	case StateEvicting:
		return "ResidencyEvicting"
	default:
		return "StateUnknown"
	}
}
