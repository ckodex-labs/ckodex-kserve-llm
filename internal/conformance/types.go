/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package conformance

import (
	"context"
	"fmt"
	"time"
)

// ConformanceClass represents one of the 12 canonical Conformance Classes
// defined in CKODEX Constitution §§ 21–23.
type ConformanceClass string

const (
	ClassPositive             ConformanceClass = "positive"
	ClassNegative             ConformanceClass = "negative"
	ClassAnti                 ConformanceClass = "anti"
	ClassTemporal             ConformanceClass = "temporal"
	ClassDegradation          ConformanceClass = "degradation"
	ClassRecovery             ConformanceClass = "recovery"
	ClassRace                 ConformanceClass = "race"
	ClassMetamorphic          ConformanceClass = "metamorphic"
	ClassSaturation           ConformanceClass = "saturation"
	ClassValidButWrong        ConformanceClass = "valid_but_wrong"
	ClassSilence              ConformanceClass = "silence"
	ClassCounterfactualReplay ConformanceClass = "counterfactual_replay"
)

// AllConformanceClasses lists the 12 canonical conformance classes in order.
var AllConformanceClasses = []ConformanceClass{
	ClassPositive,
	ClassNegative,
	ClassAnti,
	ClassTemporal,
	ClassDegradation,
	ClassRecovery,
	ClassRace,
	ClassMetamorphic,
	ClassSaturation,
	ClassValidButWrong,
	ClassSilence,
	ClassCounterfactualReplay,
}

// InvariantSeverity defines the governance severity of an invariant.
type InvariantSeverity string

const (
	SeverityHardAnti   InvariantSeverity = "hard_anti_invariant"
	SeverityStandard   InvariantSeverity = "standard_invariant"
	SeverityDegradable InvariantSeverity = "degradable_expectation"
)

// CategoricalVerdict represents the lattice-ordered discrete outcome
// of a conformance test or suite evaluation.
type CategoricalVerdict string

const (
	VerdictSatisfied     CategoricalVerdict = "satisfied"
	VerdictDegraded      CategoricalVerdict = "degraded"
	VerdictRejected      CategoricalVerdict = "rejected"
	VerdictQuarantined   CategoricalVerdict = "quarantined"
	VerdictHaltViolation CategoricalVerdict = "halt_violation"
)

// verdictLatticeRank returns the lattice ranking for dominance resolution.
// Higher rank dominates lower rank: HaltViolation > Quarantined > Rejected > Degraded > Satisfied.
func verdictLatticeRank(v CategoricalVerdict) int {
	switch v {
	case VerdictHaltViolation:
		return 5
	case VerdictQuarantined:
		return 4
	case VerdictRejected:
		return 3
	case VerdictDegraded:
		return 2
	case VerdictSatisfied:
		return 1
	default:
		return 0
	}
}

// StatePlaneVector captures the multi-plane vector state of an evaluated component.
type StatePlaneVector struct {
	Lifecycle string `json:"lifecycle"`
	Trust     string `json:"trust"`
	Integrity string `json:"integrity"`
	Binding   string `json:"binding"`
}

// ConformanceVector defines a single executable test vector belonging to a conformance class.
type ConformanceVector struct {
	ID          string
	Name        string
	Class       ConformanceClass
	Severity    InvariantSeverity
	Target      string
	Description string
	Execute     func(ctx context.Context) (*VectorResult, error)
}

// VectorResult is the concrete outcome of executing a single ConformanceVector.
type VectorResult struct {
	VectorID          string
	Name              string
	Class             ConformanceClass
	Severity          InvariantSeverity
	Verdict           CategoricalVerdict
	ObservedState     StatePlaneVector
	EvidenceDigest    string
	Message           string
	ExecutionDuration time.Duration
}

// SuiteSummary holds the aggregated evaluation of all executed vectors.
type SuiteSummary struct {
	TotalVectors       int
	PassedCount        int
	DegradedCount      int
	RejectedCount      int
	QuarantinedCount   int
	HardViolationCount int
	CategoricalVerdict CategoricalVerdict
	DominanceInvoked   bool
	CompositeTrust     string
	ClassCoverage      map[ConformanceClass]int
	Results            []VectorResult
}

// IsViable returns true only if the categorical state has not halted on an invariant violation.
func (s *SuiteSummary) IsViable() bool {
	return s.CategoricalVerdict != VerdictHaltViolation && s.HardViolationCount == 0
}

// Validate checks whether a ConformanceVector definition is valid.
func (v *ConformanceVector) Validate() error {
	if v.ID == "" {
		return fmt.Errorf("conformance vector ID cannot be empty")
	}
	if v.Class == "" {
		return fmt.Errorf("conformance vector %s has empty class", v.ID)
	}
	if v.Execute == nil {
		return fmt.Errorf("conformance vector %s has nil execute function", v.ID)
	}
	return nil
}
