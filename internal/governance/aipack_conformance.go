/*
Copyright 2026 CKodex Authors.
*/

package governance

import (
	"context"
	"errors"
	"fmt"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
)

// AIPackConformanceResult is the outcome of an AIPack governance check.
type AIPackConformanceResult struct {
	Valid   bool
	Code    string
	Message string
}

// AIPackValidator defines the interface for AIPack governance validators.
type AIPackValidator interface {
	ValidateAIPack(ctx context.Context, pack *servingv1alpha2.AIPack) AIPackConformanceResult
}

// AIPackKindValidator rejects unknown kinds (AIPACK-KIND-000) and
// family mismatches per §3.5 (AIPACK-KIND-001).
type AIPackKindValidator struct{}

func (v *AIPackKindValidator) ValidateAIPack(_ context.Context, pack *servingv1alpha2.AIPack) AIPackConformanceResult {
	if err := aipack.ValidateKind(pack.Spec.Kind); err != nil {
		code := "AIPACK-KIND-000"
		var aipErr *aipack.AIPackError
		if errors.As(err, &aipErr) {
			code = string(aipErr.Code)
		}
		return AIPackConformanceResult{Valid: false, Code: code, Message: err.Error()}
	}

	if pack.Spec.Family != nil {
		expected, ok := aipack.FamilyForKind(pack.Spec.Kind)
		if !ok {
			return AIPackConformanceResult{Valid: false, Code: "AIPACK-KIND-000", Message: fmt.Sprintf("kind %q has no canonical family mapping", pack.Spec.Kind)}
		}
		if *pack.Spec.Family != expected {
			return AIPackConformanceResult{
				Valid:   false,
				Code:    "AIPACK-KIND-001",
				Message: fmt.Sprintf("family %q does not match canonical family %q for kind %q (AIPACK-SPEC §3.5)", *pack.Spec.Family, expected, pack.Spec.Kind),
			}
		}
	}

	return AIPackConformanceResult{Valid: true}
}

// AIPackCompositionValidator runs composition.ValidateRef + ValidateComposition
// for Agent (C1) artifacts. Non-Agent kinds skip composition checks.
type AIPackCompositionValidator struct{}

func (v *AIPackCompositionValidator) ValidateAIPack(_ context.Context, pack *servingv1alpha2.AIPack) AIPackConformanceResult {
	if pack.Spec.Source.Ref != "" {
		if err := aipack.ValidateRef(pack.Spec.Source.Ref); err != nil {
			return AIPackConformanceResult{Valid: false, Code: "AIPACK-COMP-001", Message: err.Error()}
		}
	}

	if pack.Spec.Kind != servingv1alpha2.KindAgent || pack.Spec.Composition == nil {
		return AIPackConformanceResult{Valid: true}
	}

	if err := aipack.ValidateComposition(pack.Spec.Composition); err != nil {
		code := "AIPACK-COMP-004"
		var aipErr *aipack.AIPackError
		if errors.As(err, &aipErr) {
			code = string(aipErr.Code)
		}
		return AIPackConformanceResult{Valid: false, Code: code, Message: err.Error()}
	}

	return AIPackConformanceResult{Valid: true}
}

// AIPackAttestationValidator verifies that all required predicates for the
// artifact's kind are present in its attestation block.
type AIPackAttestationValidator struct{}

func (v *AIPackAttestationValidator) ValidateAIPack(_ context.Context, pack *servingv1alpha2.AIPack) AIPackConformanceResult {
	required := aipack.RequiredPredicates(pack.Spec.Kind)

	if pack.Spec.Attestation == nil {
		if len(required) > 0 {
			return AIPackConformanceResult{
				Valid:   false,
				Code:    "AIPACK-ATTEST-001",
				Message: fmt.Sprintf("kind %q requires %d attestation predicate(s) but attestation block is absent", pack.Spec.Kind, len(required)),
			}
		}
		return AIPackConformanceResult{Valid: true}
	}

	present := predicateURIs(pack.Spec.Attestation)
	missing := aipack.MissingPredicates(pack.Spec.Kind, present)
	if len(missing) > 0 {
		return AIPackConformanceResult{
			Valid:   false,
			Code:    "AIPACK-ATTEST-002",
			Message: fmt.Sprintf("kind %q is missing required predicates: %v", pack.Spec.Kind, missing),
		}
	}

	return AIPackConformanceResult{Valid: true}
}

// predicateURIs extracts the PredicateURI strings from an attestation block.
func predicateURIs(a *servingv1alpha2.AIPackAttestation) []string {
	if a == nil {
		return nil
	}
	out := make([]string, 0, len(a.Predicates))
	for _, p := range a.Predicates {
		out = append(out, p.PredicateURI)
	}
	return out
}

// AIPackConformanceEngine aggregates all AIPack governance checks.
type AIPackConformanceEngine struct {
	Validators []AIPackValidator
}

// NewDefaultAIPackEngine returns an engine with all three validators wired in.
func NewDefaultAIPackEngine() *AIPackConformanceEngine {
	return &AIPackConformanceEngine{
		Validators: []AIPackValidator{
			&AIPackKindValidator{},
			&AIPackCompositionValidator{},
			&AIPackAttestationValidator{},
		},
	}
}

// Check runs all validators and returns on the first failure.
// Returns (valid, errorCode, message).
func (e *AIPackConformanceEngine) Check(ctx context.Context, pack *servingv1alpha2.AIPack) (bool, string, string) {
	for _, v := range e.Validators {
		res := v.ValidateAIPack(ctx, pack)
		if !res.Valid {
			return false, res.Code, res.Message
		}
	}
	return true, "", "All AIPack conformance vectors passed"
}
