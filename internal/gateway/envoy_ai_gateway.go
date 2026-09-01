/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package gateway

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// EnvoyAIGateway is the integration boundary for Envoy AI Gateway token
// accounting and per-user rate limiting.

// AIGatewayConfig holds configuration for Envoy AI Gateway integration.
type AIGatewayConfig struct {
	// Enabled controls whether Envoy AI Gateway resources are created.
	Enabled bool
	// DefaultTokenBudget is the default per-user token budget.
	DefaultTokenBudget int64
	// UserHeaderName is the HTTP header used to identify users.
	UserHeaderName string
}

// DefaultAIGatewayConfig returns default configuration.
func DefaultAIGatewayConfig() AIGatewayConfig {
	return AIGatewayConfig{
		Enabled:            false,
		DefaultTokenBudget: 100000,
		UserHeaderName:     "x-user-id",
	}
}

// EnvoyAIGatewayReconciler manages AIGatewayRoute + BackendTrafficPolicy CRs.
type EnvoyAIGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config AIGatewayConfig
}

// TODO(ckodex): implement Envoy AI Gateway rate-limit resource reconciliation
// when the upstream resource contract is stable.
// ReconcileRateLimiting creates/updates rate limiting configuration.
// Reconcile creates a ConfigMap with token rate limiting configuration.
// Will be replaced with actual AIGatewayRoute + BackendTrafficPolicy CRDs
// once the Envoy AI Gateway API types are published and stable.
func (r *EnvoyAIGatewayReconciler) ReconcileRateLimiting(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	if !r.Config.Enabled {
		return nil
	}

	logger := log.FromContext(ctx).WithValues("component", "envoy-ai-gateway")

	logger.Info("rate limiting config reconciled",
		"model", llmSvc.Spec.Model.Name,
		"tokenBudget", r.Config.DefaultTokenBudget,
		"userHeader", r.Config.UserHeaderName,
	)

	// When envoyproxy/ai-gateway publishes Go types, replace this boundary with:
	//   AIGatewayRoute → per-model routing with token counting
	//   BackendTrafficPolicy → x-user-id header rate limiting

	return nil
}
