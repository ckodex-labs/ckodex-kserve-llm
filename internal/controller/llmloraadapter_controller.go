/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/governance"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
	"github.com/ckodex-labs/kserve-llm-operator/internal/storage"
	"github.com/sony/gobreaker"
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

	// Warmup tracking to prevent duplicate warmup requests
	warmupMu   sync.Mutex
	warmupDone map[string]bool
}

const (
	loraFinalizer             = "serving.ckodex.com/lora-finalizer"
	loraCacheManagedByLabel   = "serving.ckodex.com/managed-by"
	loraCacheOwnerNamespace   = "serving.ckodex.com/owner-namespace"
	loraCacheOwnerName        = "serving.ckodex.com/owner-name"
	loraCacheOwnerUID         = "serving.ckodex.com/owner-uid"
	loraCacheManagedByAdapter = "llmloraadapter"
)

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=localmodelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

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

	if lora.DeletionTimestamp != nil {
		return r.finalizeLora(ctx, &lora, originalLora)
	}

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
	expectedCache := newLoraCache(&lora)
	cacheName := expectedCache.Name

	var existingCache servingv1alpha2.LocalModelCache
	err := r.Get(ctx, client.ObjectKey{Name: cacheName}, &existingCache)
	if err != nil && apierrors.IsNotFound(err) {
		logger.Info("Creating LocalModelCache for LoRA adapter", "Name", cacheName)
		if err := r.Create(ctx, expectedCache); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}
	if err := validateLoraCacheOwner(&existingCache, &lora); err != nil {
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

	if updated, err := r.hydrateVerificationEvidence(ctx, &lora, &existingCache); err != nil {
		logger.Error(err, "Failed to read LoRA runtime verification evidence", "adapter", lora.Name)
	} else if updated {
		if err := r.Status().Update(ctx, &lora); err != nil {
			return ctrl.Result{}, err
		}
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
		r.Recorder.Eventf(&lora, corev1.EventTypeNormal, "GovernancePass", "Conformance vectors passed. Adapter remains %s while stronger verification is pending.", lora.Status.StatePlanes.Trust)
		observability.GovernanceState.WithLabelValues(lora.Status.StatePlanes.Lifecycle, lora.Status.StatePlanes.Trust).Set(1)
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

func (r *LLMLoraAdapterReconciler) finalizeLora(
	ctx context.Context,
	lora *servingv1alpha2.LLMLoraAdapter,
	original *servingv1alpha2.LLMLoraAdapter,
) (ctrl.Result, error) {
	if !containsString(lora.Finalizers, loraFinalizer) {
		return ctrl.Result{}, nil
	}
	if err := r.unloadFromTargetService(ctx, lora); err != nil {
		return ctrl.Result{}, fmt.Errorf("unload LoRA adapter: %w", err)
	}
	if err := r.deleteLoraCache(ctx, lora); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete LoRA cache: %w", err)
	}
	lora.Finalizers = removeString(lora.Finalizers, loraFinalizer)
	if err := r.Patch(ctx, lora, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func newLoraCache(lora *servingv1alpha2.LLMLoraAdapter) *servingv1alpha2.LocalModelCache {
	return &servingv1alpha2.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: loraCacheName(lora.Namespace, lora.Name),
			Labels: map[string]string{
				loraCacheManagedByLabel: loraCacheManagedByAdapter,
			},
			Annotations: map[string]string{
				loraCacheOwnerNamespace:          lora.Namespace,
				loraCacheOwnerName:               lora.Name,
				loraCacheOwnerUID:                string(lora.UID),
				cacheWorkloadNamespaceAnnotation: lora.Namespace,
			},
		},
		Spec: servingv1alpha2.LocalModelCacheSpec{SourceModelURI: lora.Spec.Model.URI},
	}
}

func loraCacheName(namespace, name string) string {
	sum := sha256.Sum256([]byte(namespace + "/" + name))
	return fmt.Sprintf("lora-%x", sum[:10])
}

func validateLoraCacheOwner(cache *servingv1alpha2.LocalModelCache, lora *servingv1alpha2.LLMLoraAdapter) error {
	if cache.Labels[loraCacheManagedByLabel] != loraCacheManagedByAdapter ||
		cache.Annotations[loraCacheOwnerNamespace] != lora.Namespace ||
		cache.Annotations[loraCacheOwnerName] != lora.Name ||
		cache.Annotations[loraCacheOwnerUID] != string(lora.UID) {
		return fmt.Errorf("LocalModelCache %s is not owned by LLMLoraAdapter %s/%s", cache.Name, lora.Namespace, lora.Name)
	}
	if cache.Spec.SourceModelURI != lora.Spec.Model.URI {
		return fmt.Errorf("LocalModelCache %s source URI does not match LLMLoraAdapter", cache.Name)
	}
	return nil
}

func (r *LLMLoraAdapterReconciler) deleteLoraCache(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter) error {
	var cache servingv1alpha2.LocalModelCache
	if err := r.Get(ctx, client.ObjectKey{Name: loraCacheName(lora.Namespace, lora.Name)}, &cache); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := validateLoraCacheOwner(&cache, lora); err != nil {
		return err
	}
	return client.IgnoreNotFound(r.Delete(ctx, &cache))
}

func (r *LLMLoraAdapterReconciler) hydrateVerificationEvidence(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter, cache *servingv1alpha2.LocalModelCache) (bool, error) {
	if !storage.HasOCIScheme(lora.Spec.Model.URI) {
		return false, nil
	}
	if len(cache.Status.NodeStatuses) == 0 {
		return false, nil
	}

	verifiedNodes := 0
	var latestRecord *provenance.RuntimeVerificationRecord
	for _, nodeStatus := range cache.Status.NodeStatuses {
		if nodeStatus.Phase != "Ready" {
			return false, nil
		}
		jobName := fmt.Sprintf("%s-%s-%s", warmupJobPrefix, nodeStatus.ModelURIHash,
			fmt.Sprintf("%x", sha256.Sum256([]byte(nodeStatus.NodeName)))[:8])
		record, err := readJobVerificationRecord(ctx, r.Client, cacheWorkloadNamespace(cache), jobName)
		if err != nil {
			return false, err
		}
		if record == nil || !record.Verified() {
			return false, nil
		}
		verifiedNodes++
		latestRecord = record
	}

	if latestRecord == nil || verifiedNodes == 0 {
		return false, nil
	}

	verifiedAt, err := time.Parse(time.RFC3339, latestRecord.VerifiedAt)
	if err != nil {
		verifiedAt = time.Now().UTC()
	}
	now := metav1.NewTime(verifiedAt)

	lora.Status.EvidenceBundle.SignatureDigest = latestRecord.SignatureDigest
	lora.Status.EvidenceBundle.AttestationURI = latestRecord.AttestationURI
	lora.Status.EvidenceBundle.SBOMDigest = latestRecord.SBOMDigest
	lora.Status.EvidenceBundle.LastVerifiedAt = &now
	lora.Status.StatePlanes.Trust = "verified"
	return true, nil
}

func readJobVerificationRecord(ctx context.Context, c client.Client, namespace, jobName string) (*provenance.RuntimeVerificationRecord, error) {
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{"job-name": jobName}); err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name != "warmup" || status.State.Terminated == nil {
				continue
			}
			message := status.State.Terminated.Message
			if strings.TrimSpace(message) == "" {
				continue
			}
			record, err := provenance.ParseRuntimeVerificationRecord(message)
			if err != nil {
				return nil, fmt.Errorf("parse warmup verification record from pod %s: %w", pod.Name, err)
			}
			return record, nil
		}
	}

	return nil, nil
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
			// 3-attempt retry with 500 ms backoff for transient vLLM startup races.
			var lastErr error
			for attempt := 0; attempt < 3; attempt++ {
				resp, postErr := postJSON(ctx, httpClient, url, reqBody)
				if postErr == nil {
					ok := resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted
					_ = resp.Body.Close()
					if ok {
						return nil, nil
					}
					lastErr = fmt.Errorf("vLLM returned non-OK status %d", resp.StatusCode)
				} else {
					lastErr = postErr
				}
				if attempt < 2 {
					select {
					case <-time.After(500 * time.Millisecond):
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
			}
			return nil, lastErr
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

		// 3. Proactive Warmup (M3 Vision)
		r.warmupMu.Lock()
		warmupKey := fmt.Sprintf("%s/%s/%s", pod.Name, lora.Name, lora.Spec.AdapterName)
		if !r.warmupDone[warmupKey] {
			r.warmupMu.Unlock()
			go r.performWarmup(ctx, podIP, lora.Spec.AdapterName)
			r.warmupMu.Lock()
			if r.warmupDone == nil {
				r.warmupDone = make(map[string]bool)
			}
			r.warmupDone[warmupKey] = true
		}
		r.warmupMu.Unlock()
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
			resp, err := postJSON(ctx, httpClient, url, reqBody)
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

func (r *LLMLoraAdapterReconciler) performWarmup(ctx context.Context, podIP, adapterName string) {
	logger := log.FromContext(ctx)
	url := fmt.Sprintf("http://%s:8000/v1/completions", podIP)

	// Minimal warmup request to trigger weight allocation in VRAM
	warmupReq := map[string]interface{}{
		"model":      adapterName,
		"prompt":     " ",
		"max_tokens": 1,
		"echo":       false,
	}
	body, _ := json.Marshal(warmupReq)

	httpClient := r.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := postJSON(ctx, httpClient, url, body)
	if err != nil {
		logger.Error(err, "Warmup request failed", "pod", podIP)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	logger.Info("Proactive warmup complete", "pod", podIP, "adapter", adapterName)
}

func postJSON(ctx context.Context, httpClient *http.Client, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return httpClient.Do(req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMLoraAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("llmloraadapter").
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.LLMLoraAdapter{}).
		Watches(
			&servingv1alpha2.LocalModelCache{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
				annotations := obj.GetAnnotations()
				if obj.GetLabels()[loraCacheManagedByLabel] != loraCacheManagedByAdapter {
					return nil
				}
				namespace := annotations[loraCacheOwnerNamespace]
				name := annotations[loraCacheOwnerName]
				if namespace == "" || name == "" {
					return nil
				}
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{Namespace: namespace, Name: name},
				}}
			}),
		).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				pod, ok := obj.(*corev1.Pod)
				if !ok {
					return nil
				}

				// Find service name from labels
				svcName, ok := pod.Labels["app.kubernetes.io/instance"]
				if !ok {
					return nil
				}

				// List all adapters in the same namespace
				var adapters servingv1alpha2.LLMLoraAdapterList
				if err := mgr.GetClient().List(ctx, &adapters, client.InNamespace(pod.Namespace)); err != nil {
					return nil
				}

				var requests []reconcile.Request
				for _, adapter := range adapters.Items {
					if adapter.Spec.TargetService == svcName {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      adapter.Name,
								Namespace: adapter.Namespace,
							},
						})
					}
				}
				return requests
			}),
		).
		Complete(r)
}
