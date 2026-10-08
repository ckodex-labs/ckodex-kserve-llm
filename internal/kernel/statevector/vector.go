/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package statevector

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Presence represents the ontological existence state of an entity.
type Presence string

const (
	PresenceEmpty    Presence = "EMPTY"
	PresencePresent  Presence = "PRESENT"
	PresenceUnknown  Presence = "UNKNOWN"
	PresenceRedacted Presence = "REDACTED"
)

// IsValid reports whether the Presence state is known and valid.
func (p Presence) IsValid() bool {
	return p == PresenceEmpty || p == PresencePresent || p == PresenceUnknown || p == PresenceRedacted
}

// Valence represents the behavioral alignment or affinity state.
type Valence string

const (
	ValencePositive Valence = "POSITIVE"
	ValenceNegative Valence = "NEGATIVE"
	ValenceNeutral  Valence = "NEUTRAL"
	ValenceMixed    Valence = "MIXED"
)

// IsValid reports whether the Valence state is valid.
func (v Valence) IsValid() bool {
	return v == ValencePositive || v == ValenceNegative || v == ValenceNeutral || v == ValenceMixed
}

// Coherence represents the logical consistency and relational integrity.
type Coherence string

const (
	CoherenceCoherent   Coherence = "COHERENT"
	CoherencePartial    Coherence = "PARTIAL"
	CoherenceDecoherent Coherence = "DECOHERENT"
)

// IsValid reports whether the Coherence state is valid.
func (c Coherence) IsValid() bool {
	return c == CoherenceCoherent || c == CoherencePartial || c == CoherenceDecoherent
}

// Evidence represents the epistemic backing of the state.
type Evidence string

const (
	EvidenceUnverified   Evidence = "UNVERIFIED"
	EvidenceVerified     Evidence = "VERIFIED"
	EvidenceInferred     Evidence = "INFERRED"
	EvidenceContradicted Evidence = "CONTRADICTED"
)

// IsValid reports whether the Evidence state is valid.
func (e Evidence) IsValid() bool {
	return e == EvidenceUnverified || e == EvidenceVerified || e == EvidenceInferred || e == EvidenceContradicted
}

// AntiRelationType enumerates adversarial and antagonistic relations.
type AntiRelationType string

const (
	AntiRelationAttacks     AntiRelationType = "ATTACKS"
	AntiRelationInvalidates AntiRelationType = "INVALIDATES"
	AntiRelationContradicts AntiRelationType = "CONTRADICTS"
)

// IsValid reports whether the AntiRelationType is valid.
func (a AntiRelationType) IsValid() bool {
	return a == AntiRelationAttacks || a == AntiRelationInvalidates || a == AntiRelationContradicts
}

// AntiRelation models an antagonistic relation between premises, states, or claims.
type AntiRelation struct {
	Type   AntiRelationType `json:"type"`
	Source string           `json:"source"`
	Target string           `json:"target"`
	Reason string           `json:"reason"`
	Hard   bool             `json:"hard"`
}

// Decision represents the transition verdict.
type Decision string

const (
	DecisionAccept     Decision = "ACCEPT"
	DecisionReject     Decision = "REJECT"
	DecisionQuarantine Decision = "QUARANTINE"
)

// Lineage models the causal chain L in S(e,t) = <P, V, C, E, L, τ>.
type Lineage struct {
	ParentID    string `json:"parentId,omitempty"`
	ChainDigest string `json:"chainDigest,omitempty"`
	Depth       uint64 `json:"depth,omitempty"`
}

// Advance produces the next causal lineage node chained from the current.
func (l Lineage) Advance(stepDigest string) Lineage {
	h := sha256.New()
	h.Write([]byte(l.ChainDigest))
	h.Write([]byte(":"))
	h.Write([]byte(stepDigest))
	return Lineage{
		ParentID:    l.ChainDigest,
		ChainDigest: hex.EncodeToString(h.Sum(nil)),
		Depth:       l.Depth + 1,
	}
}

// StateVector is the canonical state vector S(e,t) = <P, V, C, E, L, τ>.
type StateVector struct {
	Entity    string    `json:"entity"`
	Tau       uint64    `json:"tau"`
	Presence  Presence  `json:"presence"`
	Valence   Valence   `json:"valence"`
	Coherence Coherence `json:"coherence"`
	Evidence  Evidence  `json:"evidence"`
	Lineage   Lineage   `json:"lineage"`
}

// String returns the mathematical notation representation of the state vector.
func (s StateVector) String() string {
	return fmt.Sprintf("S(%s,%d) = <%s, %s, %s, %s, L(%s:%d), %d>",
		s.Entity, s.Tau, s.Presence, s.Valence, s.Coherence, s.Evidence,
		s.Lineage.ChainDigest, s.Lineage.Depth, s.Tau)
}

// Sentinel errors for state vector evaluation.
var (
	ErrInvalidEntity       = errors.New("statevector: entity must not be empty")
	ErrInvalidPresence     = errors.New("statevector: invalid presence state")
	ErrInvalidValence      = errors.New("statevector: invalid valence state")
	ErrInvalidCoherence    = errors.New("statevector: invalid coherence state")
	ErrInvalidEvidence     = errors.New("statevector: invalid evidence state")
	ErrEntityMismatch      = errors.New("statevector: policy entity does not match state vector entity")
	ErrEmptyPolicyEnvelope = errors.New("statevector: policy envelope entity must not be empty")
)

// Validate checks the structural integrity of the state vector.
func (s StateVector) Validate() error {
	if s.Entity == "" {
		return ErrInvalidEntity
	}
	if !s.Presence.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidPresence, s.Presence)
	}
	if !s.Valence.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidValence, s.Valence)
	}
	if !s.Coherence.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidCoherence, s.Coherence)
	}
	if !s.Evidence.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidEvidence, s.Evidence)
	}
	return nil
}

// ConformanceSignal models a weighted conformance observation signal.
type ConformanceSignal struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Weight float64 `json:"weight"`
}

// ConformanceVector models execution context C and its conformance metrics.
type ConformanceVector struct {
	Signals       []ConformanceSignal `json:"signals,omitempty"`
	AntiRelations []AntiRelation      `json:"antiRelations,omitempty"`
	Metadata      map[string]string   `json:"metadata,omitempty"`
}

// PolicyEnvelope Pe defines invariants, thresholds, and governance policies for entity e.
type PolicyEnvelope struct {
	EntityID       string             `json:"entityId"`
	ScoreThreshold float64            `json:"scoreThreshold"`
	HardInvariants []AntiRelationType `json:"hardInvariants,omitempty"`
}

// TransitionInput X specifies proposed transition actions, assertions, and deltas.
type TransitionInput struct {
	Action        string         `json:"action"`
	Digest        string         `json:"digest,omitempty"`
	TargetState   *StateVector   `json:"targetState,omitempty"`
	AntiRelations []AntiRelation `json:"antiRelations,omitempty"`
}

// Observation O records operational events generated during transition.
type Observation struct {
	Timestamp time.Time         `json:"timestamp"`
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Details   map[string]string `json:"details,omitempty"`
}

// EvidenceRecord E records verified attestation and proof tokens.
type EvidenceRecord struct {
	Status      Evidence `json:"status"`
	ProofDigest string   `json:"proofDigest"`
	VerifiedAt  int64    `json:"verifiedAt"`
}

// Residual R records residual risk, invariant breaches, and reason diagnostics.
type Residual struct {
	Score          float64        `json:"score"`
	Dominated      bool           `json:"dominated"`
	HardViolations []AntiRelation `json:"hardViolations,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
	Reason         string         `json:"reason"`
}

// TransitionResult represents the output 5-tuple <d, O, S1, E, R>.
type TransitionResult struct {
	Decision    Decision       `json:"decision"`
	Observation Observation    `json:"observation"`
	NextState   StateVector    `json:"nextState"`
	Evidence    EvidenceRecord `json:"evidence"`
	Residual    Residual       `json:"residual"`
}

// Tuple returns the canonical five elements <d, O, S1, E, R>.
func (tr TransitionResult) Tuple() (Decision, Observation, StateVector, EvidenceRecord, Residual) {
	return tr.Decision, tr.Observation, tr.NextState, tr.Evidence, tr.Residual
}

// classifyAntiRelations partitions anti-relations into hard and soft violations against Pe.
func classifyAntiRelations(pe PolicyEnvelope, allRelations []AntiRelation) ([]AntiRelation, []AntiRelation) {
	hardSet := make(map[AntiRelationType]bool)
	for _, inv := range pe.HardInvariants {
		hardSet[inv] = true
	}
	var hardViolations, softViolations []AntiRelation
	for _, rel := range allRelations {
		if rel.Hard || hardSet[rel.Type] {
			hardViolations = append(hardViolations, rel)
		} else {
			softViolations = append(softViolations, rel)
		}
	}
	return hardViolations, softViolations
}

// computeBaseScore calculates the weighted score of conformance signals.
func computeBaseScore(signals []ConformanceSignal) float64 {
	if len(signals) == 0 {
		return 1.0
	}
	var totalWeight, weightedSum float64
	for _, sig := range signals {
		w := sig.Weight
		if w <= 0 {
			w = 1.0
		}
		totalWeight += w
		s := sig.Score
		if s < 0 {
			s = 0
		} else if s > 1.0 {
			s = 1.0
		}
		weightedSum += s * w
	}
	if totalWeight == 0 {
		return 0.0
	}
	return weightedSum / totalWeight
}

// applyAntiInvariantDominance transforms the state vector under hard invariant collapse.
func applyAntiInvariantDominance(s0 StateVector, nextLineage Lineage, violations []AntiRelation) StateVector {
	s1 := StateVector{
		Entity:    s0.Entity,
		Tau:       s0.Tau + 1,
		Presence:  s0.Presence,
		Valence:   ValenceNegative,
		Coherence: CoherenceDecoherent,
		Evidence:  EvidenceContradicted,
		Lineage:   nextLineage,
	}
	for _, v := range violations {
		if v.Type == AntiRelationInvalidates {
			s1.Presence = PresenceRedacted
		}
	}
	return s1
}

// buildDominatedOutcome constructs the dominated transition result.
func buildDominatedOutcome(s0 StateVector, x TransitionInput, nextLineage Lineage, hardViolations []AntiRelation) TransitionResult {
	s1 := applyAntiInvariantDominance(s0, nextLineage, hardViolations)
	primary := hardViolations[0]
	reason := fmt.Sprintf("Hard anti-invariant %s dominated composite score: %s", primary.Type, primary.Reason)

	return TransitionResult{
		Decision: DecisionReject,
		Observation: Observation{
			Timestamp: time.Now().UTC(),
			Code:      "ANTI_INVARIANT_DOMINATED",
			Message:   reason,
			Details:   map[string]string{"primary_violation": string(primary.Type)},
		},
		NextState: s1,
		Evidence: EvidenceRecord{
			Status:      EvidenceContradicted,
			ProofDigest: nextLineage.ChainDigest,
			VerifiedAt:  time.Now().UTC().Unix(),
		},
		Residual: Residual{
			Score:          0.0,
			Dominated:      true,
			HardViolations: hardViolations,
			Reason:         reason,
		},
	}
}

// buildConformingOutcome constructs the next state and result when conformance succeeds.
func buildConformingOutcome(s0 StateVector, x TransitionInput, nextLineage Lineage, score float64, soft []AntiRelation) TransitionResult {
	s1 := StateVector{
		Entity:    s0.Entity,
		Tau:       s0.Tau + 1,
		Presence:  PresencePresent,
		Valence:   ValencePositive,
		Coherence: CoherenceCoherent,
		Evidence:  EvidenceVerified,
		Lineage:   nextLineage,
	}
	if x.TargetState != nil {
		if x.TargetState.Presence != "" {
			s1.Presence = x.TargetState.Presence
		}
		if x.TargetState.Valence != "" {
			s1.Valence = x.TargetState.Valence
		}
	}
	var warnings []string
	for _, sv := range soft {
		warnings = append(warnings, fmt.Sprintf("Soft anti-relation %s: %s", sv.Type, sv.Reason))
	}
	return TransitionResult{
		Decision: DecisionAccept,
		Observation: Observation{
			Timestamp: time.Now().UTC(),
			Code:      "TRANSITION_ACCEPTED",
			Message:   "Conformance evaluation succeeded",
		},
		NextState: s1,
		Evidence: EvidenceRecord{
			Status:      s1.Evidence,
			ProofDigest: nextLineage.ChainDigest,
			VerifiedAt:  time.Now().UTC().Unix(),
		},
		Residual: Residual{
			Score:    score,
			Warnings: warnings,
			Reason:   "Conformance threshold satisfied",
		},
	}
}

// buildNonConformingOutcome constructs the rejected result when conformance score fails threshold.
func buildNonConformingOutcome(s0 StateVector, x TransitionInput, nextLineage Lineage, score float64, reason string) TransitionResult {
	s1 := StateVector{
		Entity:    s0.Entity,
		Tau:       s0.Tau + 1,
		Presence:  s0.Presence,
		Valence:   ValenceNeutral,
		Coherence: CoherencePartial,
		Evidence:  EvidenceUnverified,
		Lineage:   nextLineage,
	}
	return TransitionResult{
		Decision: DecisionReject,
		Observation: Observation{
			Timestamp: time.Now().UTC(),
			Code:      "THRESHOLD_UNMET",
			Message:   reason,
		},
		NextState: s1,
		Evidence: EvidenceRecord{
			Status:      EvidenceUnverified,
			ProofDigest: nextLineage.ChainDigest,
			VerifiedAt:  time.Now().UTC().Unix(),
		},
		Residual: Residual{
			Score:  score,
			Reason: reason,
		},
	}
}

// Transition evaluates δ(Pe, S0, X, C) = <d, O, S1, E, R>.
// Hard anti-invariants strictly dominate scores regardless of metric signals.
func Transition(pe PolicyEnvelope, s0 StateVector, x TransitionInput, c ConformanceVector) (TransitionResult, error) {
	if pe.EntityID == "" {
		return TransitionResult{}, ErrEmptyPolicyEnvelope
	}
	if err := s0.Validate(); err != nil {
		return TransitionResult{}, fmt.Errorf("initial state invalid: %w", err)
	}
	if pe.EntityID != s0.Entity {
		return TransitionResult{}, fmt.Errorf("%w: policy for %q, state for %q", ErrEntityMismatch, pe.EntityID, s0.Entity)
	}

	var allRelations []AntiRelation
	allRelations = append(allRelations, x.AntiRelations...)
	allRelations = append(allRelations, c.AntiRelations...)

	stepDigest := x.Digest
	if stepDigest == "" {
		stepDigest = x.Action
	}
	nextLineage := s0.Lineage.Advance(stepDigest)

	hardViolations, softViolations := classifyAntiRelations(pe, allRelations)
	if len(hardViolations) > 0 {
		return buildDominatedOutcome(s0, x, nextLineage, hardViolations), nil
	}

	baseScore := computeBaseScore(c.Signals)
	// Soft violations penalize score by 0.20 per occurrence, bounded at 0.
	penalty := float64(len(softViolations)) * 0.20
	finalScore := baseScore - penalty
	if finalScore < 0 {
		finalScore = 0
	}

	threshold := pe.ScoreThreshold
	if threshold <= 0 {
		threshold = 0.50
	}

	if finalScore < threshold {
		reason := fmt.Sprintf("Conformance score %.2f is below threshold %.2f", finalScore, threshold)
		return buildNonConformingOutcome(s0, x, nextLineage, finalScore, reason), nil
	}

	return buildConformingOutcome(s0, x, nextLineage, finalScore, softViolations), nil
}
