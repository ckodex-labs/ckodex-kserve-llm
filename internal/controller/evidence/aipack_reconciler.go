/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package evidence

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/governance"
)

// ReconcileAIPacks updates NIST SR-2 compliance conditions on an
// LLMInferenceService based on the attestation state of associated AIPack
// artifacts. It mirrors the pattern in Reconcile() for LoRA adapters.
//
// The function does not patch the CR; callers must patch after calling this.
func (g *GovernanceReconciler) ReconcileAIPacks(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, packs []servingv1alpha2.AIPack) error {
	logger := log.FromContext(ctx)

	totalPacks := 0
	verifiedPacks := 0

	for i := range packs {
		pack := &packs[i]
		totalPacks++

		hasRequired := governance.HasRequiredAIPackAttestations(pack.Spec.Kind, pack.Spec.Attestation)
		if !hasRequired {
			logger.Info("AIPack missing required attestation predicates",
				"name", pack.Name,
				"namespace", pack.Namespace,
				"kind", pack.Spec.Kind,
			)
			continue
		}

		result, err := governance.VerifyAIPackAttestation(ctx, pack.Spec.Kind, pack.Spec.Source.Ref, pack.Spec.Attestation)
		if err != nil {
			return fmt.Errorf("verify aipack attestation %s/%s: %w", pack.Namespace, pack.Name, err)
		}

		if result.Verified {
			verifiedPacks++
		} else {
			logger.Info("AIPack attestation verification failed",
				"name", pack.Name,
				"kind", pack.Spec.Kind,
				"failedPredicates", result.FailedPredicates,
				"message", result.Message,
			)
		}
	}

	aipackSR2 := buildAIPackSR2Condition(totalPacks, verifiedPacks)
	meta.SetStatusCondition(&llmSvc.Status.Conditions, aipackSR2)

	logger.Info("Updated AIPack governance evidence", "total", totalPacks, "verified", verifiedPacks)
	return nil
}

func buildAIPackSR2Condition(total, verified int) metav1.Condition {
	c := metav1.Condition{
		Type:               "Compliance-SR-2-AIPack",
		LastTransitionTime: metav1.Now(),
	}

	switch {
	case total == 0:
		c.Status = metav1.ConditionTrue
		c.Reason = "NoAIPacksAssociated"
		c.Message = "No AIPack artifacts are associated with this workload"
	case verified == total:
		c.Status = metav1.ConditionTrue
		c.Reason = "AllAIPacksAttested"
		c.Message = fmt.Sprintf("All %d associated AIPack artifact(s) have verified attestation predicates", total)
	default:
		c.Status = metav1.ConditionFalse
		c.Reason = "AIPackAttestationIncomplete"
		c.Message = fmt.Sprintf("%d of %d AIPack artifact(s) lack required attestation predicates", total-verified, total)
	}

	return c
}
