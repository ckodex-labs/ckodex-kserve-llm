/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (r *LLMInferenceServiceReconciler) updateLLMInferenceStatus(ctx context.Context, state *llmInferenceReconcileState) error {
	llmSvc := state.llmSvc
	isOptimized := GetWellKnownConfig(llmSvc.Spec.Model.URI) != nil
	hwType := r.HardwareCache.Get(ctx, r.Client, r.APIReader)
	llmSvc.Status.DetectedHardware = string(hwType)
	var metrics *servingv1alpha2.AdaptiveMetrics
	if r.Metrics != nil {
		metrics = r.Metrics.GetAdaptiveMetrics(ctx, llmSvc.Namespace, llmSvc.Name)
	}
	var err error
	if state.multiNode {
		err = r.StatusReconciler.UpdateFromKServe(ctx, llmSvc, state.beforePatch, isOptimized, metrics)
	} else {
		err = r.StatusReconciler.Update(ctx, llmSvc, state.beforePatch, isOptimized, metrics)
	}
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}
