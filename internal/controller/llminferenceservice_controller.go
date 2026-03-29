/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ckodex-labs/kserve-llm-operator/internal/storage"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/auth"
	"github.com/ckodex-labs/kserve-llm-operator/internal/autoscaler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/gateway"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/ckodex-labs/kserve-llm-operator/internal/security"
)

// LLMInferenceServiceReconciler reconciles LLMInferenceService objects.
// Follows KServe control plane pattern: watch-reconcile loop with
// clean control/data plane separation.
type LLMInferenceServiceReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Gateway           *gateway.Reconciler
	Autoscaler        *autoscaler.Reconciler
	OPA               *security.OPAReconciler // nil when EnableSecurity=false
	OPAConfig         security.OPAConfig      // populated from OperatorConfig.Security when OPA != nil
	NetworkPolicy     *security.NetworkPolicyReconciler
	Vault             *security.VaultReconciler
	SPIRE             *security.SPIREReconciler
	SPIRERegistration *security.SPIRERegistrationReconciler // nil when EnableSecurity=false
	Ebpf              *security.EbpfReconciler
	LWS               *Reconciler // nil when LWS CRD not available
	Audit             *observability.AuditLogger
	Inst              *observability.Instrumentation // nil → no forbidden-tuple metrics emitted
	AuthMiddleware    *auth.Middleware               // nil when EnableAuth=false
	BudgetEnforcer    *auth.TokenBudgetEnforcer      // nil when EnableAuth=false
	EnableGRPC        bool                           // When true, adds gRPC port to Service and reconciles GRPCRoute
	Recorder          record.EventRecorder
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

// Reconcile implements the main reconcile loop.
func (r *LLMInferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the LLMInferenceService CR
	var llmSvc servingv1alpha2.LLMInferenceService
	if err := r.Get(ctx, req.NamespacedName, &llmSvc); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("LLMInferenceService not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetch LLMInferenceService: %w", err)
	}

	// Capture original object for diffing and patching at the end
	llmSvcBeforePatch := llmSvc.DeepCopy()

	// 2. Handle finalizer for cleanup
	if llmSvc.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&llmSvc, FinalizerName) {
			if err := r.cleanupResources(ctx, &llmSvc); err != nil {
				return ctrl.Result{}, fmt.Errorf("cleanup resources: %w", err)
			}
			controllerutil.RemoveFinalizer(&llmSvc, FinalizerName)
			if err := r.Update(ctx, &llmSvc); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&llmSvc, FinalizerName) {
		controllerutil.AddFinalizer(&llmSvc, FinalizerName)
		if err := r.Update(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
	}

	// 3. Reconcile Deployment
	if err := r.reconcileDeployment(ctx, &llmSvc); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile deployment: %w", err)
	}

	// 3b. Reconcile PodDisruptionBudget
	if err := r.reconcilePDB(ctx, &llmSvc); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile pdb: %w", err)
	}

	// 4. Reconcile Service
	if err := r.reconcileService(ctx, &llmSvc); err != nil {
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

	// 7. Reconcile Network Policies (default-deny + allow-gateway)
	if r.NetworkPolicy != nil {
		if err := r.NetworkPolicy.ReconcileNetworkPolicies(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile network policies: %w", err)
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

	// 12. Reconcile LeaderWorkerSet for multi-node GPU topology (when spec.parallelism is set)
	if r.LWS != nil {
		if err := r.LWS.Reconcile(ctx, &llmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile lws: %w", err)
		}
	}

	// 13. Update status
	if err := r.updateStatus(ctx, &llmSvc, llmSvcBeforePatch); err != nil {
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
func (r *LLMInferenceServiceReconciler) reconcileDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)

	replicas := int32(1)
	if llmSvc.Spec.Replicas != nil {
		replicas = *llmSvc.Spec.Replicas
	}

	desired := r.buildDeployment(ctx, llmSvc, replicas)

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
	if !r.containersEqual(existing.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers) {
		logger.Info("Deployment containers changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
		changed = true
	}
	if !r.containersEqual(existing.Spec.Template.Spec.InitContainers, desired.Spec.Template.Spec.InitContainers) {
		logger.Info("Deployment init containers changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.InitContainers = desired.Spec.Template.Spec.InitContainers
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Spec.Volumes, desired.Spec.Template.Spec.Volumes) {
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
		return r.Update(ctx, &existing)
	}

	return nil
}

// buildDeployment constructs the desired Deployment spec.
func (r *LLMInferenceServiceReconciler) buildDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32) *appsv1.Deployment {
	logger := log.FromContext(ctx)
	labels := map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
		// Sanitize model name for safe label value (replace / with .)
		"serving.ckodex.com/model": strings.ReplaceAll(llmSvc.Spec.Model.Name, "/", "."),
	}

	// Start from user's pod template
	podSpec := llmSvc.Spec.Template.Spec.DeepCopy()

	// Apply hardware-aware optimizations if enabled or by default
	if llmSvc.Spec.AutoOptimize == nil || *llmSvc.Spec.AutoOptimize {
		r.applyHardwareOptimizations(ctx, llmSvc, podSpec)
	}

	// Phase 5 Hardening: Enforce default resources and termination grace period
	if podSpec.TerminationGracePeriodSeconds == nil {
		podSpec.TerminationGracePeriodSeconds = ptr.To(int64(DefaultTerminationGracePeriod))
	}

	// Ensure the primary container (vLLM) has resource requests/limits.
	if len(podSpec.Containers) > 0 {
		c := &podSpec.Containers[0]
		if c.Resources.Requests == nil {
			c.Resources.Requests = make(corev1.ResourceList)
		}
		if _, ok := c.Resources.Requests[corev1.ResourceCPU]; !ok {
			c.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(DefaultVLLMCPURequest)
		}
		if _, ok := c.Resources.Requests[corev1.ResourceMemory]; !ok {
			c.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(DefaultVLLMMemoryRequest)
		}
		// Set limits to match requests (Guaranteed QoS) for performance stability.
		if c.Resources.Limits == nil {
			c.Resources.Limits = make(corev1.ResourceList)
		}
		if _, ok := c.Resources.Limits[corev1.ResourceCPU]; !ok {
			c.Resources.Limits[corev1.ResourceCPU] = c.Resources.Requests[corev1.ResourceCPU]
		}
		if _, ok := c.Resources.Limits[corev1.ResourceMemory]; !ok {
			c.Resources.Limits[corev1.ResourceMemory] = c.Resources.Requests[corev1.ResourceMemory]
		}
	}

	// Inject StorageInitializer init container for hf:// URIs
	// Unless skipped due to Zero-Copy LocalModelCache
	skipInitializer := false

	// Phase 6: LocalModelCache (Zero-Copy)
	var lmcList servingv1alpha2.LocalModelCacheList
	var activeLMC *servingv1alpha2.LocalModelCache
	if err := r.List(ctx, &lmcList); err == nil {
		for _, lmc := range lmcList.Items {
			if lmc.Spec.SourceModelURI == llmSvc.Spec.Model.URI {
				activeLMC = &lmc
				break
			}
		}
	}

	if activeLMC != nil {
		readyNodes := []string{}
		for _, ns := range activeLMC.Status.NodeStatuses {
			if ns.Phase == "Ready" {
				readyNodes = append(readyNodes, ns.NodeName)
			}
		}

		if len(readyNodes) > 0 {
			logger.Info("Active LocalModelCache found", "lmc", activeLMC.Name, "nodes", readyNodes)
			skipInitializer = true

			// 1. Inject Node Affinity to pin to cached nodes
			if podSpec.Affinity == nil {
				podSpec.Affinity = &corev1.Affinity{}
			}
			if podSpec.Affinity.NodeAffinity == nil {
				podSpec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
			}
			podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpIn,
								Values:   readyNodes,
							},
						},
					},
				},
			}

			// 2. Override Model Volume with Zero-Copy Mount (HostPath for KIND/Local)
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: ModelVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: fmt.Sprintf("/tmp/ckodex/models/%s", activeLMC.Name),
						Type: ptrToHostPath(corev1.HostPathType(corev1.HostPathDirectoryOrCreate)),
					},
				},
			})
			logger.Info("Zero-Copy volume injected via HostPath, initializer will be bypassed")
		}
	}

	if !skipInitializer {
		initContainer := r.buildStorageInitializer(ctx, llmSvc, activeLMC)
		if initContainer != nil {
			podSpec.InitContainers = append([]corev1.Container{*initContainer}, podSpec.InitContainers...)
		}
	}

	// Ensure model volume exists
	hasModelVolume := false
	for _, v := range podSpec.Volumes {
		if v.Name == ModelVolumeName {
			hasModelVolume = true
			break
		}
	}
	if !hasModelVolume {
		uri := llmSvc.Spec.Model.URI
		switch {
		case strings.HasPrefix(uri, "modelpack://"):
			// Leverage Model CSI Driver natively, extracting oci reference
			ref := strings.TrimPrefix(uri, "modelpack://")
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: ModelVolumeName,
				VolumeSource: corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver: "model.csi.modelpack.org",
						VolumeAttributes: map[string]string{
							"modelRef": ref,
						},
					},
				},
			})

		case strings.HasPrefix(uri, "hf-mount://"):
			// HuggingFace CSI driver: lazy-mount repo via NFS/FUSE.
			// Only accessed bytes are fetched — no full model download needed.
			// URI format: hf-mount://org/repo or hf-mount://org/repo@revision
			repoPath := strings.TrimPrefix(uri, "hf-mount://")
			repo := repoPath
			revision := ""
			if idx := strings.Index(repoPath, "@"); idx != -1 {
				repo = repoPath[:idx]
				revision = repoPath[idx+1:]
			}

			volAttrs := map[string]string{
				"repo":     repo,
				"readOnly": "true",
			}
			if revision != "" {
				volAttrs["revision"] = revision
			}

			// Pass HF token from storage secret if configured
			if llmSvc.Spec.Model.Storage != nil && llmSvc.Spec.Model.Storage.SecretRef != nil {
				volAttrs["tokenSecret"] = llmSvc.Spec.Model.Storage.SecretRef.Name
			}

			readOnly := true
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: ModelVolumeName,
				VolumeSource: corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver:           HFMountCSIDriver,
						ReadOnly:         &readOnly,
						VolumeAttributes: volAttrs,
					},
				},
			})

		default:
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: ModelVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})
		}
	}

	// Ensure model volume mount on first container
	if len(podSpec.Containers) > 0 {
		hasMount := false
		for _, m := range podSpec.Containers[0].VolumeMounts {
			if m.Name == ModelVolumeName {
				hasMount = true
				break
			}
		}
		if !hasMount {
			podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
				Name:      ModelVolumeName,
				MountPath: ModelMountPath,
			})
		}
	}

	// Default resource requests/limits on the main container to prevent OOM kills
	// and scheduler misplacement when the user has not specified resources.
	if len(podSpec.Containers) > 0 {
		res := &podSpec.Containers[0].Resources
		if res.Requests == nil && res.Limits == nil {
			res.Requests = corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			}
			res.Limits = corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			}
		}
	}

	// Add health probes so pods are only marked Ready once vLLM has loaded the model.
	// Without these, the Gateway routes traffic to pods still loading — causing 503s.
	if len(podSpec.Containers) > 0 && podSpec.Containers[0].ReadinessProbe == nil {
		podSpec.Containers[0].ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/v1/models",
					Port: intstr.FromInt32(8000),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    30, // 5 min total (30 * 10s) for CPU model loading
		}
	}
	if len(podSpec.Containers) > 0 && podSpec.Containers[0].LivenessProbe == nil {
		podSpec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health", // vLLM health endpoint (available in all versions)
					Port: intstr.FromInt32(8000),
				},
			},
			InitialDelaySeconds: 120, // CPU model loading can take minutes
			PeriodSeconds:       15,
			TimeoutSeconds:      5,
			FailureThreshold:    10,
		}
	}

	// Enforce container-level security context for vLLM.
	// vLLM requires root in some images due to getpwuid() calls during startup
	// and NUMA thread binding. Set per-container (not pod-level) to minimize
	// blast radius and allow sidecars to run non-root.
	if len(podSpec.Containers) > 0 {
		c := &podSpec.Containers[0]
		if c.SecurityContext == nil {
			c.SecurityContext = &corev1.SecurityContext{}
		}
		if c.SecurityContext.RunAsUser == nil {
			c.SecurityContext.RunAsUser = ptr.To(int64(0))
			c.SecurityContext.RunAsNonRoot = ptr.To(false)
		}
	}

	// Enforce vLLM environment defaults (fix getpwuid error for non-root)
	if len(podSpec.Containers) > 0 {
		vllmContainer := &podSpec.Containers[0]
		hasHome := false
		for _, e := range vllmContainer.Env {
			if e.Name == "HOME" {
				hasHome = true
				break
			}
		}
		if !hasHome {
			vllmContainer.Env = append(vllmContainer.Env, corev1.EnvVar{
				Name:  "HOME",
				Value: "/tmp",
			})
		}
		hasTorchCache := false
		for _, e := range vllmContainer.Env {
			if e.Name == "TORCHINDUCTOR_CACHE_DIR" {
				hasTorchCache = true
				break
			}
		}
		if !hasTorchCache {
			vllmContainer.Env = append(vllmContainer.Env, corev1.EnvVar{
				Name:  "TORCHINDUCTOR_CACHE_DIR",
				Value: "/tmp",
			})
		}
		// Set vLLM logging level (INFO for production, DEBUG for troubleshooting)
		vllmContainer.Env = append(vllmContainer.Env, corev1.EnvVar{
			Name:  "VLLM_LOGGING_LEVEL",
			Value: "INFO",
		})
	}
	// Phase 8: OpenTelemetry Tracking
	if llmSvc.Annotations == nil {
		llmSvc.Annotations = make(map[string]string)
	}
	// We'll use the Deployment template annotations instead of PodSpec

	// Phase 7: SPIFFE/SPIRE Zero-Trust Identity
	if r.SPIRE != nil {
		r.SPIRE.InjectSidecar(podSpec, llmSvc)
		logger.Info("SPIFFE/SPIRE sidecar injected for zero-trust identity")
	}

	if len(podSpec.Containers) > 0 {
		logger.Info("Finalizing desired vllm container env", "count", len(podSpec.Containers[0].Env), "env", podSpec.Containers[0].Env)
	}

	// Propagate CostAllocationTags as Deployment labels (prefix: ckodex.cost.)
	// so FinOps tooling (Kubecost, OpenCost, etc.) can group GPU costs by team/project.
	for k, v := range llmSvc.Spec.CostAllocationTags {
		labels["ckodex.cost/"+strings.ReplaceAll(k, ".", "-")] = v
	}

	// SLO annotations — readable by monitoring agents without parsing CRD spec.
	annotations := map[string]string{}
	if llmSvc.Spec.SLO != nil {
		annotations["ckodex.com/slo-p99-latency-ms"] = fmt.Sprintf("%d", llmSvc.Spec.SLO.TargetP99LatencyMs)
		annotations["ckodex.com/slo-availability"] = fmt.Sprintf("%.4f", llmSvc.Spec.SLO.TargetAvailability)
		annotations["ckodex.com/slo-error-budget-days"] = fmt.Sprintf("%d", llmSvc.Spec.SLO.ErrorBudgetDays)
	}
	if llmSvc.Spec.Canary != nil {
		annotations["ckodex.com/canary-weight"] = fmt.Sprintf("%d", llmSvc.Spec.Canary.Weight)
		annotations["ckodex.com/canary-base-model"] = llmSvc.Spec.Canary.BaseModel
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        llmSvc.Name,
			Namespace:   llmSvc.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"prometheus.io/scrape":          "true",
						"prometheus.io/port":            "8000",
						"otel.sidecar.inject":           "true", // Automated OTel injection
						"ckodex.com/otel-trace-enabled": "true",
					},
				},
				Spec: *podSpec,
			},
		},
	}
}

// buildStorageInitializer creates an init container for model download.
func (r *LLMInferenceServiceReconciler) buildStorageInitializer(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, activeLMC *servingv1alpha2.LocalModelCache) *corev1.Container {
	logger := log.FromContext(ctx)
	uri := llmSvc.Spec.Model.URI
	logger.Info("Building storage initializer", "uri", uri)
	if uri == "" || strings.HasPrefix(uri, "modelpack://") || strings.HasPrefix(uri, "hf-mount://") {
		// Native CSI mounts (modelpack, hf-mount) handle volume mounting without an init container.
		// hf-mount uses the HuggingFace CSI driver to lazily mount repos via NFS/FUSE.
		return nil
	}

	// Detect scheme
	parts := strings.SplitN(uri, "://", 2)
	scheme := ""
	if len(parts) > 1 {
		scheme = parts[0]
	}

	initializerImage := StorageInitializerImage
	// If it's one of our natively supported schemes, use our custom initializer
	// Exception: HuggingFace is better handled by the KServe initializer for now (snapshot download).
	if scheme != "hf" && scheme != "huggingface" {
		if _, err := storage.GetClient(scheme); err == nil {
			initializerImage = CKodexStorageInitializerImage
		}
	}

	// On ARM64 (Apple Silicon), always prefer our custom storage initializer which
	// is built with TARGETARCH support. The upstream kserve/storage-initializer
	// may not publish ARM64 manifests.
	var nodeList corev1.NodeList
	if err := r.List(ctx, &nodeList); err == nil {
		hwType := r.detectHardware(nodeList.Items)
		if hwType == HardwareAppleSilicon {
			initializerImage = CKodexStorageInitializerImage
		}
	}

	// Check if we should use LocalModelCache (Zero-Copy)
	isZeroCopyReady := false
	if activeLMC != nil {
		for _, ns := range activeLMC.Status.NodeStatuses {
			if ns.Phase == "Ready" {
				isZeroCopyReady = true
				break
			}
		}
	}

	if isZeroCopyReady {
		// Bypass storage-initializer completely
		// vLLM will download the model directly
		return nil
	}

	// CKODEX §10: empty ∧ DAL≥3 → HALT.
	// A storage pull is a cross-boundary operation (DAL ≥ 3). If no credentials
	// are configured (Storage == nil), the operator is in the "empty" state.
	// Emit the forbidden-tuple counter so dashboards and alerts can fire.
	if llmSvc.Spec.Model.Storage == nil && r.Inst != nil {
		r.Inst.ForbiddenTupleCounter.Add(ctx, 1,
			observability.TupleTypeAttr("empty_high_dal"))
	}

	runAsUser := int64(1000)
	runAsNonRoot := true

	container := &corev1.Container{
		Name:            "storage-initializer",
		Image:           initializerImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{uri, ModelMountPath},
		Env: []corev1.EnvVar{
			{
				// Full URL including scheme — required by boto3 endpoint_url and aws-sdk-go-v2.
				Name:  "S3_ENDPOINT",
				Value: SeaweedFSFilerS3Endpoint,
			},
			{
				// AWS SDK standard env var (aws-sdk-go-v2 v1.21+, botocore 1.29+).
				Name:  "AWS_ENDPOINT_URL",
				Value: SeaweedFSFilerS3Endpoint,
			},
			{
				Name:  "AWS_NO_SIGN_REQUEST",
				Value: "yes",
			},
			{
				Name:  "S3_USE_HTTPS",
				Value: "false",
			},
			{
				// SeaweedFS requires path-style: http://host/bucket/key.
				// Virtual-hosted style (http://bucket.host/key) fails — no wildcard DNS in-cluster.
				Name:  "S3_USE_PATH_STYLE",
				Value: "true",
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:    &runAsUser,
			RunAsNonRoot: &runAsNonRoot,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      ModelVolumeName,
				MountPath: ModelMountPath,
			},
		},
	}

	if llmSvc.Spec.Model.Storage != nil {
		if llmSvc.Spec.Model.Storage.SecretRef != nil {
			container.EnvFrom = []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: *llmSvc.Spec.Model.Storage.SecretRef,
					},
				},
			}
		}

		if llmSvc.Spec.Model.Storage.VaultRef != "" {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  "VAULT_PATH",
				Value: llmSvc.Spec.Model.Storage.VaultRef,
			})
		}
		if llmSvc.Spec.Model.Storage.VaultAddr != "" {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  "VAULT_ADDR",
				Value: llmSvc.Spec.Model.Storage.VaultAddr,
			})
		}
	}

	return container
}

// reconcilePDB creates or updates a PodDisruptionBudget for the inference deployment.
func (r *LLMInferenceServiceReconciler) reconcilePDB(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)

	labels := map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
	}

	minAvailable := intstr.FromInt32(1)
	desired := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name,
			Namespace: llmSvc.Namespace,
			Labels:    labels,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference on PDB: %w", err)
	}

	var existing policyv1.PodDisruptionBudget
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		logger.Info("creating PodDisruptionBudget", "name", desired.Name)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get PDB: %w", err)
	}

	// Update only if spec changed
	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		logger.Info("updating PodDisruptionBudget", "name", desired.Name)
		return r.Update(ctx, &existing)
	}

	return nil
}

// reconcileService creates or updates the inference Service.
func (r *LLMInferenceServiceReconciler) reconcileService(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)

	labels := map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
	}

	ports := []corev1.ServicePort{
		{
			Name:       "http-inference",
			Protocol:   corev1.ProtocolTCP,
			Port:       80,
			TargetPort: intstr.FromInt32(8000),
		},
	}
	if r.EnableGRPC {
		ports = append(ports, corev1.ServicePort{
			Name:       "grpc-inference",
			Protocol:   corev1.ProtocolTCP,
			Port:       8001,
			TargetPort: intstr.FromInt32(8001),
		})
	}

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name,
			Namespace: llmSvc.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    ports,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	var existing corev1.Service
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		logger.Info("creating Service", "name", desired.Name)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get service: %w", err)
	}

	// Update only if ports or selector changed (preserve ClusterIP)
	if !equality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) ||
		!equality.Semantic.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		existing.Spec.Ports = desired.Spec.Ports
		existing.Spec.Selector = desired.Spec.Selector
		logger.Info("updating Service", "name", desired.Name)
		return r.Update(ctx, &existing)
	}

	return nil
}

// updateStatus updates the LLMInferenceService status.
func (r *LLMInferenceServiceReconciler) updateStatus(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, llmSvcBeforePatch *servingv1alpha2.LLMInferenceService) error {
	// Get the deployment to check readiness
	var deploy appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}, &deploy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			llmSvc.Status.Replicas = 0
			llmSvc.Status.ModelReady = false
		} else {
			return fmt.Errorf("get deployment for status: %w", err)
		}
	} else {
		llmSvc.Status.Replicas = deploy.Status.ReadyReplicas
		llmSvc.Status.ModelReady = deploy.Status.ReadyReplicas > 0
	}

	llmSvc.Status.ObservedGeneration = llmSvc.Generation

	// Set conditions
	readyCondition := metav1.Condition{
		Type:               servingv1alpha2.ConditionReady,
		ObservedGeneration: llmSvc.Generation,
		LastTransitionTime: metav1.Now(),
	}
	if llmSvc.Status.ModelReady {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "Ready"
		readyCondition.Message = "Model is loaded and serving"
	} else {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "NotReady"
		readyCondition.Message = "Waiting for model pods to become ready"
	}

	// Update or add condition
	found := false
	for i, c := range llmSvc.Status.Conditions {
		if c.Type == servingv1alpha2.ConditionReady {
			llmSvc.Status.Conditions[i] = readyCondition
			found = true
			break
		}
	}
	if !found {
		llmSvc.Status.Conditions = append(llmSvc.Status.Conditions, readyCondition)
	}

	// Set URL
	llmSvc.Status.URL = fmt.Sprintf("http://%s.%s.svc.cluster.local/v2/models/%s",
		llmSvc.Name, llmSvc.Namespace, llmSvc.Spec.Model.Name)

	// Only patch status if it actually changed
	if !equality.Semantic.DeepEqual(&llmSvcBeforePatch.Status, &llmSvc.Status) {
		err := r.Status().Patch(ctx, llmSvc, client.MergeFrom(llmSvcBeforePatch))
		if err != nil {
			if apierrors.IsConflict(err) {
				// Conflict is benign during status updates; we'll retry on next reconciliation
				return nil
			}
			return err
		}
	}
	return nil
}

// cleanupResources handles cleanup when the CR is deleted.
func (r *LLMInferenceServiceReconciler) cleanupResources(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)
	logger.Info("cleaning up resources", "name", llmSvc.Name)

	// Owned resources (Deployment, Service, HTTPRoute, PDB) are garbage collected via
	// owner references automatically. Non-owned cross-namespace resources need explicit cleanup.

	// Remove the SPIRE registration entry ConfigMap from the spire namespace.
	// This is cross-namespace and not owned by the LLMInferenceService, so owner-ref GC
	// does not handle it.
	if r.SPIRERegistration != nil {
		if err := r.SPIRERegistration.DeleteRegistrationEntry(ctx, llmSvc.Namespace, llmSvc.Name); err != nil {
			logger.Error(err, "failed to delete SPIRE registration entry during cleanup",
				"namespace", llmSvc.Namespace, "name", llmSvc.Name)
			// Non-fatal: log and continue so the finalizer is not blocked.
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
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
		Complete(r)
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

type HardwareType string

const (
	HardwareAppleSilicon    HardwareType = "AppleSilicon"    // ARM64 in containers (CPU mode)
	HardwareAppleSiliconMPS HardwareType = "AppleSiliconMPS" // ARM64 with Metal GPU (native macOS)
	HardwareNVIDIA          HardwareType = "NVIDIA"
	HardwareAMD             HardwareType = "AMD"
	HardwareGenericX86      HardwareType = "GenericX86"
	HardwareUnknown         HardwareType = "Unknown"

	// vLLM Images
	VLLMGenericImage  = "public.ecr.aws/q9t5s3a7/vllm-cpu-release-repo:v0.17.1"
	VLLMCPUArm64Image = "vllm/vllm-openai-cpu:latest" // CPU-compiled ARM64 image for Linux containers
	VLLMMPSImage      = "vllm/vllm-openai-cpu:latest" // MPS native macOS also uses CPU image in containers
	VLLMROCmImage     = "vllm/vllm-openai:latest-rocm"
)

// detectHardware identifies the best available hardware across all nodes.
// It examines every node and returns the highest-priority hardware type found.
// Priority: NVIDIA > AMD > AppleSiliconMPS > AppleSilicon > GenericX86 > Unknown.
func (r *LLMInferenceServiceReconciler) detectHardware(nodes []corev1.Node) HardwareType {
	if len(nodes) == 0 {
		return HardwareUnknown
	}

	// Priority map: higher = preferred when multiple hardware types are present.
	priority := map[HardwareType]int{
		HardwareUnknown:         0,
		HardwareGenericX86:      1,
		HardwareAppleSilicon:    2,
		HardwareAppleSiliconMPS: 3,
		HardwareAMD:             4,
		HardwareNVIDIA:          5,
	}

	best := HardwareUnknown
	for _, node := range nodes {
		var detected HardwareType

		// 1. NVIDIA GPU (highest priority accelerator)
		if qty, ok := node.Status.Capacity["nvidia.com/gpu"]; ok && !qty.IsZero() {
			detected = HardwareNVIDIA
		} else if node.Labels["nvidia.com/gpu.present"] == "true" {
			detected = HardwareNVIDIA

			// 2. AMD GPU (ROCm)
		} else if qty, ok := node.Status.Capacity["amd.com/gpu"]; ok && !qty.IsZero() {
			detected = HardwareAMD

			// 3. Apple Silicon with Metal GPU
		} else if node.Status.NodeInfo.Architecture == "arm64" {
			if qty, ok := node.Status.Capacity["apple.com/gpu"]; ok && !qty.IsZero() {
				detected = HardwareAppleSiliconMPS
			} else if node.Labels["apple.com/gpu.present"] == "true" {
				detected = HardwareAppleSiliconMPS
			} else {
				// ARM64 without Metal GPU → CPU mode (Docker/KIND runs Linux containers)
				detected = HardwareAppleSilicon
			}

			// 4. Generic x86_64
		} else if node.Status.NodeInfo.Architecture == "amd64" {
			detected = HardwareGenericX86
		}

		if priority[detected] > priority[best] {
			best = detected
		}
	}

	return best
}

// applyHardwareOptimizations detects the underlying hardware and applies
// best-practice defaults for the specific environment.
func (r *LLMInferenceServiceReconciler) applyHardwareOptimizations(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	logger := log.FromContext(ctx)

	var nodeList corev1.NodeList
	if err := r.List(ctx, &nodeList); err != nil {
		logger.Error(err, "unable to list nodes for hardware detection")
		return
	}

	hwType := r.detectHardware(nodeList.Items)
	if hwType == HardwareUnknown {
		return
	}

	logger.Info("Hardware detected, applying optimizations", "type", hwType)

	if len(podSpec.Containers) == 0 {
		return
	}
	container := &podSpec.Containers[0]

	envVars := make(map[string]string)
	args := []string{}

	switch hwType {
	case HardwareAppleSiliconMPS:
		// Native macOS with Metal GPU exposed via apple.com/gpu capacity.
		// MPS (Metal Performance Shaders) provides GPU acceleration for inference.
		if container.Image == "" || container.Image == "vllm/vllm-openai:latest" || strings.Contains(container.Image, "cuda") {
			container.Image = VLLMMPSImage
		}
		envVars["VLLM_TARGET_DEVICE"] = "mps"
		envVars["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0.0" // Let PyTorch use all available Metal GPU memory
		args = append(args, "--device", "mps", "--host", "0.0.0.0", "--port", "8000")
		logger.Info("Metal GPU (MPS) acceleration enabled for Apple Silicon")

	case HardwareAppleSilicon:
		// ARM64 in containers (KIND/Docker runs Linux, not macOS).
		// MPS is unavailable — use CPU mode with CPU-compiled vLLM image.
		if container.Image == "" || container.Image == "vllm/vllm-openai:latest" || strings.Contains(container.Image, "cuda") {
			container.Image = VLLMCPUArm64Image
		}
		// Note: VLLM_TARGET_DEVICE is ignored by vLLM v0.17+ — platform detection is compile-time.
		// The CPU-compiled image (vllm-openai-cpu) auto-detects the CPU platform.
		// Disable NUMA thread binding — containers on Apple Silicon lack NUMA topology.
		// vLLM v0.18+ uses VLLM_CPU_OMP_THREADS_BIND with value "nobind" to skip NUMA binding.
		envVars["VLLM_CPU_OMP_THREADS_BIND"] = "nobind"
		envVars["VLLM_CPU_KVCACHE_SPACE"] = "4"
		// Limit max model length on CPU to fit KV cache within available memory.
		// Users can override via spec.template.spec.containers[0].args.
		args = append(args, "--host", "0.0.0.0", "--port", "8000", "--max-model-len", "4096")

	case HardwareGenericX86:
		// Always force CPU image if AutoOptimize is on and image name looks like default CUDA
		if container.Image == "" || container.Image == "vllm/vllm-openai:latest" || strings.Contains(container.Image, "cuda") {
			container.Image = VLLMGenericImage
		}
		// Note: VLLM_TARGET_DEVICE is ignored by vLLM v0.17+ — platform detection is compile-time.
		envVars["VLLM_CPU_KVCACHE_SPACE"] = "10"
		envVars["VLLM_CPU_OMP_THREADS_BIND"] = "auto"
		envVars["NVIDIA_VISIBLE_DEVICES"] = ""
		envVars["TORCHINDUCTOR_FREEZING"] = "1"
		args = append(args, "--device", "cpu", "--host", "0.0.0.0", "--port", "8000", "--max-model-len", "4096")
	case HardwareNVIDIA:
		envVars["VLLM_TARGET_DEVICE"] = "cuda"
		envVars["GPU_MEMORY_UTILIZATION"] = "0.9"
		envVars["VLLM_ENABLE_CUDA_COMPATIBILITY"] = "true"

	case HardwareAMD:
		if container.Name == "" {
			container.Name = "vllm"
		}
		if container.Image == "" || !strings.Contains(container.Image, "-rocm") {
			container.Image = VLLMROCmImage
		}
		envVars["VLLM_TARGET_DEVICE"] = "rocm"
		envVars["VLLM_ATTENTION_BACKEND"] = "ROCM_FLASH_ATTN"

	}

	// Apply Environment Variables in deterministic order
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := envVars[k]
		found := false
		for i, ev := range container.Env {
			if ev.Name == k {
				// Only override if empty or "auto"
				if ev.Value == "" || ev.Value == "auto" {
					container.Env[i].Value = v
				}
				found = true
				break
			}
		}
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{Name: k, Value: v})
		}
	}

	// Apply Command Arguments (only if not already present)
	for i := 0; i < len(args); i += 2 {
		flag := args[i]
		value := args[i+1]
		found := false
		for _, a := range container.Args {
			if a == flag {
				found = true
				break
			}
		}
		if !found {
			container.Args = append(container.Args, flag, value)
		}
	}
}

// containersEqual compares two slices of containers looking only at managed fields.
func (r *LLMInferenceServiceReconciler) volumesEqual(a, b []corev1.VolumeMount) bool {
	// Helper to filter out system-injected mounts
	filterManaged := func(vms []corev1.VolumeMount) []corev1.VolumeMount {
		managed := []corev1.VolumeMount{}
		for _, vm := range vms {
			if !strings.HasPrefix(vm.Name, "kube-api-access-") && vm.Name != "default-token" {
				managed = append(managed, vm)
			}
		}
		return managed
	}
	return equality.Semantic.DeepEqual(filterManaged(a), filterManaged(b))
}

func (r *LLMInferenceServiceReconciler) containersEqual(a, b []corev1.Container) bool {
	logger := ctrl.Log.WithName("containersEqual")
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			logger.V(1).Info("container name mismatch", "existing", a[i].Name, "desired", b[i].Name)
			return false
		}
		if a[i].Image != b[i].Image {
			logger.V(1).Info("container image mismatch", "container", a[i].Name, "existing", a[i].Image, "desired", b[i].Image)
			return false
		}
		if !equality.Semantic.DeepEqual(a[i].Args, b[i].Args) {
			logger.V(1).Info("container args mismatch", "container", a[i].Name)
			return false
		}

		// Sort Env of both containers before comparison
		sortEnv := func(env []corev1.EnvVar) []corev1.EnvVar {
			sorted := append([]corev1.EnvVar{}, env...)
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].Name < sorted[j].Name
			})
			return sorted
		}
		envA := sortEnv(a[i].Env)
		envB := sortEnv(b[i].Env)
		if !equality.Semantic.DeepEqual(envA, envB) {
			logger.V(1).Info("container env mismatch", "container", a[i].Name, "existingCount", len(envA), "desiredCount", len(envB))
			for _, ea := range envA {
				found := false
				for _, eb := range envB {
					if ea.Name == eb.Name {
						if ea.Value != eb.Value {
							logger.V(1).Info("env value mismatch", "key", ea.Name, "existing", ea.Value, "desired", eb.Value)
						}
						found = true
						break
					}
				}
				if !found {
					logger.V(1).Info("env missing in desired", "key", ea.Name)
				}
			}
			for _, eb := range envB {
				found := false
				for _, ea := range envA {
					if ea.Name == eb.Name {
						found = true
						break
					}
				}
				if !found {
					logger.V(1).Info("env missing in existing", "key", eb.Name)
				}
			}
			return false
		}
		if !r.volumesEqual(a[i].VolumeMounts, b[i].VolumeMounts) {
			logger.V(1).Info("container volume mounts mismatch", "container", a[i].Name)
			return false
		}
		if !equality.Semantic.DeepEqual(a[i].Resources, b[i].Resources) {
			logger.V(1).Info("container resources mismatch", "container", a[i].Name)
			return false
		}
	}
	return true
}

func ptrToHostPath(hp corev1.HostPathType) *corev1.HostPathType {
	return &hp
}
