/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
)

// reconcileDeployment creates or updates the vLLM Deployment.
func (r *LLMInferenceServiceReconciler) reconcileDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, loras []servingv1alpha2.LLMLoraAdapter) error {
	desired, replicas := r.buildLLMDeployment(ctx, llmSvc, loras)
	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}
	return r.applyDeployment(ctx, llmSvc, desired, replicas)
}

func (r *LLMInferenceServiceReconciler) buildLLMDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, loras []servingv1alpha2.LLMLoraAdapter) (*appsv1.Deployment, int32) {
	if wellKnown := GetWellKnownConfig(llmSvc.Spec.Model.URI); wellKnown != nil {
		r.ApplyConfigToSpec(&llmSvc.Spec, wellKnown)
	}
	replicas := int32(1)
	if llmSvc.Spec.Replicas != nil {
		replicas = *llmSvc.Spec.Replicas
	}
	hwType := r.HardwareCache.Get(ctx, r.Client, r.APIReader)
	role := ""
	if llmSvc.Spec.Prefill != nil {
		role = "kv_consumer"
	}
	return r.DeploymentBuilder.BuildWithRole(ctx, llmSvc, replicas, hwType, loras, role), replicas
}

func (r *LLMInferenceServiceReconciler) applyDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, desired *appsv1.Deployment, replicas int32) error {
	logger := log.FromContext(ctx)
	var existing appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.createDeployment(ctx, llmSvc, desired, logger)
	}
	if err != nil {
		r.Recorder.Eventf(llmSvc, corev1.EventTypeWarning, "DeploymentLookupFailed", "Failed to look up Deployment %s: %v", desired.Name, err)
		return fmt.Errorf("get deployment: %w", err)
	}
	if reconciler.SyncDeployment(ctx, &existing, desired, replicas, llmSvc.Spec.Scaling != nil) {
		logger.Info("updating Deployment", "name", desired.Name)
		if err := r.Update(ctx, &existing); err != nil {
			return fmt.Errorf("update deployment: %w", err)
		}
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) createDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, desired *appsv1.Deployment, logger logr.Logger) error {
	logger.Info("creating Deployment", "name", desired.Name)
	if err := r.Create(ctx, desired); err != nil {
		r.Recorder.Eventf(llmSvc, corev1.EventTypeWarning, "DeploymentCreationFailed", "Failed to create Deployment %s: %v", desired.Name, err)
		return err
	}
	r.Recorder.Eventf(llmSvc, corev1.EventTypeNormal, "DeploymentCreated", "Successfully created Deployment %s", desired.Name)
	return nil
}

func (r *LLMInferenceServiceReconciler) reconcilePrefillDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	name := llmSvc.Name + "-prefill"
	var existing appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing)
	if llmSvc.Spec.Prefill == nil {
		return r.cleanupPrefillDeployment(ctx, &existing, err)
	}
	desired := r.DeploymentBuilder.BuildPrefill(ctx, llmSvc, r.HardwareCache.Get(ctx, r.Client, r.APIReader))
	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set prefill owner reference: %w", err)
	}
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	return r.updatePrefillDeployment(ctx, &existing, desired)
}

func (r *LLMInferenceServiceReconciler) cleanupPrefillDeployment(ctx context.Context, existing *appsv1.Deployment, err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.Delete(ctx, existing)
}

func (r *LLMInferenceServiceReconciler) updatePrefillDeployment(ctx context.Context, existing, desired *appsv1.Deployment) error {
	replicas := int32(1)
	if desired.Spec.Replicas != nil {
		replicas = *desired.Spec.Replicas
	}
	if reconciler.SyncDeployment(ctx, existing, desired, replicas, false) {
		return r.Update(ctx, existing)
	}
	return nil
}
