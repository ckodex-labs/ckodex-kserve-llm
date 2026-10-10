/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package statevector_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ckodex-labs/kserve-llm-operator/internal/kernel/statevector"
)

func sampleInitialState() statevector.StateVector {
	return statevector.StateVector{
		Entity:    "model-pack-alpha",
		Tau:       1,
		Presence:  statevector.PresencePresent,
		Valence:   statevector.ValencePositive,
		Coherence: statevector.CoherenceCoherent,
		Evidence:  statevector.EvidenceVerified,
		Lineage: statevector.Lineage{
			ChainDigest: "init-sha256",
			Depth:       1,
		},
	}
}

func samplePolicy() statevector.PolicyEnvelope {
	return statevector.PolicyEnvelope{
		EntityID:       "model-pack-alpha",
		ScoreThreshold: 0.70,
		HardInvariants: []statevector.AntiRelationType{
			statevector.AntiRelationInvalidates,
			statevector.AntiRelationContradicts,
		},
	}
}

func TestStateVector_Validation(t *testing.T) {
	s := sampleInitialState()
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid state vector, got %v", err)
	}

	invalidEntity := s
	invalidEntity.Entity = ""
	if err := invalidEntity.Validate(); !errors.Is(err, statevector.ErrInvalidEntity) {
		t.Fatalf("expected ErrInvalidEntity, got %v", err)
	}

	invalidPres := s
	invalidPres.Presence = "BOGUS"
	if err := invalidPres.Validate(); !errors.Is(err, statevector.ErrInvalidPresence) {
		t.Fatalf("expected ErrInvalidPresence, got %v", err)
	}

	invalidVal := s
	invalidVal.Valence = "BOGUS"
	if err := invalidVal.Validate(); !errors.Is(err, statevector.ErrInvalidValence) {
		t.Fatalf("expected ErrInvalidValence, got %v", err)
	}

	invalidCoh := s
	invalidCoh.Coherence = "BOGUS"
	if err := invalidCoh.Validate(); !errors.Is(err, statevector.ErrInvalidCoherence) {
		t.Fatalf("expected ErrInvalidCoherence, got %v", err)
	}

	invalidEv := s
	invalidEv.Evidence = "BOGUS"
	if err := invalidEv.Validate(); !errors.Is(err, statevector.ErrInvalidEvidence) {
		t.Fatalf("expected ErrInvalidEvidence, got %v", err)
	}
}

func TestStateVector_StringAndLineageAdvance(t *testing.T) {
	s0 := sampleInitialState()
	str := s0.String()
	if !strings.Contains(str, "S(model-pack-alpha,1)") || !strings.Contains(str, "PRESENT") {
		t.Errorf("unexpected string representation: %s", str)
	}

	l1 := s0.Lineage.Advance("delta-action")
	if l1.Depth != 2 {
		t.Errorf("expected depth 2, got %d", l1.Depth)
	}
	if l1.ParentID != s0.Lineage.ChainDigest {
		t.Errorf("expected ParentID %s, got %s", s0.Lineage.ChainDigest, l1.ParentID)
	}
	if l1.ChainDigest == "" {
		t.Error("expected non-empty chained digest")
	}
}

func TestTransition_SuccessContract(t *testing.T) {
	pe := samplePolicy()
	s0 := sampleInitialState()
	x := statevector.TransitionInput{
		Action: "promote-service",
		Digest: "sha256:deploy-step",
	}
	c := statevector.ConformanceVector{
		Signals: []statevector.ConformanceSignal{
			{Name: "health", Score: 1.0, Weight: 1.0},
			{Name: "accuracy", Score: 0.95, Weight: 1.0},
		},
	}

	res, err := statevector.Transition(pe, s0, x, c)
	if err != nil {
		t.Fatalf("transition failed unexpectedly: %v", err)
	}

	d, o, s1, e, r := res.Tuple()
	if d != statevector.DecisionAccept || s1.Tau != s0.Tau+1 {
		t.Fatalf("unexpected decision or tau: d=%v, tau=%d", d, s1.Tau)
	}
	if s1.Lineage.Depth != s0.Lineage.Depth+1 || e.Status != statevector.EvidenceVerified {
		t.Errorf("unexpected lineage or evidence: depth=%d, ev=%v", s1.Lineage.Depth, e.Status)
	}
	if r.Score < 0.90 || r.Dominated || o.Code != "TRANSITION_ACCEPTED" {
		t.Errorf("unexpected residual or observation: score=%.2f, dom=%v, code=%s", r.Score, r.Dominated, o.Code)
	}
}

func assertInvalidatesDominated(t *testing.T, res statevector.TransitionResult) {
	t.Helper()
	d, o, s1, e, r := res.Tuple()
	if d != statevector.DecisionReject {
		t.Errorf("hard anti-invariant must force DecisionReject, got %v", d)
	}
	if e.Status != statevector.EvidenceContradicted || !r.Dominated || r.Score != 0.0 {
		t.Errorf("expected dominated contradiction: ev=%v, dom=%v, score=%f", e.Status, r.Dominated, r.Score)
	}
	if len(r.HardViolations) != 1 {
		t.Fatalf("expected 1 hard violation, got %d", len(r.HardViolations))
	}
	if s1.Presence != statevector.PresenceRedacted || s1.Coherence != statevector.CoherenceDecoherent {
		t.Errorf("expected PresenceRedacted and CoherenceDecoherent, got %v, %v", s1.Presence, s1.Coherence)
	}
	if s1.Evidence != statevector.EvidenceContradicted || o.Code != "ANTI_INVARIANT_DOMINATED" {
		t.Errorf("expected EvidenceContradicted and ANTI_INVARIANT_DOMINATED: ev=%v, code=%s", s1.Evidence, o.Code)
	}
}

func TestTransition_HardAntiInvariant_Invalidates(t *testing.T) {
	pe := samplePolicy()
	s0 := sampleInitialState()
	x := statevector.TransitionInput{
		Action: "runtime-verify",
		AntiRelations: []statevector.AntiRelation{
			{
				Type:   statevector.AntiRelationInvalidates,
				Source: "revocation-service",
				Target: s0.Entity,
				Reason: "Root key compromise",
				Hard:   true,
			},
		},
	}
	// Even with perfect signals (1.0), hard anti-invariant must dominate!
	c := statevector.ConformanceVector{
		Signals: []statevector.ConformanceSignal{
			{Name: "perfect-metric-1", Score: 1.0, Weight: 10.0},
			{Name: "perfect-metric-2", Score: 1.0, Weight: 10.0},
		},
	}

	res, err := statevector.Transition(pe, s0, x, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvalidatesDominated(t, res)
}

func TestTransition_HardAntiInvariant_Contradicts(t *testing.T) {
	pe := samplePolicy()
	s0 := sampleInitialState()
	// CONTRADICTS is in pe.HardInvariants, so even if rel.Hard is false, policy forces it hard.
	c := statevector.ConformanceVector{
		AntiRelations: []statevector.AntiRelation{
			{
				Type:   statevector.AntiRelationContradicts,
				Source: "formal-verification",
				Target: s0.Entity,
				Reason: "Mutual state exclusion detected",
			},
		},
	}
	x := statevector.TransitionInput{Action: "sync-state"}

	res, err := statevector.Transition(pe, s0, x, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Decision != statevector.DecisionReject || !res.Residual.Dominated {
		t.Errorf("CONTRADICTS must reject and dominate: d=%v, dom=%v", res.Decision, res.Residual.Dominated)
	}
	if res.NextState.Coherence != statevector.CoherenceDecoherent {
		t.Errorf("expected CoherenceDecoherent, got %v", res.NextState.Coherence)
	}
	if res.NextState.Evidence != statevector.EvidenceContradicted {
		t.Errorf("expected EvidenceContradicted, got %v", res.NextState.Evidence)
	}
}

func TestTransition_HardAntiInvariant_Attacks(t *testing.T) {
	pe := samplePolicy()
	s0 := sampleInitialState()
	x := statevector.TransitionInput{
		Action: "audit",
		AntiRelations: []statevector.AntiRelation{
			{
				Type:   statevector.AntiRelationAttacks,
				Source: "adversarial-probe",
				Target: s0.Entity,
				Reason: "Evasion detected",
				Hard:   true,
			},
		},
	}

	res, err := statevector.Transition(pe, s0, x, statevector.ConformanceVector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Decision != statevector.DecisionReject {
		t.Errorf("ATTACKS must reject, got %v", res.Decision)
	}
	if res.NextState.Valence != statevector.ValenceNegative {
		t.Errorf("ATTACKS must force ValenceNegative, got %v", res.NextState.Valence)
	}
}

func TestTransition_SoftAntiRelation_PenalizesScore(t *testing.T) {
	pe := samplePolicy()
	s0 := sampleInitialState()
	// Soft ATTACKS relation (pe only defines INVALIDATES and CONTRADICTS as hard).
	x := statevector.TransitionInput{
		Action: "evaluate",
		AntiRelations: []statevector.AntiRelation{
			{
				Type:   statevector.AntiRelationAttacks,
				Source: "lint-warning",
				Target: s0.Entity,
				Reason: "Non-critical syntax ambiguity",
				Hard:   false,
			},
		},
	}
	// High base signals: 0.95. Soft penalty of 0.20 leaves 0.75 >= threshold 0.70.
	c := statevector.ConformanceVector{
		Signals: []statevector.ConformanceSignal{
			{Name: "benchmark", Score: 0.95, Weight: 1.0},
		},
	}

	res, err := statevector.Transition(pe, s0, x, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Decision != statevector.DecisionAccept || res.Residual.Dominated {
		t.Errorf("soft violation with score >= threshold should accept: d=%v, dom=%v", res.Decision, res.Residual.Dominated)
	}
	if len(res.Residual.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(res.Residual.Warnings))
	}
}

func TestTransition_BelowThresholdRejection(t *testing.T) {
	pe := samplePolicy() // threshold 0.70
	s0 := sampleInitialState()
	x := statevector.TransitionInput{Action: "verify"}
	c := statevector.ConformanceVector{
		Signals: []statevector.ConformanceSignal{
			{Name: "benchmark", Score: 0.50, Weight: 1.0},
		},
	}

	res, err := statevector.Transition(pe, s0, x, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Decision != statevector.DecisionReject || res.Residual.Dominated {
		t.Errorf("score below threshold must reject without domination: d=%v, dom=%v", res.Decision, res.Residual.Dominated)
	}
	if res.NextState.Coherence != statevector.CoherencePartial {
		t.Errorf("expected CoherencePartial, got %v", res.NextState.Coherence)
	}
}

func TestTransition_EntityMismatchAndValidation(t *testing.T) {
	pe := samplePolicy()
	s0 := sampleInitialState()
	s0.Entity = "different-entity"

	_, err := statevector.Transition(pe, s0, statevector.TransitionInput{}, statevector.ConformanceVector{})
	if !errors.Is(err, statevector.ErrEntityMismatch) {
		t.Fatalf("expected ErrEntityMismatch, got %v", err)
	}

	peEmpty := samplePolicy()
	peEmpty.EntityID = ""
	_, err = statevector.Transition(peEmpty, sampleInitialState(), statevector.TransitionInput{}, statevector.ConformanceVector{})
	if !errors.Is(err, statevector.ErrEmptyPolicyEnvelope) {
		t.Fatalf("expected ErrEmptyPolicyEnvelope, got %v", err)
	}
}
