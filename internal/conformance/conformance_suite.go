/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package conformance

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ConformanceSuite executes and evaluates vectors across the 12 Conformance Classes.
type ConformanceSuite struct {
	mu      sync.RWMutex
	vectors []ConformanceVector
}

// NewConformanceSuite creates an empty conformance test suite.
func NewConformanceSuite() *ConformanceSuite {
	return &ConformanceSuite{
		vectors: make([]ConformanceVector, 0),
	}
}

// Register registers a single conformance vector.
func (cs *ConformanceSuite) Register(v ConformanceVector) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("invalid vector: %w", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, existing := range cs.vectors {
		if existing.ID == v.ID {
			return fmt.Errorf("vector with ID %q already registered", v.ID)
		}
	}
	cs.vectors = append(cs.vectors, v)
	return nil
}

// RegisterMany registers multiple conformance vectors.
func (cs *ConformanceSuite) RegisterMany(vectors ...ConformanceVector) error {
	for _, v := range vectors {
		if err := cs.Register(v); err != nil {
			return fmt.Errorf("failed to register vector %s: %w", v.ID, err)
		}
	}
	return nil
}

// Vectors returns a slice of all registered conformance vectors.
func (cs *ConformanceSuite) Vectors() []ConformanceVector {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	out := make([]ConformanceVector, len(cs.vectors))
	copy(out, cs.vectors)
	return out
}

// VectorsByClass returns all registered vectors for a specific conformance class.
func (cs *ConformanceSuite) VectorsByClass(class ConformanceClass) []ConformanceVector {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var matching []ConformanceVector
	for _, v := range cs.vectors {
		if v.Class == class {
			matching = append(matching, v)
		}
	}
	return matching
}

// VectorsByTarget returns all registered vectors for a specific target component.
func (cs *ConformanceSuite) VectorsByTarget(target string) []ConformanceVector {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var matching []ConformanceVector
	for _, v := range cs.vectors {
		if v.Target == target {
			matching = append(matching, v)
		}
	}
	return matching
}

// Run executes all registered vectors sequentially and evaluates the categorical state.
func (cs *ConformanceSuite) Run(ctx context.Context) (*SuiteSummary, error) {
	vectors := cs.Vectors()
	results := make([]VectorResult, 0, len(vectors))

	for _, v := range vectors {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("conformance run aborted: %w", err)
		}

		start := time.Now()
		res, err := v.Execute(ctx)
		elapsed := time.Since(start)

		if err != nil {
			res = &VectorResult{
				VectorID:          v.ID,
				Name:              v.Name,
				Class:             v.Class,
				Severity:          v.Severity,
				Verdict:           VerdictHaltViolation,
				Message:           fmt.Sprintf("Execution error: %v", err),
				ExecutionDuration: elapsed,
			}
		} else if res != nil {
			res.ExecutionDuration = elapsed
		}
		results = append(results, *res)
	}

	summary := cs.EvaluateCategoricalState(results)
	return &summary, nil
}

// EvaluateCategoricalState aggregates vector outcomes against categorical state
// without averaging away anti-violations.
func (cs *ConformanceSuite) EvaluateCategoricalState(results []VectorResult) SuiteSummary {
	summary := SuiteSummary{
		TotalVectors:  len(results),
		ClassCoverage: make(map[ConformanceClass]int),
		Results:       results,
	}

	highestRank := verdictLatticeRank(VerdictSatisfied)
	aggregatedVerdict := VerdictSatisfied
	hasHardAntiViolation := false

	for _, r := range results {
		summary.ClassCoverage[r.Class]++
		tallyResultVerdict(&summary, r.Verdict)

		rank := verdictLatticeRank(r.Verdict)
		if rank > highestRank {
			highestRank = rank
			aggregatedVerdict = r.Verdict
		}

		if r.Severity == SeverityHardAnti && r.Verdict != VerdictSatisfied {
			hasHardAntiViolation = true
		}
	}

	applyDominanceResolution(&summary, aggregatedVerdict, hasHardAntiViolation)
	return summary
}

// tallyResultVerdict increments the appropriate counter in the suite summary.
func tallyResultVerdict(summary *SuiteSummary, verdict CategoricalVerdict) {
	switch verdict {
	case VerdictSatisfied:
		summary.PassedCount++
	case VerdictDegraded:
		summary.DegradedCount++
	case VerdictRejected:
		summary.RejectedCount++
	case VerdictQuarantined:
		summary.QuarantinedCount++
	case VerdictHaltViolation:
		summary.HardViolationCount++
	}
}

// applyDominanceResolution applies strict dominance of hard anti-invariants
// over arithmetic pass rates.
func applyDominanceResolution(summary *SuiteSummary, latticeVerdict CategoricalVerdict, hasHardAntiViolation bool) {
	if hasHardAntiViolation {
		summary.CategoricalVerdict = VerdictHaltViolation
		summary.DominanceInvoked = true
		summary.CompositeTrust = "denied"
		return
	}

	summary.CategoricalVerdict = latticeVerdict
	summary.DominanceInvoked = false

	switch latticeVerdict {
	case VerdictSatisfied:
		summary.CompositeTrust = "verified"
	case VerdictDegraded:
		summary.CompositeTrust = "asserted"
	case VerdictRejected:
		summary.CompositeTrust = "untrusted"
	case VerdictQuarantined:
		summary.CompositeTrust = "quarantined"
	case VerdictHaltViolation:
		summary.CompositeTrust = "denied"
	}
}
