/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"time"
)

// LatencyBudget propagates a request-level deadline across inference phases.
// Each phase (route → prefill → decode → respond) gets a proportional budget.
type LatencyBudget struct {
	// TotalBudget is the end-to-end latency limit.
	TotalBudget time.Duration

	// RoutingBudget is the max time for endpoint selection.
	RoutingBudget time.Duration

	// PrefillBudget is the max time for the prefill phase.
	PrefillBudget time.Duration

	// DecodeBudget is the max time for decode (remaining after route+prefill).
	DecodeBudget time.Duration

	// Deadline is the absolute wall-clock deadline.
	Deadline time.Time
}

// DefaultLatencyBudget returns a 30s total budget split across phases.
func DefaultLatencyBudget() LatencyBudget {
	total := 30 * time.Second
	return LatencyBudget{
		TotalBudget:   total,
		RoutingBudget: 50 * time.Millisecond,
		PrefillBudget: 10 * time.Second,
		DecodeBudget:  total - 50*time.Millisecond - 10*time.Second,
		Deadline:      time.Now().Add(total),
	}
}

// WithContext returns a context with the budget's deadline.
func (b *LatencyBudget) WithContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, b.Deadline)
}

// Remaining returns the time left in the budget.
func (b *LatencyBudget) Remaining() time.Duration {
	return time.Until(b.Deadline)
}

// PhaseContext returns a context scoped to a specific phase budget.
func (b *LatencyBudget) PhaseContext(ctx context.Context, phase string) (context.Context, context.CancelFunc) {
	var budget time.Duration
	switch phase {
	case "route":
		budget = b.RoutingBudget
	case "prefill":
		budget = b.PrefillBudget
	case "decode":
		budget = b.DecodeBudget
	default:
		budget = b.Remaining()
	}

	deadline := time.Now().Add(budget)
	if deadline.After(b.Deadline) {
		deadline = b.Deadline
	}
	return context.WithDeadline(ctx, deadline)
}

// Exceeded returns true if the budget is exhausted.
func (b *LatencyBudget) Exceeded() bool {
	return time.Now().After(b.Deadline)
}
