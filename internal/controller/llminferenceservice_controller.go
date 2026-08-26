/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ckodex-labs/kserve-llm-operator/internal/auth"
	"github.com/ckodex-labs/kserve-llm-operator/internal/autoscaler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/cleanup"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/evidence"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/status"
	"github.com/ckodex-labs/kserve-llm-operator/internal/gateway"
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
	OPA                               *security.OPAReconciler
	OPAConfig                         security.OPAConfig
	NetworkPolicy                     *security.NetworkPolicyReconciler
	ExternalSecret                    *security.ExternalSecretReconciler
	Vault                             *security.VaultReconciler
	SPIRE                             *security.SPIREReconciler
	SPIRERegistration                 *security.SPIRERegistrationReconciler
	Ebpf                              *security.EbpfReconciler
	ToolSurface                       *security.ToolSurfaceReconciler
	Audit                             *observability.AuditLogger
	Inst                              *observability.Instrumentation
	AuthMiddleware                    *auth.Middleware
	BudgetEnforcer                    *auth.TokenBudgetEnforcer
	Recorder                          record.EventRecorder
	APIReader                         client.Reader
	EnableGRPC                        bool
	EnableHardwareSelection           bool
	EnableExperimentalStatusHardening bool
	OTEL_Endpoint                     string

	AirGappedMode          bool
	LocalRegistry          string
	LocalCosignKeyPath     string
	RuntimeImage           string
	HFInitializerImage     string
	HFMirrorURL            string
	KServeMultiNodeRuntime string

	DeploymentBuilder    *deployment.Builder
	StatusReconciler     *status.Reconciler
	CleanupReconciler    *cleanup.Reconciler
	ServiceReconciler    *reconciler.ServiceReconciler
	PDBReconciler        *reconciler.PDBReconciler
	GovernanceReconciler *evidence.GovernanceReconciler
	HardwareCache        deployment.HardwareCache
	HFCSI                *HFCSIReconciler
	KServeMultiNode      *kserveintegration.Reconciler

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
