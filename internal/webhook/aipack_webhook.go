/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package webhook

import (
	"context"
	"fmt"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
)

// SetupAIPackWebhook registers the AIPack validating webhook with the manager.
// Call this from the manager setup alongside SetupWebhooks.
func SetupAIPackWebhook(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &servingv1alpha2.AIPack{}).
		WithValidator(&AIPackValidator{}).
		Complete()
}

// AIPackValidator is the validating webhook for AIPack resources.
// It enforces:
//   - Kind is one of the 15 normative kinds (AIPACK-KIND-000)
//   - Declared family matches §3.5 canonical mapping (AIPACK-KIND-001)
//   - Source.Ref contains a sha256 digest when non-empty (AIPACK-COMP-001)
//   - Composition slot constraints for Agent (C1) (AIPACK-COMP-004)
//   - Kind-specific required field presence
type AIPackValidator struct{}

// ValidateCreate validates a new AIPack resource.
func (v *AIPackValidator) ValidateCreate(_ context.Context, obj *servingv1alpha2.AIPack) (admission.Warnings, error) {
	return v.validate(obj)
}

// ValidateUpdate validates an updated AIPack resource.
func (v *AIPackValidator) ValidateUpdate(_ context.Context, _ *servingv1alpha2.AIPack, newObj *servingv1alpha2.AIPack) (admission.Warnings, error) {
	return v.validate(newObj)
}

// ValidateDelete is a no-op — deletion of AIPack resources is always permitted.
func (v *AIPackValidator) ValidateDelete(_ context.Context, _ *servingv1alpha2.AIPack) (admission.Warnings, error) {
	return nil, nil
}

func (v *AIPackValidator) validate(pack *servingv1alpha2.AIPack) (admission.Warnings, error) {
	var errs []string

	errs = append(errs, validateKind(pack)...)
	errs = append(errs, validateSource(pack)...)
	errs = append(errs, validateComposition(pack)...)
	errs = append(errs, validateKindSpecificFields(pack)...)

	if len(errs) > 0 {
		return nil, fmt.Errorf("AIPack %s/%s validation failed: %s", pack.Namespace, pack.Name, strings.Join(errs, "; "))
	}
	return nil, nil
}

func validateKind(pack *servingv1alpha2.AIPack) []string {
	var errs []string

	if err := aipack.ValidateKind(pack.Spec.Kind); err != nil {
		errs = append(errs, err.Error())
		return errs
	}

	if pack.Spec.Family != nil {
		expected, ok := aipack.FamilyForKind(pack.Spec.Kind)
		if ok && *pack.Spec.Family != expected {
			errs = append(errs, fmt.Sprintf("[AIPACK-KIND-001] spec.family %q does not match canonical family %q for kind %q", *pack.Spec.Family, expected, pack.Spec.Kind))
		}
	}

	return errs
}

func validateSource(pack *servingv1alpha2.AIPack) []string {
	var errs []string

	if pack.Spec.Source.Ref == "" {
		if pack.Spec.Kind != servingv1alpha2.KindAgent {
			errs = append(errs, "[AIPACK-COMP-001] spec.source.ref is required for non-Agent artifact kinds")
		}
		return errs
	}

	if err := aipack.ValidateRef(pack.Spec.Source.Ref); err != nil {
		errs = append(errs, err.Error())
	}

	return errs
}

func validateComposition(pack *servingv1alpha2.AIPack) []string {
	if pack.Spec.Kind != servingv1alpha2.KindAgent {
		if pack.Spec.Composition != nil {
			return []string{"[AIPACK-KIND-000] spec.composition is only valid for Agent (C1) artifacts"}
		}
		return nil
	}

	if pack.Spec.Composition == nil {
		return []string{"[AIPACK-COMP-004] Agent (C1) artifacts must include spec.composition"}
	}

	if err := aipack.ValidateComposition(pack.Spec.Composition); err != nil {
		return []string{err.Error()}
	}

	return nil
}

// validateKindSpecificFields enforces kind-specific required field presence.
func validateKindSpecificFields(pack *servingv1alpha2.AIPack) []string {
	var errs []string

	switch pack.Spec.Kind {
	case servingv1alpha2.KindLoRA:
		if pack.Spec.LoRA == nil || pack.Spec.LoRA.BaseRef == "" {
			errs = append(errs, "[AIPACK-COMPAT-001] LoRA artifacts require spec.lora.baseRef pointing to the base model")
		}
	case servingv1alpha2.KindFineTune:
		if pack.Spec.FineTune == nil || pack.Spec.FineTune.BaseRef == "" {
			errs = append(errs, "[AIPACK-COMPAT-001] FineTune artifacts require spec.fineTune.baseRef pointing to the base model")
		}
	case servingv1alpha2.KindRetrievalIndex:
		if pack.Spec.RetrievalIndex == nil || pack.Spec.RetrievalIndex.EmbeddingModel == "" {
			errs = append(errs, "[AIPACK-COMPAT-002] RetrievalIndex artifacts require spec.retrievalIndex.embeddingModel")
		}
	case servingv1alpha2.KindEval:
		if pack.Spec.Eval == nil || pack.Spec.Eval.HarnessRef == "" {
			errs = append(errs, "Eval artifacts require spec.eval.harnessRef")
		}
	}

	return errs
}
