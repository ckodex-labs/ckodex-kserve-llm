/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/residency"
)

// onboardingResidencyReadiness projects LLM status into the read-only
// residency policy owned by ModelOnboarding. It does not mutate routes,
// sessions, caches, or runtime workloads.
func onboardingResidencyReadiness(
	llmSvc *servingv1alpha2.LLMInferenceService,
) (residency.Status, residency.Policy, error) {
	requirePrefill := llmSvc.Spec.Prefill != nil
	observation := residency.Observation{
		ArtifactCached: conditionTrue(llmSvc, servingv1alpha2.ConditionModelLoaded) || llmSvc.Status.ModelReady,
		RuntimeLoaded:  conditionTrue(llmSvc, servingv1alpha2.ConditionModelLoaded) || llmSvc.Status.ModelReady,
		RuntimeReady:   llmSvc.Status.ModelReady,
		PrefillReady:   !requirePrefill || conditionTrue(llmSvc, servingv1alpha2.ConditionPrefillReady),
		RouteAttached:  conditionTrue(llmSvc, servingv1alpha2.ConditionGatewayReady),
	}
	observation.AcceptingRequests = observation.RouteAttached && observation.RuntimeReady
	return residency.Project(residency.StateReady, observation, requirePrefill)
}

func ensureOnboardingResidencyReady(llmSvc *servingv1alpha2.LLMInferenceService) error {
	status, policy, err := onboardingResidencyReadiness(llmSvc)
	if err != nil {
		return fmt.Errorf("project model residency: %w", err)
	}
	if !status.Ready || !policy.AllowRoute {
		return fmt.Errorf("model residency is not ready: %s", status.Reason)
	}
	return nil
}

func conditionTrue(llmSvc *servingv1alpha2.LLMInferenceService, conditionType string) bool {
	condition := meta.FindStatusCondition(llmSvc.Status.Conditions, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue &&
		condition.ObservedGeneration == llmSvc.Generation
}
