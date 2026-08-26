/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// mapLoraAdapterToInferenceService maps an LLMLoraAdapter to its target LLMInferenceService.
func (r *LLMInferenceServiceReconciler) mapLoraAdapterToInferenceService(_ context.Context, obj client.Object) []reconcile.Request {
	lora, ok := obj.(*servingv1alpha2.LLMLoraAdapter)
	if !ok {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: lora.Spec.TargetService, Namespace: lora.Namespace}}}
}

// mapLocalModelCacheToInferenceServices maps a LocalModelCache to all LLMInferenceServices using that model.
func (r *LLMInferenceServiceReconciler) mapLocalModelCacheToInferenceServices(ctx context.Context, obj client.Object) []reconcile.Request {
	lmc, ok := obj.(*servingv1alpha2.LocalModelCache)
	if !ok {
		return nil
	}
	var list servingv1alpha2.LLMInferenceServiceList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	return localModelCacheRequests(lmc, list.Items)
}

func localModelCacheRequests(lmc *servingv1alpha2.LocalModelCache, services []servingv1alpha2.LLMInferenceService) []reconcile.Request {
	results := make([]reconcile.Request, 0)
	for _, llm := range services {
		if llm.Spec.Model.URI == lmc.Spec.SourceModelURI {
			results = append(results, reconcile.Request{NamespacedName: types.NamespacedName{Name: llm.Name, Namespace: llm.Namespace}})
		}
	}
	return results
}
