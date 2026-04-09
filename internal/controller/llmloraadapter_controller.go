/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"bytes"
	"encoding/json"
	"net/http"

	"github.com/ckodex-labs/kserve-llm-operator/internal/governance"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/sony/gobreaker"
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VLLMLoadLoraRequest struct {
	LoraName string `json:"lora_name"`
	LoraPath string `json:"lora_path"`
}

type LLMLoraAdapterReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	HTTPClient     *http.Client
	Recorder       record.EventRecorder
	CircuitBreaker *gobreaker.CircuitBreaker
	Audit          *observability.AuditLogger
}

const (
// ModelMountPath is now imported from constants.go inside the same package.
)

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=localmodelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch

func (r *LLMLoraAdapterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the LLMLoraAdapter instance
	var lora servingv1alpha2.LLMLoraAdapter
	if err := r.Get(ctx, req.NamespacedName, &lora); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	originalLora := lora.DeepCopy()

	// 0. Governed Unit: Initialization of State Planes
	if lora.Status.StatePlanes.Lifecycle == "" {
		lora.Status.StatePlanes.Lifecycle = "proposed"
		lora.Status.StatePlanes.Trust = "unknown"
		lora.Status.StatePlanes.Risk = "normal"
		if err := r.Status().Patch(ctx, &lora, client.MergeFrom(originalLora)); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 0. Hard Invariant: Quarantine Enforcement
	if lora.Status.StatePlanes.Lifecycle == "quarantined" {
		logger.Info("Adapter is quarantined, blocking load", "Adapter", lora.Name)
		// Ensure it's detached
		_ = r.unloadFromTargetService(ctx, &lora)
		r.Recorder.Event(&lora, corev1.EventTypeWarning, "Quarantined", "Access to this composite model is forcibly blocked due to governance failure")
		
		observability.QuarantineIncidents.WithLabelValues(lora.Name, "manual_quarantine").Inc()
		observability.GovernanceState.WithLabelValues("quarantined", lora.Status.StatePlanes.Trust).Set(1)
		
		return ctrl.Result{}, nil
	}

	// 0. Handle Deletion
	const loraFinalizer = "serving.ckodex.com/lora-finalizer"
	if lora.DeletionTimestamp != nil {
		if containsString(lora.Finalizers, loraFinalizer) {
			logger.Info("Deleting LoRA adapter, triggering unload", "Adapter", lora.Name)
			if err := r.unloadFromTargetService(ctx, &lora); err != nil {
				logger.Error(err, "Failed to unload LoRA from target service")
			}
			lora.Finalizers = removeString(lora.Finalizers, loraFinalizer)
			if err := r.Patch(ctx, &lora, client.MergeFrom(originalLora)); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !containsString(lora.Finalizers, loraFinalizer) {
		lora.Finalizers = append(lora.Finalizers, loraFinalizer)
		if err := r.Update(ctx, &lora); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 1. Initial State Signaling - Ensure tests see "Progressing" immediately
	foundProgressing := false
	for _, c := range lora.Status.Conditions {
		if c.Type == "Progressing" || c.Type == servingv1alpha2.AdapterConditionReady {
			foundProgressing = true
			break
		}
	}
	if !foundProgressing {
		patch := client.MergeFrom(lora.DeepCopy())
		lora.Status.Conditions = append(lora.Status.Conditions, metav1.Condition{
			Type:               "Progressing",
			Status:             metav1.ConditionTrue,
			Reason:             "Reconciling",
			Message:            "LoRA hot-swap reconciliation started",
			LastTransitionTime: metav1.Now(),
		})
		if err := r.Status().Patch(ctx, &lora, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 1. Create or ensure LocalModelCache for the LoRA
	cacheName := fmt.Sprintf("lora-%s", lora.Name)
	expectedCache := &servingv1alpha2.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cacheName,
			Namespace: lora.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&lora, servingv1alpha2.SchemeGroupVersion.WithKind("LLMLoraAdapter")),
			},
		},
		Spec: servingv1alpha2.LocalModelCacheSpec{
			SourceModelURI: lora.Spec.Model.URI,
		},
	}

	var existingCache servingv1alpha2.LocalModelCache
	err := r.Get(ctx, client.ObjectKey{Name: cacheName, Namespace: lora.Namespace}, &existingCache)
	if err != nil && apierrors.IsNotFound(err) {
		logger.Info("Creating LocalModelCache for LoRA adapter", "Name", cacheName)
		if err := r.Create(ctx, expectedCache); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// 2. Check if Cache is downloaded
	isDownloaded := false
	for _, cond := range existingCache.Status.Conditions {
		if cond.Type == servingv1alpha2.ConditionReady && cond.Status == "True" {
			isDownloaded = true
			break
		}
	}

	if !isDownloaded {
		logger.Info("Waiting for LoRA LocalModelCache to finish downloading", "Name", cacheName)
		return ctrl.Result{RequeueAfter: 5000000000}, nil // 5 seconds
	}

	// 3. Evidence Plane: Canonical Governance Checks
	engine := governance.NewDefaultEngine()
	valid, reason := engine.Check(ctx, &lora)
	if !valid {
		logger.Error(nil, "Governance Check Failed", "Reason", reason)
		governance.TransitionStates(&lora, false, reason)
		r.Recorder.Eventf(&lora, corev1.EventTypeWarning, "GovernanceFail", "Adapter failed conformance vectors: %s", reason)
		
		observability.QuarantineIncidents.WithLabelValues(lora.Name, reason).Inc()
		observability.GovernanceState.WithLabelValues("quarantined", "denied").Set(1)
		
		if err := r.Status().Update(ctx, &lora); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil // Reconcile stops at Quarantine
	}

	// Transition to Active/Verified if not already
	if lora.Status.StatePlanes.Lifecycle != "active" {
		patch := client.MergeFrom(lora.DeepCopy())
		governance.TransitionStates(&lora, true, "")
		r.Recorder.Eventf(&lora, corev1.EventTypeNormal, "GovernancePass", "All conformance vectors passed. Transitioning to active.")
		observability.GovernanceState.WithLabelValues("active", "verified").Set(1)
		if err := r.Status().Patch(ctx, &lora, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 4. Register LoRA with Target Service (Hot Swap via vLLM admin API)
	// Fetch target service
	var targetSvc servingv1alpha2.LLMInferenceService
	if err := r.Get(ctx, client.ObjectKey{Name: lora.Spec.TargetService, Namespace: lora.Namespace}, &targetSvc); err != nil {
		logger.Error(err, "Target LLMInferenceService not found", "Target", lora.Spec.TargetService)
		return ctrl.Result{}, nil // Wait for target to appear
	}

	// If target isn't ready, we wait.
	if !targetSvc.Status.ModelReady {
		logger.Info("Target service is not ready yet. Waiting to inject LoRA.")
		return ctrl.Result{RequeueAfter: 5000000000}, nil
	}

	// Register LoRA with Target Service (Hot Swap via vLLM admin API)
	if err := r.registerWithTargetService(ctx, &lora, &targetSvc); err != nil {
		logger.Error(err, "Failed to register LoRA with target service pods")
		return ctrl.Result{RequeueAfter: 5000000000}, nil
	}

	// Final Status update
	if lora.Status.ActiveRevision == 0 {
		lora.Status.ActiveRevision = 1
		lora.Status.Conditions = append(lora.Status.Conditions, metav1.Condition{
			Type:               servingv1alpha2.AdapterConditionReady,
			Status:             metav1.ConditionTrue,
			Reason:             "AdapterLoaded",
			Message:            "Adapter successfully hot-swapped into vLLM runtime",
			LastTransitionTime: metav1.Now(),
		})
		if err := r.Status().Update(ctx, &lora); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("LoRA Adapter hot-swapped successfully!", "AdapterName", lora.Spec.AdapterName)
	}

	return ctrl.Result{}, nil
}

func (r *LLMLoraAdapterReconciler) registerWithTargetService(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter, svc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)

	if r.CircuitBreaker == nil {
		r.CircuitBreaker = NewDefaultCircuitBreaker(CircuitBreakerSettings{
			Name: "vllm-adapter-load",
		}, r.Recorder, lora)
	}

	// 1. Find all pods for the target service
	podList := &corev1.PodList{}
	labels := map[string]string{
		"app.kubernetes.io/instance": svc.Name,
	}
	if err := r.List(ctx, podList, client.InNamespace(svc.Namespace), client.MatchingLabels(labels)); err != nil {
		return err
	}

	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods found for target service %s", svc.Name)
	}

	// 2. Load adapter on each pod
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		podIP := pod.Status.PodIP
		if podIP == "" {
			continue
		}

		url := fmt.Sprintf("http://%s:8000/v1/load_lora_adapter", podIP)
		reqBody, _ := json.Marshal(VLLMLoadLoraRequest{
			LoraName: lora.Spec.AdapterName,
			LoraPath: fmt.Sprintf("%s/lora-%s", ModelMountPath, lora.Name),
		})

		_, err := r.CircuitBreaker.Execute(func() (interface{}, error) {
			httpClient := r.HTTPClient
			if httpClient == nil {
				httpClient = http.DefaultClient
			}
			resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(reqBody))
			if err != nil {
				return nil, err
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
				return nil, fmt.Errorf("vLLM returned non-OK status %d", resp.StatusCode)
			}
			return nil, nil
		})

		if err != nil {
			logger.Error(err, "Failed to load LoRA on pod", "pod", pod.Name)
			r.Recorder.Eventf(lora, corev1.EventTypeWarning, "RegistrationFailed",
				"Circuit breaker or vLLM error on pod %s: %v", pod.Name, err)
			return err
		}

		logger.Info("Successfully sent load_lora_adapter request", "pod", pod.Name)
		r.Recorder.Eventf(lora, corev1.EventTypeNormal, "Registered",
			"Successfully loaded LoRA adapter on pod %s", pod.Name)
	}

	return nil
}

func (r *LLMLoraAdapterReconciler) unloadFromTargetService(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter) error {
	if r.CircuitBreaker == nil {
		r.CircuitBreaker = NewDefaultCircuitBreaker(CircuitBreakerSettings{
			Name: "vllm-adapter-unload",
		}, r.Recorder, lora)
	}

	// Find the target service to get its pods
	var svc servingv1alpha2.LLMInferenceService
	if err := r.Get(ctx, client.ObjectKey{Name: lora.Spec.TargetService, Namespace: lora.Namespace}, &svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Service gone, nothing to unload
		}
		return err
	}

	podList := &corev1.PodList{}
	labels := map[string]string{
		"app.kubernetes.io/instance": svc.Name,
	}
	if err := r.List(ctx, podList, client.InNamespace(svc.Namespace), client.MatchingLabels(labels)); err != nil {
		return err
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Status.PodIP == "" {
			continue
		}

		url := fmt.Sprintf("http://%s:8000/v1/unload_lora_adapter", pod.Status.PodIP)
		reqBody, _ := json.Marshal(map[string]string{"lora_name": lora.Spec.AdapterName})

		_, _ = r.CircuitBreaker.Execute(func() (interface{}, error) {
			httpClient := r.HTTPClient
			if httpClient == nil {
				httpClient = http.DefaultClient
			}
			resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(reqBody))
			if err != nil {
				return nil, err
			}
			defer func() { _ = resp.Body.Close() }()
			return nil, nil
		})
	}

	return nil
}

// Utility functions for string slice manipulation
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMLoraAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.LLMLoraAdapter{}).
		Owns(&servingv1alpha2.LocalModelCache{}).
		Complete(r)
}
