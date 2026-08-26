/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (r *LLMInferenceServiceReconciler) recordLLMInferenceAudit(ctx context.Context, state *llmInferenceReconcileState, logger logr.Logger) {
	llmSvc := state.llmSvc
	if r.Audit != nil {
		mode := "observe"
		if r.OPA != nil {
			mode = "enforced"
		}
		details := llmInferenceAuditDetails(llmSvc, mode)
		resourceRef := fmt.Sprintf("LLMInferenceService/%s/%s", llmSvc.Namespace, llmSvc.Name)
		r.Audit.LogUpdate(ctx, resourceRef, "controller", details)
		if llmSvc.Status.ModelReady {
			r.Audit.LogReceipt(ctx, "", "ok", "Model server materialized and ready", details)
		}
	}
	logger.Info("reconciliation complete", "name", llmSvc.Name, "replicas", llmSvc.Status.Replicas, "ready", llmSvc.Status.ModelReady)
}

func llmInferenceAuditDetails(llmSvc *servingv1alpha2.LLMInferenceService, mode string) map[string]string {
	modelRef, modelScheme := minimizedModelReference(llmSvc.Spec.Model.URI)
	return map[string]string{
		"replicas":     fmt.Sprintf("%d", llmSvc.Status.Replicas),
		"ready":        fmt.Sprintf("%t", llmSvc.Status.ModelReady),
		"model_ref":    modelRef,
		"model_scheme": modelScheme,
		"engine":       llmSvc.Spec.Engine,
		"exec.mode":    mode,
		"detected_hw":  llmSvc.Status.DetectedHardware,
	}
}

func minimizedModelReference(modelURI string) (string, string) {
	digest := sha256.Sum256([]byte(modelURI))
	modelScheme := ""
	if parsed, err := url.Parse(modelURI); err == nil {
		modelScheme = strings.ToLower(parsed.Scheme)
	}
	return "sha256:" + hex.EncodeToString(digest[:]), modelScheme
}

func (r *LLMInferenceServiceReconciler) finishLLMInferenceReconcile(state *llmInferenceReconcileState) (ctrl.Result, error) {
	if state.multiNode && !state.llmSvc.Status.ModelReady {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}
