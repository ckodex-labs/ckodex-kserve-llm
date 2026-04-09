/*
Copyright 2026 CKodex Authors.
*/

package governance

import (
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AggregateStatePlanes calculates the effective state planes for a composite model system.
// Logic:
// - Lifecycle: Active only if ALL components are active/verified.
// - Trust: The minimum trust level among all components (foundation + adapters).
// - Risk: The maximum risk level among all components.
func AggregateStatePlanes(foundation *servingv1alpha2.LLMInferenceService, adapters []servingv1alpha2.LLMLoraAdapter) servingv1alpha2.StatePlanes {
	effective := servingv1alpha2.StatePlanes{
		Lifecycle: "active",
		Trust:     "trusted", // Start with highest, downgrade as we encounter weaker trust
		Risk:      "normal",
	}

	// Trust hierarchy
	trustScores := map[string]int{
		"denied":    -1,
		"unknown":   0,
		"asserted":  1,
		"verified":  2,
		"trusted":   3,
	}

	scoreToTrust := map[int]string{
		-1: "denied",
		0:  "unknown",
		1:  "asserted",
		2:  "verified",
		3:  "trusted",
	}

	currentTrustScore := trustScores["asserted"] // Default to asserted if ready

	// Check Foundation Trust via Governance Conditions
	for _, cond := range foundation.Status.Conditions {
		if cond.Type == "Compliance-AC-4" {
			if cond.Status == metav1.ConditionTrue && cond.Reason == "DPIVerified" {
				currentTrustScore = trustScores["verified"]
			} else if cond.Status == metav1.ConditionFalse {
				currentTrustScore = trustScores["denied"]
			}
			break
		}
	}

	for _, a := range adapters {
		// Lifecycle
		if a.Status.StatePlanes.Lifecycle != "active" {
			effective.Lifecycle = "quarantined"
		}

		// Trust
		adapterTrust := a.Status.StatePlanes.Trust
		if adapterTrust == "" {
			adapterTrust = "unknown"
		}
		if trustScores[adapterTrust] < currentTrustScore {
			currentTrustScore = trustScores[adapterTrust]
		}

		// Risk
		if a.Status.StatePlanes.Risk == "quarantined" || a.Status.StatePlanes.Risk == "evaluating" {
			effective.Risk = "high"
		}
	}

	effective.Trust = scoreToTrust[currentTrustScore]
	
	// If any component is quarantined, the composite is quarantined.
	if effective.Lifecycle == "quarantined" {
		effective.Trust = "denied"
	}

	return effective
}
