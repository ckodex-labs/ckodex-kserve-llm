/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package evidence

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/governance"
)

func (g *GovernanceReconciler) buildComplianceConditions(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, activeLoras []servingv1alpha2.LLMLoraAdapter) ([]metav1.Condition, error) {
	sr2, err := g.buildSR2Condition(ctx, llmSvc, activeLoras)
	if err != nil {
		return nil, err
	}
	return []metav1.Condition{
		unavailableEvidenceCondition("Compliance-AC-4", "Network enforcement evidence is unavailable; ToolSurface configuration alone does not prove enforcement"),
		unavailableEvidenceCondition("Compliance-AU-2", "Durable audit sink evidence is unavailable; configured paths or sink settings alone do not prove persistence"),
		buildSI7Condition(llmSvc, activeLoras),
		unavailableEvidenceCondition("Compliance-SI-4", "Telemetry evidence is unavailable; configuration alone does not prove active monitoring"),
		sr2,
	}, nil
}

func unavailableEvidenceCondition(control, message string) metav1.Condition {
	return metav1.Condition{
		Type:               control,
		Status:             metav1.ConditionUnknown,
		Reason:             "EvidenceUnavailable",
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

func buildSI7Condition(llmSvc *servingv1alpha2.LLMInferenceService, activeLoras []servingv1alpha2.LLMLoraAdapter) metav1.Condition {
	state := governance.AggregateStatePlanes(llmSvc, activeLoras)
	condition := metav1.Condition{
		Type:               "Compliance-SI-7",
		Status:             metav1.ConditionFalse,
		Reason:             "IntegrityUnverified",
		Message:            fmt.Sprintf("Lifecycle: %s, Trust: %s, Risk: %s", state.Lifecycle, state.Trust, state.Risk),
		LastTransitionTime: metav1.Now(),
	}
	if state.Trust == "verified" || state.Trust == "trusted" {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "IntegrityVerified"
	} else if state.Lifecycle == "quarantined" || state.Trust == "denied" {
		condition.Reason = "SecurityBreach"
	}
	return condition
}

func (g *GovernanceReconciler) buildSR2Condition(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, activeLoras []servingv1alpha2.LLMLoraAdapter) (metav1.Condition, error) {
	condition := metav1.Condition{
		Type:               "Compliance-SR-2",
		Status:             metav1.ConditionFalse,
		Reason:             "VerificationPending",
		Message:            "No cryptographic provenance verification result has been recorded for this workload",
		LastTransitionTime: metav1.Now(),
	}
	totalArtifacts, verifiedArtifacts, err := g.countVerifiedArtifacts(ctx, llmSvc, activeLoras)
	if err != nil {
		return metav1.Condition{}, err
	}
	setSR2Result(&condition, totalArtifacts, verifiedArtifacts, g.AirGappedMode, g.LocalCosignKeyPath)
	return condition, nil
}

func (g *GovernanceReconciler) countVerifiedArtifacts(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, activeLoras []servingv1alpha2.LLMLoraAdapter) (int, int, error) {
	total, verified := 0, 0
	if runtimeVerifiableModelURI(llmSvc.Spec.Model.URI) {
		total++
		ok, err := g.baseModelVerificationRecorded(ctx, llmSvc)
		if err != nil {
			return 0, 0, fmt.Errorf("inspect base model verification: %w", err)
		}
		if ok {
			verified++
		}
	}
	for i := range activeLoras {
		total++
		if governance.HasVerifiedSupplyChainEvidence(&activeLoras[i]) {
			verified++
		}
	}
	return total, verified, nil
}

func setSR2Result(condition *metav1.Condition, total, verified int, airGapped bool, keyPath string) {
	switch {
	case total > 0 && verified == total:
		condition.Status = metav1.ConditionTrue
		condition.Reason = "ProvenanceVerified"
		if airGapped {
			condition.Message = "All active artifacts recorded offline provenance verification with a configured local key path"
		} else {
			condition.Message = "All active artifacts recorded cryptographic provenance verification metadata"
		}
	case airGapped && keyPath == "":
		condition.Reason = "OfflineKeyMissing"
		condition.Message = "Air-gapped mode is enabled, but CKODEX_LOCAL_COSIGN_KEY_PATH is not configured"
	case airGapped:
		condition.Reason = "OfflineVerificationPending"
		condition.Message = fmt.Sprintf("Offline key path %q is configured, but the controller has not recorded cryptographic verification for all active artifacts", keyPath)
	case total == 0:
		condition.Reason = "NoVerifiableArtifacts"
		condition.Message = "No active artifact evidence bundles are present to prove runtime supply-chain verification"
	}
}
