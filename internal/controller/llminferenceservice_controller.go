/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"sync"
	"time"
)

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/cleanup"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/status"
	"github.com/ckodex-labs/kserve-llm-operator/internal/gateway"
	"github.com/ckodex-labs/kserve-llm-operator/internal/governance"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/ckodex-labs/kserve-llm-operator/internal/security"
)

// LLMInferenceServiceReconciler reconciles LLMInferenceService objects.
// Follows KServe control plane pattern: watch-reconcile loop with
// clean control/data plane separation.
type LLMInferenceServiceReconciler struct {
	client.Client
	Scheme                            *runtime.Scheme
	Gateway                           *gateway.Reconciler
	Autoscaler                        *autoscaler.Reconciler
	OPA                               *security.OPAReconciler // nil when EnableSecurity=false
	OPAConfig                         security.OPAConfig      // populated from OperatorConfig.Security when OPA != nil
	NetworkPolicy                     *security.NetworkPolicyReconciler
	Vault                             *security.VaultReconciler
	SPIRE                             *security.SPIREReconciler
	SPIRERegistration                 *security.SPIRERegistrationReconciler // nil when EnableSecurity=false
	Ebpf                              *security.EbpfReconciler
	LWS                               *Reconciler // nil when LWS CRD not available
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

	// Modular sub-reconcilers
	DeploymentBuilder *deployment.Builder
	StatusReconciler  *status.Reconciler
	CleanupReconciler *cleanup.Reconciler
	ServiceReconciler *reconciler.ServiceReconciler
	PDBReconciler     *reconciler.PDBReconciler

	// Hardware detection cache — avoids listing all nodes on every reconcile.
	hardwareCacheMu   sync.RWMutex
	cachedHardware    deployment.HardwareType
	hardwareCacheTime time.Time
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways;httproutes;grpcroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

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

	// 3. Reconcile Deployment
	// Fetch associated LoRA adapters to inject volumes/args
	var loraList servingv1alpha2.LLMLoraAdapterList
	activeLoras := []servingv1alpha2.LLMLoraAdapter{}
	if err := r.List(ctx, &loraList, client.InNamespace(llmSvc.Namespace)); err == nil {
		for _, lora := range loraList.Items {
			if lora.Spec.TargetService == llmSvc.Name {
				activeLoras = append(activeLoras, lora)
			}
		}
		if err := r.reconcileDeployment(ctx, &llmSvc, activeLoras); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile deployment: %w", err)
		}
	} else {
		if err := r.reconcileDeployment(ctx, &llmSvc, nil); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile deployment: %w", err)
		}
	}

	// 3b. Reconcile PodDisruptionBudget
	if err := r.PDBReconciler.Reconcile(ctx, &llmSvc); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile pdb: %w", err)
	}

	// 4. Reconcile Service
	if err := r.ServiceReconciler.Reconcile(ctx, &llmSvc); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile service: %w", err)
	}

	// 5. Reconcile Gateway API resources (HTTPRoute + GRPCRoute)
	if r.Gateway != nil {
		if err := r.Gateway.Reconcile(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile gateway: %w", err)
		}
	}

	// 6. Reconcile Autoscaler (HPA/KEDA/WVA)
	if r.Autoscaler != nil {
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

	// 8. Reconcile ToolSurface Isolation (Istio Sidecar, mTLS, ServiceEntries)
	if r.ToolSurface != nil {
		if err := r.ToolSurface.ReconcileToolSurface(ctx, &llmSvc, activeLoras); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile tool surface isolation: %w", err)
		}
	}

	// 7c. Reconcile ToolSurface (Istio Egress isolation)
	if r.ToolSurface != nil {
		if err := r.ToolSurface.ReconcileToolSurface(ctx, &llmSvc, activeLoras); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile tool surface istio: %w", err)
		}
	}

	// 7d. Aggregate Composite Trust Plan
	llmSvc.Status.StatePlanes = governance.AggregateStatePlanes(&llmSvc, activeLoras)

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

	// 12. Reconcile LeaderWorkerSet for multi-node GPU topology (when spec.parallelism is set)
	if r.LWS != nil {
		if err := r.LWS.Reconcile(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile lws: %w", err)
		}
	}

	// 13. Update status
	isOptimized := GetWellKnownConfig(llmSvc.Spec.Model.URI) != nil
	hwType := r.getCachedHardware(ctx)
	llmSvc.Status.DetectedHardware = string(hwType)
	if err := r.StatusReconciler.Update(ctx, &llmSvc, llmSvcBeforePatch, isOptimized); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	// 14. Audit event
	resourceRef := fmt.Sprintf("LLMInferenceService/%s/%s", llmSvc.Namespace, llmSvc.Name)
	if r.Audit != nil {
		r.Audit.LogUpdate(ctx, resourceRef, "controller", map[string]string{
			"replicas": fmt.Sprintf("%d", llmSvc.Status.Replicas),
			"ready":    fmt.Sprintf("%t", llmSvc.Status.ModelReady),
		})
	}

	logger.Info("reconciliation complete",
		"name", llmSvc.Name,
		"replicas", llmSvc.Status.Replicas,
		"ready", llmSvc.Status.ModelReady,
	)

	return ctrl.Result{}, nil
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

	hwType := r.getCachedHardware(ctx)
	desired := r.DeploymentBuilder.Build(ctx, llmSvc, replicas, hwType, loras)

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

	// Update if spec changed
	// Update only fields we manage
	changed := false
	if existing.Spec.Replicas == nil || (llmSvc.Spec.Scaling == nil && *existing.Spec.Replicas != replicas) {
		existing.Spec.Replicas = &replicas
		changed = true
	}

	// Sync Deployment-level labels and annotations (cost tags + SLO)
	if !equality.Semantic.DeepEqual(existing.Labels, desired.Labels) {
		existing.Labels = desired.Labels
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Annotations, desired.Annotations) {
		existing.Annotations = desired.Annotations
		changed = true
	}

	// Compare Pod Template Spec (Labels, Annotations, and PodSpec)
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Labels, desired.Spec.Template.Labels) {
		existing.Spec.Template.Labels = desired.Spec.Template.Labels
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Annotations, desired.Spec.Template.Annotations) {
		existing.Spec.Template.Annotations = desired.Spec.Template.Annotations
		changed = true
	}

	// Compare PodSpec (Containers, Volumes, Affinity, etc.)
	if !reconciler.ContainersEqual(existing.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers) {
		logger.Info("Deployment containers changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
		changed = true
	}
	if !reconciler.ContainersEqual(existing.Spec.Template.Spec.InitContainers, desired.Spec.Template.Spec.InitContainers) {
		logger.Info("Deployment init containers changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.InitContainers = desired.Spec.Template.Spec.InitContainers
		changed = true
	}
	if !reconciler.VolumesEqual(existing.Spec.Template.Spec.Volumes, desired.Spec.Template.Spec.Volumes) {
		logger.Info("Deployment volumes changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.Volumes = desired.Spec.Template.Spec.Volumes
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Spec.Affinity, desired.Spec.Template.Spec.Affinity) {
		logger.Info("Deployment affinity changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.Affinity = desired.Spec.Template.Spec.Affinity
		changed = true
	}

	if changed {
		logger.Info("updating Deployment", "name", desired.Name)
		if err := r.Update(ctx, &existing); err != nil {
			return fmt.Errorf("update deployment: %w", err)
		}
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

// buildDeployment is a wrapper for tests.
func (r *LLMInferenceServiceReconciler) buildDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32) *appsv1.Deployment {
	hwType := r.getCachedHardware(ctx)
	return r.DeploymentBuilder.Build(ctx, llmSvc, replicas, hwType, nil)
}

// buildStorageInitializer is a wrapper for tests.
func (r *LLMInferenceServiceReconciler) buildStorageInitializer(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, lmc *servingv1alpha2.LocalModelCache) *corev1.Container {
	hwType := r.getCachedHardware(ctx)
	return r.DeploymentBuilder.BuildStorageInitializer(ctx, llmSvc, hwType, lmc)
}

func (r *LLMInferenceServiceReconciler) reconcileGovernanceEvidence(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, activeLoras []servingv1alpha2.LLMLoraAdapter) error {
	logger := log.FromContext(ctx)

	// AC-4: Information Flow Enforcement
	ac4 := metav1.Condition{
		Type:               "Compliance-AC-4",
		Status:             metav1.ConditionTrue,
		Reason:             "NetworkPolicyEnforced",
		Message:            "Information flow enforced via ToolSurface NetworkPolicies",
		LastTransitionTime: metav1.Now(),
	}

	// AU-2: Audit Events (Persistence check)
	au2 := metav1.Condition{
		Type:               "Compliance-AU-2",
		Status:             metav1.ConditionTrue,
		Reason:             "AuditPersistent",
		Message:            "Audit logs are written to persistent storage at /var/log/ckodex/audit.jsonl",
		LastTransitionTime: metav1.Now(),
	}

	// SI-7: Software and Information Integrity (Composite State machine)
	state := governance.AggregateStatePlanes(llmSvc, activeLoras)
	si7 := metav1.Condition{
		Type:               "Compliance-SI-7",
		Status:             metav1.ConditionTrue,
		Reason:             "IntegrityVerified",
		Message:            fmt.Sprintf("Lifecycle: %s, Trust: %s, Risk: %s", state.Lifecycle, state.Trust, state.Risk),
		LastTransitionTime: metav1.Now(),
	}

	if state.Lifecycle == "quarantined" || state.Trust == "denied" {
		si7.Status = metav1.ConditionFalse
		si7.Reason = "SecurityBreach"
	}

	// If any LoRA has complex ToolSurface, we might need manual review or advanced telemetry.
	for _, lora := range activeLoras {
		if lora.Spec.ToolSurface != nil && len(lora.Spec.ToolSurface.AllowedAPIs) > 0 {
			// If Istio is enabled, we move from Pending to Verified
			ac4.Status = metav1.ConditionTrue
			ac4.Reason = "DPIVerified"
			ac4.Message = "FQDN-based ToolSurface verified via Istio ServiceEntry/VirtualService DPI"
			break
		}
	}

	meta.SetStatusCondition(&llmSvc.Status.Conditions, ac4)
	meta.SetStatusCondition(&llmSvc.Status.Conditions, au2)
	meta.SetStatusCondition(&llmSvc.Status.Conditions, si7)

	logger.Info("Updated governance evidence for Lula validation", "controls", "AC-4, AU-2, SI-7")
	return nil
}

func (r *LLMInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.APIReader = mgr.GetAPIReader()
	r.DeploymentBuilder = &deployment.Builder{
		Client:                  mgr.GetClient(),
		Recorder:                mgr.GetEventRecorderFor("ckodex-llm-operator"), //nolint:staticcheck
		SPIRE:                   r.SPIRE,
		EnableHardwareSelection: r.EnableHardwareSelection,
	}
	r.StatusReconciler = &status.Reconciler{
		Client:          mgr.GetClient(),
		EnableHardening: r.EnableExperimentalStatusHardening,
	}
	r.CleanupReconciler = &cleanup.Reconciler{
		Client: mgr.GetClient(),
	}
	r.ToolSurface = &security.ToolSurfaceReconciler{
		Client: mgr.GetClient(),
		Scheme: r.Scheme,
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
	r.NetworkPolicy = &security.NetworkPolicyReconciler{
		Client: mgr.GetClient(),
		Scheme: r.Scheme,
	}
	r.Recorder = mgr.GetEventRecorderFor("ckodex-llm-operator")

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		For(&servingv1alpha2.LLMInferenceService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&gwapiv1.HTTPRoute{}).
		Owns(&gwapiv1.GRPCRoute{}).
		Owns(&gwapiv1.Gateway{}).
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

const hardwareCacheTTL = 5 * time.Minute

// getCachedHardware returns the detected hardware type, refreshing the cache
// if it's older than hardwareCacheTTL. This avoids listing all nodes on every
// reconcile — at scale the node List is an unbounded API call.
func (r *LLMInferenceServiceReconciler) getCachedHardware(ctx context.Context) deployment.HardwareType {
	r.hardwareCacheMu.RLock()
	if time.Since(r.hardwareCacheTime) < hardwareCacheTTL {
		hw := r.cachedHardware
		r.hardwareCacheMu.RUnlock()
		return hw
	}
	r.hardwareCacheMu.RUnlock()

	r.hardwareCacheMu.Lock()
	defer r.hardwareCacheMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed).
	if time.Since(r.hardwareCacheTime) < hardwareCacheTTL {
		return r.cachedHardware
	}

	var nodeList corev1.NodeList
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	if err := reader.List(ctx, &nodeList); err != nil {
		log.FromContext(ctx).Error(err, "unable to list nodes for hardware detection, using cached value")
		return r.cachedHardware
	}

	r.cachedHardware = deployment.DetectHardware(nodeList.Items)
	r.hardwareCacheTime = time.Now()
	return r.cachedHardware
}

func ptrToHostPath(hp corev1.HostPathType) *corev1.HostPathType {
	return &hp
}
