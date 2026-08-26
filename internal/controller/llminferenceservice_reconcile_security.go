/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
)

func (r *LLMInferenceServiceReconciler) reconcileSecurityIsolation(ctx context.Context, state *llmInferenceReconcileState) error {
	llmSvc := state.llmSvc
	if r.NetworkPolicy != nil {
		if err := r.NetworkPolicy.ReconcileNetworkPolicy(ctx, llmSvc); err != nil {
			return fmt.Errorf("reconcile network policy: %w", err)
		}
	}
	if r.ToolSurface != nil {
		if err := r.ToolSurface.ReconcileToolSurface(ctx, llmSvc, state.activeLoras); err != nil {
			return fmt.Errorf("reconcile tool surface: %w", err)
		}
	}
	return nil
}
