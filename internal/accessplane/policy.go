/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package accessplane evaluates tenant admission and model-routing policy.
package accessplane

import (
	"context"
	"fmt"
)

// Disposition is the action a request plane must take after policy evaluation.
type Disposition string

const (
	DispositionAdmit        Disposition = "admit"
	DispositionBackpressure Disposition = "backpressure"
	DispositionReject       Disposition = "reject"
)

// Intent is the policy-relevant portion of an inference request.
type Intent struct {
	TenantID string
	Route    string
}

// TargetPolicy describes one model target in deterministic preference order.
type TargetPolicy struct {
	Model       string
	MaxInFlight int
}

// RoutePolicy maps an intent-level route to ordered model targets.
type RoutePolicy struct {
	Targets []TargetPolicy
}

// TenantPolicy defines the routes and admission limits for one tenant.
type TenantPolicy struct {
	TenantID      string
	MaxInFlight   int
	MaxQueueDepth int
	Routes        map[string]RoutePolicy
}

// TenantObservation is caller-supplied runtime load for one tenant.
type TenantObservation struct {
	InFlight   int
	QueueDepth int
}

// ModelObservation is caller-supplied runtime eligibility for one model.
type ModelObservation struct {
	Ready    bool
	InFlight int
}

// RuntimeObservation is an immutable input snapshot. The evaluator does not
// create, enqueue, or dequeue requests.
type RuntimeObservation struct {
	Tenant TenantObservation
	Models map[string]ModelObservation
}

// Decision is the complete access-plane result for one request intent.
// Backpressure reports policy headroom only; it does not assert that a request
// was placed in a queue.
type Decision struct {
	Disposition            Disposition
	Model                  string
	Reason                 string
	QueueCapacityRemaining int
}

// Evaluator holds a validated, immutable tenant policy catalog.
type Evaluator struct {
	tenants map[string]TenantPolicy
}

// NewEvaluator validates and copies tenant policies.
func NewEvaluator(policies []TenantPolicy) (*Evaluator, error) {
	tenants := make(map[string]TenantPolicy, len(policies))
	for _, policy := range policies {
		if err := validateTenantPolicy(policy); err != nil {
			return nil, err
		}
		if _, exists := tenants[policy.TenantID]; exists {
			return nil, fmt.Errorf("duplicate tenant policy %q", policy.TenantID)
		}
		tenants[policy.TenantID] = cloneTenantPolicy(policy)
	}
	return &Evaluator{tenants: tenants}, nil
}

// Evaluate selects a model or returns a backpressure/rejection instruction.
func (e *Evaluator) Evaluate(intent Intent, observed RuntimeObservation) Decision {
	policy, ok := e.tenants[intent.TenantID]
	if !ok {
		return reject("tenant-not-admitted")
	}
	if !validObservation(observed) {
		return reject("invalid-runtime-observation")
	}
	route, ok := policy.Routes[intent.Route]
	if !ok {
		return reject("route-not-admitted")
	}
	if observed.Tenant.InFlight >= policy.MaxInFlight {
		return capacityDecision(policy, observed, "tenant-concurrency-limit")
	}
	for _, target := range route.Targets {
		load, exists := observed.Models[target.Model]
		if exists && load.Ready && load.InFlight < target.MaxInFlight {
			return Decision{Disposition: DispositionAdmit, Model: target.Model, Reason: "target-eligible"}
		}
	}
	return capacityDecision(policy, observed, "no-eligible-target")
}

// EvaluateContext evaluates policy only while the request context remains
// active. Evaluation is in-memory and deterministic; the context check keeps a
// request whose deadline has already elapsed from entering policy evaluation.
func (e *Evaluator) EvaluateContext(
	ctx context.Context,
	intent Intent,
	observed RuntimeObservation,
) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, fmt.Errorf("access policy evaluation canceled: %w", err)
	}

	decision := e.Evaluate(intent, observed)
	if err := ctx.Err(); err != nil {
		return Decision{}, fmt.Errorf("access policy evaluation canceled: %w", err)
	}
	return decision, nil
}

func capacityDecision(policy TenantPolicy, observed RuntimeObservation, reason string) Decision {
	remaining := policy.MaxQueueDepth - observed.Tenant.QueueDepth
	if remaining > 0 {
		return Decision{
			Disposition:            DispositionBackpressure,
			Reason:                 reason,
			QueueCapacityRemaining: remaining,
		}
	}
	return reject("queue-capacity-exhausted")
}

func reject(reason string) Decision {
	return Decision{Disposition: DispositionReject, Reason: reason}
}

func validObservation(observed RuntimeObservation) bool {
	if observed.Tenant.InFlight < 0 || observed.Tenant.QueueDepth < 0 {
		return false
	}
	for _, model := range observed.Models {
		if model.InFlight < 0 {
			return false
		}
	}
	return true
}

func validateTenantPolicy(policy TenantPolicy) error {
	if policy.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if policy.MaxInFlight <= 0 {
		return fmt.Errorf("tenant %q max in-flight must be positive", policy.TenantID)
	}
	if policy.MaxQueueDepth < 0 {
		return fmt.Errorf("tenant %q max queue depth cannot be negative", policy.TenantID)
	}
	if len(policy.Routes) == 0 {
		return fmt.Errorf("tenant %q must declare at least one route", policy.TenantID)
	}
	for name, route := range policy.Routes {
		if err := validateRoutePolicy(policy.TenantID, name, route); err != nil {
			return err
		}
	}
	return nil
}

func validateRoutePolicy(tenantID, name string, route RoutePolicy) error {
	if name == "" {
		return fmt.Errorf("tenant %q route name is required", tenantID)
	}
	if len(route.Targets) == 0 {
		return fmt.Errorf("tenant %q route %q requires targets", tenantID, name)
	}
	seen := make(map[string]struct{}, len(route.Targets))
	for _, target := range route.Targets {
		if target.Model == "" || target.MaxInFlight <= 0 {
			return fmt.Errorf("tenant %q route %q has invalid target", tenantID, name)
		}
		if _, exists := seen[target.Model]; exists {
			return fmt.Errorf("tenant %q route %q repeats target %q", tenantID, name, target.Model)
		}
		seen[target.Model] = struct{}{}
	}
	return nil
}

func cloneTenantPolicy(policy TenantPolicy) TenantPolicy {
	routes := make(map[string]RoutePolicy, len(policy.Routes))
	for name, route := range policy.Routes {
		targets := append([]TargetPolicy(nil), route.Targets...)
		routes[name] = RoutePolicy{Targets: targets}
	}
	policy.Routes = routes
	return policy
}
