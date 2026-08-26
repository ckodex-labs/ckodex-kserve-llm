/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package evidence reconciles NIST 800-53 compliance conditions for LLMInferenceService
// objects by aggregating supply-chain verification state from the active LoRA adapter set.
package evidence

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
)

// GovernanceReconciler updates OSCAL/Lula compliance conditions on an LLMInferenceService.
// It reads pod state (for base-model verification) and aggregates LoRA adapter evidence.
type GovernanceReconciler struct {
	Client             client.Client
	Scheme             *runtime.Scheme
	AirGappedMode      bool
	LocalCosignKeyPath string
}

// Reconcile updates the Compliance-AC-4, AU-2, SI-7, SI-4, and SR-2 status conditions
// based on the current runtime evidence state. It does not patch the CR; callers must
// patch after calling this function.
func (g *GovernanceReconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, activeLoras []servingv1alpha2.LLMLoraAdapter) error {
	conditions, err := g.buildComplianceConditions(ctx, llmSvc, activeLoras)
	if err != nil {
		return err
	}
	for _, condition := range conditions {
		meta.SetStatusCondition(&llmSvc.Status.Conditions, condition)
	}
	log.FromContext(ctx).Info("Updated governance evidence for Lula validation", "controls", "AC-4, AU-2, SI-7, SI-4, SR-2")
	return nil
}

// runtimeVerifiableModelURI reports whether the model URI points to an OCI artifact
// that the storage-initializer can verify at pull time.
func runtimeVerifiableModelURI(uri string) bool {
	return strings.HasPrefix(uri, "oci://") || strings.HasPrefix(uri, "ocis://")
}

func (g *GovernanceReconciler) baseModelVerificationRecorded(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) (bool, error) {
	var pods corev1.PodList
	if err := g.Client.List(ctx, &pods,
		client.InNamespace(llmSvc.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "llminferenceservice",
			"app.kubernetes.io/instance": llmSvc.Name,
		},
	); err != nil {
		return false, err
	}

	readyPods := 0
	for _, pod := range pods.Items {
		if !isReadyPod(&pod) {
			continue
		}
		readyPods++
		record, err := readInitContainerVerificationRecord(&pod, "storage-initializer")
		if err != nil {
			return false, err
		}
		if record == nil || !record.Verified() {
			return false, nil
		}
	}

	return readyPods > 0, nil
}

func readInitContainerVerificationRecord(pod *corev1.Pod, containerName string) (*provenance.RuntimeVerificationRecord, error) {
	for _, status := range pod.Status.InitContainerStatuses {
		if status.Name != containerName || status.State.Terminated == nil {
			continue
		}
		message := strings.TrimSpace(status.State.Terminated.Message)
		if message == "" {
			return nil, nil
		}
		record, err := provenance.ParseRuntimeVerificationRecord(message)
		if err != nil {
			return nil, fmt.Errorf("parse init-container verification record from pod %s: %w", pod.Name, err)
		}
		return record, nil
	}
	return nil, nil
}

func isReadyPod(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
