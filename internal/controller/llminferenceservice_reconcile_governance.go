/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/governance"
)

func (r *LLMInferenceServiceReconciler) reconcileGovernance(ctx context.Context, state *llmInferenceReconcileState, logger logr.Logger) error {
	state.activePacks = r.listActiveAIPacks(ctx, state.llmSvc, logger)
	state.llmSvc.Status.StatePlanes = governance.AggregateStatePlanes(state.llmSvc, state.activeLoras)
	if r.GovernanceReconciler == nil {
		return nil
	}
	if err := r.GovernanceReconciler.Reconcile(ctx, state.llmSvc, state.activeLoras); err != nil {
		return fmt.Errorf("reconcile governance evidence: %w", err)
	}
	if err := r.GovernanceReconciler.ReconcileAIPacks(ctx, state.llmSvc, state.activePacks); err != nil {
		logger.Error(err, "aipack governance reconcile error (non-blocking)")
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) listActiveAIPacks(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, logger logr.Logger) []servingv1alpha2.AIPack {
	var list servingv1alpha2.AIPackList
	if err := r.List(ctx, &list, client.InNamespace(llmSvc.Namespace)); err != nil {
		logger.Error(err, "failed to list AIPacks; governance reconcile will run with empty pack list")
		return nil
	}
	active := make([]servingv1alpha2.AIPack, 0, len(list.Items))
	for _, pack := range list.Items {
		if pack.Labels["serving.ckodex.com/workload"] == llmSvc.Name {
			active = append(active, pack)
		}
	}
	return active
}
