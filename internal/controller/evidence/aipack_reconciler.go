/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package evidence

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

		// Auto-create LLMLoraAdapter CRs from composition.adapters (Agent kind only).
		if pack.Spec.Kind == servingv1alpha2.KindAgent {
			if err := g.ReconcileAdapters(ctx, pack, llmSvc); err != nil {
				logger.Error(err, "failed to reconcile adapters from AIPack composition (non-blocking)", "pack", pack.Name)
			}
		}
	}

	aipackSR2 := buildAIPackSR2Condition(totalPacks, verifiedPacks)
	meta.SetStatusCondition(&llmSvc.Status.Conditions, aipackSR2)

	logger.Info("Updated AIPack governance evidence", "total", totalPacks, "verified", verifiedPacks)
	return nil
}

// ReconcileAdapters creates LLMLoraAdapter CRs for each adapter slot in
// pack.Spec.Composition.Adapters. Owner refs point to the AIPack so GC cascade
// fires automatically when the AIPack is deleted. Existing CRs are left untouched
// — the hot-load controller owns their lifecycle after creation.
//
// CR name pattern: {pack.Name}-lora-{index}  (stable, namespace-unique)
// AdapterName:     {pack.Name}-{digest-suffix-8}  (usable as vLLM logical name)
func (g *GovernanceReconciler) ReconcileAdapters(
	ctx context.Context,
	pack *servingv1alpha2.AIPack,
	llmSvc *servingv1alpha2.LLMInferenceService,
) error {
	if pack.Spec.Composition == nil || len(pack.Spec.Composition.Adapters) == 0 {
		return nil
	}
	if g.Scheme == nil {
		return fmt.Errorf("ReconcileAdapters: Scheme is nil — wire GovernanceReconciler.Scheme in SetupWithManager")
	}
	logger := log.FromContext(ctx)

	for i, ref := range pack.Spec.Composition.Adapters {
		crName := fmt.Sprintf("%s-lora-%d", pack.Name, i)
		adapterName := fmt.Sprintf("%s-%s", pack.Name, digestSuffix(ref.Ref))

		desired := &servingv1alpha2.LLMLoraAdapter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName,
				Namespace: pack.Namespace,
			},
			Spec: servingv1alpha2.LLMLoraAdapterSpec{
				TargetService: llmSvc.Name,
				AdapterName:   adapterName,
				Model: servingv1alpha2.ModelSpec{
					URI:  ref.Ref,
					Name: adapterName,
				},
			},
		}
		if err := controllerutil.SetControllerReference(pack, desired, g.Scheme); err != nil {
			return fmt.Errorf("set owner ref on LLMLoraAdapter %s: %w", crName, err)
		}

		var existing servingv1alpha2.LLMLoraAdapter
		err := g.Client.Get(ctx, types.NamespacedName{Name: crName, Namespace: pack.Namespace}, &existing)
		if apierrors.IsNotFound(err) {
			if createErr := g.Client.Create(ctx, desired); createErr != nil {
				return fmt.Errorf("create LLMLoraAdapter %s: %w", crName, createErr)
			}
			logger.Info("Created LLMLoraAdapter from AIPack composition", "cr", crName, "adapter", adapterName)
			continue
		}
		if err != nil {
			return fmt.Errorf("get LLMLoraAdapter %s: %w", crName, err)
		}
		// Already exists — hot-load controller manages the rest; no update needed.
	}
	return nil
}

// digestSuffix returns the last 8 characters of an OCI digest for use in adapter names.
// Input: "registry/image@sha256:abcdef0123456789..." → "01234567" (last 8 of the hex).
func digestSuffix(ref string) string {
	const suffixLen = 8
	// sha256 digest is after the last ':'
	if idx := strings.LastIndex(ref, ":"); idx >= 0 && idx+1+suffixLen <= len(ref) {
		hex := ref[idx+1:]
		if len(hex) >= suffixLen {
			return hex[len(hex)-suffixLen:]
		}
	}
	// Fallback: last 8 chars of the whole ref
	if len(ref) >= suffixLen {
		return ref[len(ref)-suffixLen:]
	}
	return ref
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
