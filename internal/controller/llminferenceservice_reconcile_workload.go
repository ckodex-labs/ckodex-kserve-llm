/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
)

func (r *LLMInferenceServiceReconciler) reconcileWorkload(ctx context.Context, state *llmInferenceReconcileState) error {
	if state.multiNode {
		return r.reconcileMultiNodeWorkload(ctx, state)
	}
	return r.reconcileSingleNodeWorkload(ctx, state)
}

func (r *LLMInferenceServiceReconciler) reconcileMultiNodeWorkload(ctx context.Context, state *llmInferenceReconcileState) error {
	if r.KServeMultiNode == nil {
		return fmt.Errorf("KServe multi-node reconciler is not configured")
	}
	if len(state.activeLoras) > 0 {
		return fmt.Errorf("KServe v0.19 multi-node does not support CKodex LoRA adapters")
	}
	if err := r.KServeMultiNode.Reconcile(ctx, state.llmSvc); err != nil {
		return fmt.Errorf("reconcile KServe multi-node InferenceService: %w", err)
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) reconcileSingleNodeWorkload(ctx context.Context, state *llmInferenceReconcileState) error {
	llmSvc := state.llmSvc
	if err := r.reconcileDeployment(ctx, llmSvc, state.activeLoras); err != nil {
		return fmt.Errorf("reconcile deployment: %w", err)
	}
	if err := r.PDBReconciler.Reconcile(ctx, llmSvc); err != nil {
		return fmt.Errorf("reconcile pdb: %w", err)
	}
	if err := r.ServiceReconciler.Reconcile(ctx, llmSvc); err != nil {
		return fmt.Errorf("reconcile service: %w", err)
	}
	if err := r.reconcilePrefillDeployment(ctx, llmSvc); err != nil {
		return fmt.Errorf("reconcile prefill deployment: %w", err)
	}
	return nil
}
