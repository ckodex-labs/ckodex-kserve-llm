/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
)

func (r *LLMInferenceServiceReconciler) reconcileIdentityAndPolicy(ctx context.Context, state *llmInferenceReconcileState) error {
	if err := r.reconcileVault(ctx, state); err != nil {
		return err
	}
	if err := r.reconcileSPIRE(ctx, state); err != nil {
		return err
	}
	if err := r.reconcileEbpf(ctx, state); err != nil {
		return err
	}
	return r.reconcileOPA(ctx, state)
}

func (r *LLMInferenceServiceReconciler) reconcileVault(ctx context.Context, state *llmInferenceReconcileState) error {
	if r.Vault == nil {
		return nil
	}
	if err := r.Vault.ReconcileVault(ctx, state.llmSvc); err != nil {
		return fmt.Errorf("reconcile vault: %w", err)
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) reconcileSPIRE(ctx context.Context, state *llmInferenceReconcileState) error {
	llmSvc := state.llmSvc
	if r.SPIRE != nil {
		if err := r.SPIRE.ReconcileSPIRE(ctx, llmSvc.Namespace); err != nil {
			return fmt.Errorf("reconcile spire: %w", err)
		}
	}
	if r.SPIRERegistration != nil {
		if err := r.SPIRERegistration.ReconcileRegistrationEntry(ctx, llmSvc); err != nil {
			return fmt.Errorf("reconcile spire registration entry: %w", err)
		}
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) reconcileEbpf(ctx context.Context, state *llmInferenceReconcileState) error {
	if r.Ebpf == nil {
		return nil
	}
	if err := r.Ebpf.ReconcileEbpfPolicy(ctx, state.llmSvc); err != nil {
		return fmt.Errorf("reconcile ebpf: %w", err)
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) reconcileOPA(ctx context.Context, state *llmInferenceReconcileState) error {
	if r.OPA == nil {
		return nil
	}
	if err := r.OPA.ReconcileOPA(ctx, state.llmSvc.Namespace, r.OPAConfig); err != nil {
		return fmt.Errorf("reconcile opa: %w", err)
	}
	return nil
}
