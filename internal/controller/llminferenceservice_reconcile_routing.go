/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

func (r *LLMInferenceServiceReconciler) reconcileRouting(ctx context.Context, state *llmInferenceReconcileState) error {
	llmSvc := state.llmSvc
	if r.Gateway != nil {
		if err := r.Gateway.Reconcile(ctx, llmSvc); err != nil {
			return fmt.Errorf("reconcile gateway: %w", err)
		}
	}
	if err := observability.ReconcileVectorConfigMap(ctx, r.Client, r.Scheme, llmSvc, r.OTEL_Endpoint); err != nil {
		return fmt.Errorf("reconcile vector: %w", err)
	}
	if r.Autoscaler != nil && !state.multiNode {
		if err := r.Autoscaler.Reconcile(ctx, llmSvc); err != nil {
			return fmt.Errorf("reconcile autoscaler: %w", err)
		}
	}
	return nil
}
