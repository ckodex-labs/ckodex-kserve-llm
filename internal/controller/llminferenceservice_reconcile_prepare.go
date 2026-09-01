/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	kserveintegration "github.com/ckodex-labs/kserve-llm-operator/internal/kserve"
)

func (r *LLMInferenceServiceReconciler) fetchLLMInferenceService(ctx context.Context, req ctrl.Request, logger logr.Logger) (*servingv1alpha2.LLMInferenceService, bool, error) {
	var llmSvc servingv1alpha2.LLMInferenceService
	if err := r.Get(ctx, req.NamespacedName, &llmSvc); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("LLMInferenceService not found, likely deleted")
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("fetch LLMInferenceService: %w", err)
	}
	return &llmSvc, true, nil
}

func (r *LLMInferenceServiceReconciler) reconcileResourceSetup(ctx context.Context, state *llmInferenceReconcileState) (bool, error) {
	llmSvc := state.llmSvc
	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes); err == nil {
		if err := r.reconcileGPUCapacity(ctx, llmSvc, nodes.Items); err != nil {
			return false, fmt.Errorf("update GPU capacity condition for %s/%s: %w", llmSvc.Namespace, llmSvc.Name, err)
		}
	}
	cleanupFunc := func() error { return r.cleanupResources(ctx, llmSvc) }
	if deleted, err := r.CleanupReconciler.HandleFinalizer(ctx, llmSvc, api.FinalizerName, cleanupFunc); err != nil || deleted {
		return deleted, err
	}
	return false, nil
}

func (r *LLMInferenceServiceReconciler) reconcileGPUCapacity(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, nodes []corev1.Node) error {
	totalGpus := deployment.GetClusterGPUCapacity(nodes)
	ok, msg := deployment.CheckGPURequirements(llmSvc, totalGpus)
	if !ok {
		r.Recorder.Eventf(llmSvc, corev1.EventTypeWarning, "InsufficientGPUCapacity", "%s", msg)
		return r.StatusReconciler.SetCondition(ctx, llmSvc, "GPUCapacity", metav1.ConditionFalse, "InsufficientGPUs", msg)
	}
	return r.StatusReconciler.SetCondition(ctx, llmSvc, "GPUCapacity", metav1.ConditionTrue, "SufficientGPUs", "Cluster has enough GPU capacity")
}

func (r *LLMInferenceServiceReconciler) reconcileExternalInputs(ctx context.Context, state *llmInferenceReconcileState) error {
	llmSvc := state.llmSvc
	if r.ExternalSecret != nil {
		if err := r.ExternalSecret.ReconcileExternalSecret(ctx, llmSvc); err != nil {
			return fmt.Errorf("reconcile external secrets: %w", err)
		}
	}
	if r.HFCSI != nil {
		if err := r.HFCSI.Reconcile(ctx, llmSvc); err != nil {
			return fmt.Errorf("hf-csi provisioning: %w", err)
		}
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) prepareWorkloadInputs(ctx context.Context, state *llmInferenceReconcileState) error {
	llmSvc := state.llmSvc
	r.applyEarlyAIPackConfig(ctx, llmSvc)
	state.multiNode = kserveintegration.RequiresMultiNode(llmSvc)
	state.activeLoras = r.listActiveLoras(ctx, llmSvc)
	return nil
}

func (r *LLMInferenceServiceReconciler) listActiveLoras(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) []servingv1alpha2.LLMLoraAdapter {
	var list servingv1alpha2.LLMLoraAdapterList
	if err := r.List(ctx, &list, client.InNamespace(llmSvc.Namespace)); err != nil {
		return nil
	}
	active := make([]servingv1alpha2.LLMLoraAdapter, 0, len(list.Items))
	for _, lora := range list.Items {
		if lora.Spec.TargetService == llmSvc.Name {
			active = append(active, lora)
		}
	}
	return active
}
