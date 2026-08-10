/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"

	corev1 "k8s.io/api/core/v1"

	policyv1 "k8s.io/api/policy/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/auth"
	"github.com/ckodex-labs/kserve-llm-operator/internal/autoscaler"
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/cleanup"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/evidence"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/status"
	"github.com/ckodex-labs/kserve-llm-operator/internal/gateway"
	"github.com/ckodex-labs/kserve-llm-operator/internal/governance"
	kserveintegration "github.com/ckodex-labs/kserve-llm-operator/internal/kserve"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/ckodex-labs/kserve-llm-operator/internal/scheduler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/security"
)

// LLMInferenceServiceReconciler reconciles LLMInferenceService objects.
// Follows KServe control plane pattern: watch-reconcile loop with
// clean control/data plane separation.
type LLMInferenceServiceReconciler struct {
	client.Client
	Scheme                            *runtime.Scheme
	Gateway                           *gateway.Reconciler
	Scheduler                         *scheduler.Reconciler
	Autoscaler                        *autoscaler.Reconciler
	OPA                               *security.OPAReconciler // nil when EnableSecurity=false
	OPAConfig                         security.OPAConfig      // populated from OperatorConfig.Security when OPA != nil
	NetworkPolicy                     *security.NetworkPolicyReconciler
	ExternalSecret                    *security.ExternalSecretReconciler
	Vault                             *security.VaultReconciler
	SPIRE                             *security.SPIREReconciler
	SPIRERegistration                 *security.SPIRERegistrationReconciler // nil when EnableSecurity=false
	Ebpf                              *security.EbpfReconciler
	ToolSurface                       *security.ToolSurfaceReconciler
	Audit                             *observability.AuditLogger
	Inst                              *observability.Instrumentation // nil → no forbidden-tuple metrics emitted
	AuthMiddleware                    *auth.Middleware               // nil when EnableAuth=false
	BudgetEnforcer                    *auth.TokenBudgetEnforcer      // nil when EnableAuth=false
	Recorder                          record.EventRecorder
	APIReader                         client.Reader
	EnableGRPC                        bool
	EnableHardwareSelection           bool
	EnableExperimentalStatusHardening bool
	OTEL_Endpoint                     string // Contract: OTEL_EXPORTER_OTLP_ENDPOINT

	// AirGap configuration
	AirGappedMode          bool
	LocalRegistry          string
	LocalCosignKeyPath     string
	RuntimeImage           string
	HFInitializerImage     string
	HFMirrorURL            string
	KServeMultiNodeRuntime string
	Defaults               operatorconfig.DefaultsConfig

	// Modular sub-reconcilers
	DeploymentBuilder    *deployment.Builder
	StatusReconciler     *status.Reconciler
	CleanupReconciler    *cleanup.Reconciler
	ServiceReconciler    *reconciler.ServiceReconciler
	PDBReconciler        *reconciler.PDBReconciler
	GovernanceReconciler *evidence.GovernanceReconciler
	HardwareCache        deployment.HardwareCache
	// HFCSIReconciler provisions PV+PVC for hf-mount:// URIs using the official hf-csi-driver.
	// Must run before reconcileDeployment so the PVC exists when the pod is scheduled.
	HFCSI *HFCSIReconciler
	// KServeMultiNode delegates explicit multi-node workloads to KServe v0.19 workerSpec.
	KServeMultiNode *kserveintegration.Reconciler

	// M3 Vision: Real-time Metrics Query
	Metrics observability.MetricsQuerier
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways;httproutes;grpcroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=inference.networking.k8s.io,resources=inferencepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the main reconcile loop.
func (r *LLMInferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	logger := log.FromContext(ctx)
	reconcileStart := time.Now()
	modelName := "" // populated after CR fetch

	// Record reconcile metrics at the end of every reconcile call.
	defer func() {
		if r.Inst != nil && modelName != "" {
			r.Inst.RecordReconcile(ctx, modelName, time.Since(reconcileStart).Seconds(), retErr == nil)
		}
	}()

	// 1. Fetch the LLMInferenceService CR
	var llmSvc servingv1alpha2.LLMInferenceService
	if err := r.Get(ctx, req.NamespacedName, &llmSvc); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("LLMInferenceService not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetch LLMInferenceService: %w", err)
	}
	modelName = llmSvc.Spec.Model.Name

	// Capture original object for diffing and patching at the end
	llmSvcBeforePatch := llmSvc.DeepCopy()

	// 2. Resource Management & Finalizers
	// Informational GPU Capacity Check
	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes); err == nil {
		totalGpus := deployment.GetClusterGPUCapacity(nodes.Items)
		ok, msg := deployment.CheckGPURequirements(&llmSvc, totalGpus)
		if !ok {
			r.Recorder.Eventf(&llmSvc, corev1.EventTypeWarning, "InsufficientGPUCapacity", msg)
			_ = r.StatusReconciler.SetCondition(ctx, &llmSvc, "GPUCapacity", metav1.ConditionFalse, "InsufficientGPUs", msg)
		} else {
			_ = r.StatusReconciler.SetCondition(ctx, &llmSvc, "GPUCapacity", metav1.ConditionTrue, "SufficientGPUs", "Cluster has enough GPU capacity")
		}
	}

	// 3. Handle Finalizers (Consolidated Cleanup)
	cleanupFunc := func() error {
		return r.cleanupResources(ctx, &llmSvc)
	}
	if deleted, err := r.CleanupReconciler.HandleFinalizer(ctx, &llmSvc, api.FinalizerName, cleanupFunc); err != nil || deleted {
		return ctrl.Result{}, err
	}

	// 2.5 Reconcile Managed ExternalSecrets (Opt-in)
	if r.ExternalSecret != nil {
		if err := r.ExternalSecret.ReconcileExternalSecret(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile external secrets: %w", err)
		}
	}

	// 3. Provision hf-csi-driver PV+PVC for hf-mount:// URIs before the pod is built.
	// No-op for all other URI schemes.
	if r.HFCSI != nil {
		if err := r.HFCSI.Reconcile(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("hf-csi provisioning: %w", err)
		}
	}

	// Fetch AIPacks once so BaseModel quantization can be injected before the
	// deployment builder runs and governance can reuse the same snapshot.
	var aipackList servingv1alpha2.AIPackList
	activePacks := []servingv1alpha2.AIPack{}
	if err := r.List(ctx, &aipackList, client.InNamespace(llmSvc.Namespace)); err != nil {
		logger.Error(err, "failed to list AIPacks for pre-deployment injection (non-blocking)")
	} else {
		for _, pack := range aipackList.Items {
			if pack.Labels["serving.ckodex.com/workload"] == llmSvc.Name {
				activePacks = append(activePacks, pack)
			}
		}
		applyAIPackConfig(&llmSvc, activePacks)
	}

	// 3b. Select the single-node Deployment or KServe-native multi-node path.
	multiNode := kserveintegration.RequiresMultiNode(&llmSvc)

	// Fetch associated LoRA adapters to inject volumes/args.
	var loraList servingv1alpha2.LLMLoraAdapterList
	activeLoras := []servingv1alpha2.LLMLoraAdapter{}
	if err := r.List(ctx, &loraList, client.InNamespace(llmSvc.Namespace)); err == nil {
		for _, lora := range loraList.Items {
			if lora.Spec.TargetService == llmSvc.Name {
				activeLoras = append(activeLoras, lora)
			}
		}
	}

	if multiNode {
		if r.KServeMultiNode == nil {
			return ctrl.Result{}, fmt.Errorf("KServe multi-node reconciler is not configured")
		}
		if len(activeLoras) > 0 {
			return ctrl.Result{}, fmt.Errorf("KServe v0.19 multi-node does not support CKodex LoRA adapters")
		}
		if err := r.KServeMultiNode.Reconcile(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile KServe multi-node InferenceService: %w", err)
		}
	} else {
		if err := r.reconcileDeployment(ctx, &llmSvc, activeLoras); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile deployment: %w", err)
		}
		// KServe owns the multi-node head/worker lifecycle and predictor Service.
		if err := r.PDBReconciler.Reconcile(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile pdb: %w", err)
		}
		if err := r.ServiceReconciler.Reconcile(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile service: %w", err)
		}
		if err := r.reconcilePrefillDeployment(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile prefill deployment: %w", err)
		}
	}

	if llmSvc.Spec.Router.Scheduler != nil {
		if r.Scheduler == nil {
			err := fmt.Errorf("scheduler is requested but the scheduler feature is disabled")
			condition := metav1.Condition{
				Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
				Reason: "SchedulerFeatureDisabled", Message: err.Error(), ObservedGeneration: llmSvc.Generation,
			}
			return r.reconcileSchedulerBlocked(ctx, &llmSvc, llmSvcBeforePatch, condition, err, 0)
		}
		ready, err := r.Scheduler.Reconcile(ctx, &llmSvc)
		condition := metav1.Condition{
			Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
			Reason: "EndpointPickerUnavailable", Message: "Waiting for the GA InferencePool and EPP readiness",
			ObservedGeneration: llmSvc.Generation,
		}
		if err != nil {
			condition.Reason = "SchedulerReconcileFailed"
			condition.Message = err.Error()
			return r.reconcileSchedulerBlocked(
				ctx, &llmSvc, llmSvcBeforePatch, condition,
				fmt.Errorf("reconcile scheduler: %w", err), 0,
			)
		}
		if !ready {
			return r.reconcileSchedulerBlocked(ctx, &llmSvc, llmSvcBeforePatch, condition, nil, 5*time.Second)
		}
		condition.Status = metav1.ConditionTrue
		condition.Reason = "EndpointPickerReady"
		condition.Message = "GA InferencePool and EPP are ready"
		meta.SetStatusCondition(&llmSvc.Status.Conditions, condition)
	} else {
		meta.RemoveStatusCondition(&llmSvc.Status.Conditions, servingv1alpha2.ConditionSchedulerReady)
		if r.Scheduler != nil {
			if err := r.Scheduler.Cleanup(ctx, &llmSvc); err != nil {
				return ctrl.Result{}, fmt.Errorf("cleanup disabled scheduler: %w", err)
			}
		}
	}

	// 5. Reconcile Gateway API resources (HTTPRoute + GRPCRoute)
	if r.Gateway != nil {
		if err := r.Gateway.Reconcile(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile gateway: %w", err)
		}
	}

	// 6. Reconcile Vector ConfigMap (OIS v0.1)
	if err := observability.ReconcileVectorConfigMap(ctx, r.Client, r.Scheme, &llmSvc, r.OTEL_Endpoint); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile vector: %w", err)
	}

	// 6. Reconcile Autoscaler (HPA/KEDA/WVA)
	if r.Autoscaler != nil && !multiNode {
		if err := r.Autoscaler.Reconcile(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile autoscaler: %w", err)
		}
	}

	// 7. Reconcile Network Security Isolation
	if r.NetworkPolicy != nil {
		if err := r.NetworkPolicy.ReconcileNetworkPolicy(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile network policy: %w", err)
		}
	}

	// 8. Reconcile ToolSurface Isolation (Istio Sidecar, mTLS, ServiceEntries, Egress)
	if r.ToolSurface != nil {
		if err := r.ToolSurface.ReconcileToolSurface(ctx, &llmSvc, activeLoras); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile tool surface: %w", err)
		}
	}

	// Aggregate Composite Trust Plan
	llmSvc.Status.StatePlanes = governance.AggregateStatePlanes(&llmSvc, activeLoras)

	// Reconcile Governance Evidence (for OSCAL/Lula validation)
	if r.GovernanceReconciler != nil {
		if err := r.GovernanceReconciler.Reconcile(ctx, &llmSvc, activeLoras); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile governance evidence: %w", err)
		}
		// Best-effort: AIPack governance failures are non-blocking; the next reconcile will retry.
		if gErr := r.GovernanceReconciler.ReconcileAIPacks(ctx, &llmSvc, activePacks); gErr != nil {
			logger.Error(gErr, "aipack governance reconcile error (non-blocking)")
		}
	}

	// 8. Reconcile Vault Agent sidecar annotations
	if r.Vault != nil {
		if err := r.Vault.ReconcileVault(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile vault: %w", err)
		}
	}
	// 9. Reconcile SPIRE (infrastructure + identity registration)
	if r.SPIRE != nil {
		if err := r.SPIRE.ReconcileSPIRE(ctx, llmSvc.Namespace); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile spire: %w", err)
		}
	}
	// 9b. Create/update SPIRE registration entry ConfigMap for this workload.
	// The ConfigMap is written to the spire namespace so the SPIRE k8s Workload Attestor
	// can automatically create the corresponding registration entry on the SPIRE server.
	if r.SPIRERegistration != nil {
		if err := r.SPIRERegistration.ReconcileRegistrationEntry(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile spire registration entry: %w", err)
		}
	}

	// 10. Reconcile eBPF (Tetragon TracingPolicy)
	if r.Ebpf != nil {
		if err := r.Ebpf.ReconcileEbpfPolicy(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile ebpf: %w", err)
		}
	}

	// 11. Reconcile OPA Gatekeeper constraints (image allowlist, resource quota, model access)
	if r.OPA != nil {
		if err := r.OPA.ReconcileOPA(ctx, llmSvc.Namespace, r.OPAConfig); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile opa: %w", err)
		}
	}

	// 13. Update status
	isOptimized := GetWellKnownConfig(llmSvc.Spec.Model.URI) != nil
	hwType := r.HardwareCache.Get(ctx, r.Client, r.APIReader)
	llmSvc.Status.DetectedHardware = string(hwType)
	// 5. Update Status (Consolidated)
	// Fetch Adaptive Metrics if available
	var metrics *servingv1alpha2.AdaptiveMetrics
	if r.Metrics != nil {
		metrics = r.Metrics.GetAdaptiveMetrics(ctx, llmSvc.Namespace, llmSvc.Name)
	}

	var statusErr error
	if multiNode {
		statusErr = r.StatusReconciler.UpdateFromKServe(ctx, &llmSvc, llmSvcBeforePatch, isOptimized, metrics)
	} else {
		statusErr = r.StatusReconciler.Update(ctx, &llmSvc, llmSvcBeforePatch, isOptimized, metrics)
	}
	if statusErr != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", statusErr)
	}

	// 14. Audit event & Receipt (OIS v0.1)
	resourceRef := fmt.Sprintf("LLMInferenceService/%s/%s", llmSvc.Namespace, llmSvc.Name)
	if r.Audit != nil {
		mode := "observe"
		if r.OPA != nil {
			mode = "enforced"
		}

		details := map[string]string{
			"replicas":    fmt.Sprintf("%d", llmSvc.Status.Replicas),
			"ready":       fmt.Sprintf("%t", llmSvc.Status.ModelReady),
			"model_uri":   llmSvc.Spec.Model.URI,
			"engine":      llmSvc.Spec.Engine,
			"exec.mode":   mode,
			"detected_hw": llmSvc.Status.DetectedHardware,
		}

		r.Audit.LogUpdate(ctx, resourceRef, "controller", details)

		// Emit OIS Receipt if the model is ready (materialized state change)
		if llmSvc.Status.ModelReady {
			r.Audit.LogReceipt(ctx, "", "ok", "Model server materialized and ready", details)
		}
	}

	logger.Info("reconciliation complete",
		"name", llmSvc.Name,
		"replicas", llmSvc.Status.Replicas,
		"ready", llmSvc.Status.ModelReady,
	)

	if multiNode && !llmSvc.Status.ModelReady {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func setSchedulerGateReadyCondition(llmSvc *servingv1alpha2.LLMInferenceService, message string) {
	meta.SetStatusCondition(&llmSvc.Status.Conditions, metav1.Condition{
		Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionFalse,
		Reason: "SchedulerUnavailable", Message: message, ObservedGeneration: llmSvc.Generation,
	})
}

// reconcileSchedulerBlocked records a scheduler readiness failure before
// reconciling the fail-closed gateway route and persisting status. Keeping
// those operations together prevents one readiness branch from silently
// skipping the status patch when gateway reconciliation fails.
func (r *LLMInferenceServiceReconciler) reconcileSchedulerBlocked(
	ctx context.Context,
	llmSvc *servingv1alpha2.LLMInferenceService,
	before *servingv1alpha2.LLMInferenceService,
	condition metav1.Condition,
	cause error,
	requeueAfter time.Duration,
) (ctrl.Result, error) {
	meta.SetStatusCondition(&llmSvc.Status.Conditions, condition)
	setSchedulerGateReadyCondition(llmSvc, condition.Message)
	if r.Gateway != nil {
		if err := r.Gateway.Reconcile(ctx, llmSvc); err != nil {
			cause = errors.Join(cause, fmt.Errorf("route scheduler fail-closed backend: %w", err))
		}
	}
	if err := r.Status().Patch(ctx, llmSvc, client.MergeFrom(before)); err != nil {
		cause = errors.Join(cause, fmt.Errorf("update scheduler readiness: %w", err))
	}
	if cause != nil {
		return ctrl.Result{}, cause
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// reconcileDeployment creates or updates the vLLM Deployment.
func (r *LLMInferenceServiceReconciler) reconcileDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, loras []servingv1alpha2.LLMLoraAdapter) error {
	logger := log.FromContext(ctx)

	// Apply WellKnown configuration defaults if not already set (e.g. for Gemma 4)
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
	desired := r.DeploymentBuilder.BuildWithRole(ctx, llmSvc, replicas, hwType, loras, role)

	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	// Check if deployment exists
	var existing appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		logger.Info("creating Deployment", "name", desired.Name)
		if err := r.Create(ctx, desired); err != nil {
			r.Recorder.Eventf(llmSvc, corev1.EventTypeWarning, "DeploymentCreationFailed",
				"Failed to create Deployment %s: %v", desired.Name, err)
			return err
		}
		r.Recorder.Eventf(llmSvc, corev1.EventTypeNormal, "DeploymentCreated",
			"Successfully created Deployment %s", desired.Name)
		return nil
	}
	if err != nil {
		r.Recorder.Eventf(llmSvc, corev1.EventTypeWarning, "DeploymentLookupFailed",
			"Failed to look up Deployment %s: %v", desired.Name, err)
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

func (r *LLMInferenceServiceReconciler) reconcilePrefillDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	name := llmSvc.Name + "-prefill"
	var existing appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing)
	if llmSvc.Spec.Prefill == nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return r.Delete(ctx, &existing)
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
	replicas := int32(1)
	if desired.Spec.Replicas != nil {
		replicas = *desired.Spec.Replicas
	}
	if reconciler.SyncDeployment(ctx, &existing, desired, replicas, false) {
		return r.Update(ctx, &existing)
	}
	return nil
}

// cleanupResources handles cleanup when the CR is deleted.
func (r *LLMInferenceServiceReconciler) cleanupResources(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)
	logger.Info("cleaning up resources", "name", llmSvc.Name)

	if r.SPIRERegistration != nil {
		if err := r.SPIRERegistration.DeleteRegistrationEntry(ctx, llmSvc.Namespace, llmSvc.Name); err != nil {
			logger.Error(err, "failed to delete SPIRE registration entry during cleanup",
				"namespace", llmSvc.Namespace, "name", llmSvc.Name)
		}
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	inferencePool := &unstructured.Unstructured{}
	inferencePool.SetGroupVersionKind(scheduler.InferencePoolGVK)

	r.APIReader = mgr.GetAPIReader()
	r.DeploymentBuilder = &deployment.Builder{
		Client:                  mgr.GetClient(),
		Recorder:                mgr.GetEventRecorderFor("ckodex-llm-operator"),
		SPIRE:                   r.SPIRE,
		EnableHardwareSelection: r.EnableHardwareSelection,
		OTEL_Endpoint:           r.OTEL_Endpoint,
		AirGappedMode:           r.AirGappedMode,
		LocalRegistry:           r.LocalRegistry,
		LocalCosignKeyPath:      r.LocalCosignKeyPath,
		RuntimeImage:            r.RuntimeImage,
		HFInitializerImage:      r.HFInitializerImage,
		HFMirrorURL:             r.HFMirrorURL,
		Defaults:                r.Defaults,
	}
	r.StatusReconciler = &status.Reconciler{
		Client:          mgr.GetClient(),
		EnableHardening: r.EnableExperimentalStatusHardening,
	}
	r.CleanupReconciler = &cleanup.Reconciler{
		Client: mgr.GetClient(),
	}
	r.ServiceReconciler = &reconciler.ServiceReconciler{
		Client:     mgr.GetClient(),
		Scheme:     r.Scheme,
		EnableGRPC: r.EnableGRPC,
	}
	r.PDBReconciler = &reconciler.PDBReconciler{
		Client: mgr.GetClient(),
		Scheme: r.Scheme,
	}
	r.GovernanceReconciler = &evidence.GovernanceReconciler{
		Client:             mgr.GetClient(),
		Scheme:             r.Scheme,
		AirGappedMode:      r.AirGappedMode,
		LocalCosignKeyPath: r.LocalCosignKeyPath,
	}
	r.HFCSI = &HFCSIReconciler{
		Client: mgr.GetClient(),
		Scheme: r.Scheme,
	}
	r.KServeMultiNode = &kserveintegration.Reconciler{
		Client:      mgr.GetClient(),
		Scheme:      r.Scheme,
		RuntimeName: r.KServeMultiNodeRuntime,
	}
	r.Recorder = mgr.GetEventRecorderFor("ckodex-llm-operator")

	builder := ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		For(&servingv1alpha2.LLMInferenceService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&gwapiv1.HTTPRoute{}).
		Owns(&gwapiv1.GRPCRoute{}).
		Owns(&gwapiv1.Gateway{})

	// The Gateway API Inference Extension CRD is installed separately from this
	// operator. Register the ownership watch only when discovery confirms that
	// the external type is available; otherwise controller-runtime retries the
	// watch forever and blocks the manager in envtest or partial installations.
	if _, err := mgr.GetRESTMapper().RESTMapping(scheduler.InferencePoolGVK.GroupKind(), scheduler.InferencePoolGVK.Version); err == nil {
		builder = builder.Owns(inferencePool)
	} else if !meta.IsNoMatchError(err) {
		return fmt.Errorf("discover InferencePool CRD: %w", err)
	}

	return builder.
		// Watch for LocalModelCache changes to update affinity
		Watches(
			&servingv1alpha2.LocalModelCache{},
			handler.EnqueueRequestsFromMapFunc(r.mapLocalModelCacheToInferenceServices),
		).
		Watches(
			&servingv1alpha2.LLMLoraAdapter{},
			handler.EnqueueRequestsFromMapFunc(r.mapLoraAdapterToInferenceService),
		).
		Complete(r)
}

// mapLoraAdapterToInferenceService maps an LLMLoraAdapter to its target LLMInferenceService.
func (r *LLMInferenceServiceReconciler) mapLoraAdapterToInferenceService(ctx context.Context, obj client.Object) []reconcile.Request {
	lora, ok := obj.(*servingv1alpha2.LLMLoraAdapter)
	if !ok {
		return nil
	}
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      lora.Spec.TargetService,
				Namespace: lora.Namespace,
			},
		},
	}
}

// mapLocalModelCacheToInferenceServices maps a LocalModelCache to all LLMInferenceServices using that model.
func (r *LLMInferenceServiceReconciler) mapLocalModelCacheToInferenceServices(ctx context.Context, obj client.Object) []reconcile.Request {
	lmc, ok := obj.(*servingv1alpha2.LocalModelCache)
	if !ok {
		return nil
	}

	var results []reconcile.Request
	var list servingv1alpha2.LLMInferenceServiceList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}

	for _, llm := range list.Items {
		if llm.Spec.Model.URI == lmc.Spec.SourceModelURI {
			results = append(results, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llm.Name,
					Namespace: llm.Namespace,
				},
			})
		}
	}
	return results
}

// applyAIPackConfig injects BaseModel quantization metadata from governance-bound AIPacks
// into the in-memory LLMInferenceService spec. The CR is never patched — injection is
// ephemeral per reconcile loop, so user-set values always win (nil-guard below).
//
// BaseModelSpec.Quantization uses free-form strings like "int4-awq", "fp8", "bf16".
// We map to our QuantizationSpec.Method enum: awq, gptq, gguf, bitsandbytes, fp8.
// Unrecognised or training-precision values (bf16, fp32) are skipped.
func applyAIPackConfig(llmSvc *servingv1alpha2.LLMInferenceService, packs []servingv1alpha2.AIPack) {
	if llmSvc.Spec.Quantization != nil {
		return // user-set value wins; nothing to inject
	}
	for i := range packs {
		pack := &packs[i]
		if pack.Spec.Kind != servingv1alpha2.KindBaseModel || pack.Spec.BaseModel == nil {
			continue
		}
		method := normalizeQuantization(pack.Spec.BaseModel.Quantization)
		if method == "" {
			continue
		}
		llmSvc.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: method}
		return // first matching BaseModel pack wins
	}
}

// normalizeQuantization maps AIPack free-form quantization strings to QuantizationSpec.Method.
// Returns "" for unrecognised or training-precision values (bf16, fp32, bfloat16).
func normalizeQuantization(q string) string {
	switch q {
	case "awq", "int4-awq", "w4a16", "int4":
		return "awq"
	case "gptq", "int4-gptq":
		return "gptq"
	case "gguf":
		return "gguf"
	case "bitsandbytes", "bnb", "int8":
		return "bitsandbytes"
	case "fp8", "w8a8", "sq", "smoothquant":
		return "fp8"
	default:
		return "" // bf16, fp32, bfloat16 are training precision, not inference quant methods
	}
}
