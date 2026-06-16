/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/auth"
	"github.com/ckodex-labs/kserve-llm-operator/internal/autoscaler"
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller"
	"github.com/ckodex-labs/kserve-llm-operator/internal/gateway"
	"github.com/ckodex-labs/kserve-llm-operator/internal/health"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/ckodex-labs/kserve-llm-operator/internal/security"
	appversion "github.com/ckodex-labs/kserve-llm-operator/internal/version"
	"github.com/ckodex-labs/kserve-llm-operator/internal/webhook"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	// v1 must be registered before v1alpha2 so it is recognised as the storage version.
	utilruntime.Must(servingv1.AddToScheme(scheme))
	utilruntime.Must(servingv1alpha2.AddToScheme(scheme))
	utilruntime.Must(gwapiv1.Install(scheme))
}

func main() {
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
		webhookPort          int
		developmentMode      bool
		showVersion          bool
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8091", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8087", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "Webhook server port.")
	flag.BoolVar(&showVersion, "version", false, "Print the operator version and exit.")

	flag.BoolVar(&developmentMode, "development", false, "Enable development-mode logging (console output, DPanic, no sampling). Production defaults to JSON.")

	opts := zap.Options{Development: developmentMode}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	if showVersion {
		fmt.Println(appversion.Version)
		return
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog.Info("operator version", "version", appversion.Version)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port: webhookPort,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "ckodex-kserve-llm.ckodex.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Load operator config with feature gates
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.LoadFromEnv()

	// Compliance profiles — CKODEX_COMPLIANCE_PROFILES=hipaa,soc2,fedramp (comma-separated).
	// ApplyComplianceDefaults auto-corrects flags before validation so operators don't
	// need to set every individual env var when activating a profile.
	if raw := os.Getenv("CKODEX_COMPLIANCE_PROFILES"); raw != "" {
		var profiles []servingv1alpha2.ComplianceProfile
		for _, s := range strings.Split(raw, ",") {
			profiles = append(profiles, servingv1alpha2.ComplianceProfile(strings.TrimSpace(s)))
		}
		observability.ApplyComplianceDefaults(profiles, &cfg)
		if err := observability.EnforceComplianceProfiles(profiles, &cfg); err != nil {
			setupLog.Error(err, "compliance profile validation failed — operator cannot start")
			os.Exit(1)
		}
		setupLog.Info("compliance profiles active", "profiles", raw)
	}

	if err := operatorconfig.ValidateStorageCredentials(&cfg); err != nil {
		setupLog.Error(err, "startup credential validation failed")
		os.Exit(1)
	}

	setupLog.Info("operator config loaded",
		"enableScheduler", cfg.Features.EnableScheduler,
		"enableGateway", cfg.Features.EnableGateway,
		"enableAutoscaler", cfg.Features.EnableAutoscaler,
		"enableSecurity", cfg.Features.EnableSecurity,
		"enableOTelPipeline", cfg.Features.EnableOTelPipeline,
		"enableExperimentalAgents", cfg.Features.EnableExperimentalAgents,
		"auditSinkType", cfg.AuditSink.Type,
		"piiRedaction", cfg.AuditSink.PIIRedaction,
	)

	// Build reconciler with feature-gated sub-reconcilers
	reconciler := &controller.LLMInferenceServiceReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		Recorder:           mgr.GetEventRecorderFor("llminferenceservice-controller"),
		OTEL_Endpoint:      cfg.Observability.OTLPEndpoint,
		AirGappedMode:      cfg.AirGappedMode,
		LocalRegistry:      cfg.LocalRegistry,
		LocalCosignKeyPath: cfg.LocalCosignKeyPath,
	}

	// gRPC — independent of gateway (controls Service port definition)
	reconciler.EnableGRPC = cfg.Features.EnableGRPC
	reconciler.EnableHardwareSelection = cfg.Features.EnableExperimentalHardwareSelection
	reconciler.EnableExperimentalStatusHardening = cfg.Features.EnableExperimentalStatusHardening

	// Gateway
	if cfg.Features.EnableGateway {
		reconciler.Gateway = &gateway.Reconciler{
			Client:     mgr.GetClient(),
			Scheme:     mgr.GetScheme(),
			EnableGRPC: cfg.Features.EnableGRPC,
		}
		setupLog.Info("gateway reconciler enabled", "grpc", cfg.Features.EnableGRPC)
	}

	// Autoscaler
	if cfg.Features.EnableAutoscaler {
		reconciler.Autoscaler = &autoscaler.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		setupLog.Info("autoscaler reconciler enabled")
	}

	// Security: NetworkPolicy + Vault + OPA + eBPF + SPIRE mTLS
	if cfg.Features.EnableSecurity {
		reconciler.NetworkPolicy = &security.NetworkPolicyReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		reconciler.ToolSurface = &security.ToolSurfaceReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		reconciler.ExternalSecret = &security.ExternalSecretReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		reconciler.Vault = &security.VaultReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Config: security.DefaultVaultConfig(),
		}
		reconciler.OPA = &security.OPAReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		reconciler.OPAConfig = security.OPAConfig{
			AllowedRegistries:     cfg.Security.AllowedRegistries,
			MaxGPUsPerNamespace:   cfg.Security.MaxGPUsPerNamespace,
			MaxReplicasPerService: cfg.Security.MaxReplicasPerService,
		}
		reconciler.Ebpf = &security.EbpfReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		// SPIRE — injects SPIFFE sidecar and manages registration entry ConfigMaps.
		// InjectSidecar appends the CSI volume + spiffe-helper container to every
		// vLLM Deployment, providing zero-trust workload identity via mTLS.
		reconciler.SPIRE = &security.SPIREReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			VClusterMode:  cfg.VClusterMode,
			HostNamespace: cfg.HostNamespace,
		}
		reconciler.SPIRERegistration = &security.SPIRERegistrationReconciler{
			Client:          mgr.GetClient(),
			Scheme:          mgr.GetScheme(),
			VClusterMode:    cfg.VClusterMode,
			HostNamespace:   cfg.HostNamespace,
			SpireReconciler: reconciler.SPIRE,
		}
		setupLog.Info("security reconcilers enabled",
			"components", "NetworkPolicy+Vault+OPA+eBPF+SPIRE",
			"allowedRegistries", len(cfg.Security.AllowedRegistries),
		)
	}

	// LeaderWorkerSet — always wired; Reconcile is a no-op when spec.parallelism is nil.
	// LWS CRD availability is checked lazily during reconciliation via unstructured client.
	reconciler.LWS = &controller.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}

	// Auth middleware — enabled when EnableAuth=true.
	// When active, JWT bearer tokens are required on all inference proxy paths.
	// The OIDCConfig is built from OperatorConfig.Auth; the middleware is stored
	// on the reconciler so it can protect any HTTP handlers spun up for inference
	// proxying (e.g. via the webhook server or a future inference-proxy runnable).
	if cfg.Features.EnableAuth {
		if cfg.Auth.IssuerURL == "" {
			setupLog.Error(nil, "EnableAuth=true but CKODEX_AUTH_ISSUER_URL is not set; auth cannot be initialised")
			os.Exit(1)
		}
		oidcCfg := auth.OIDCConfig{
			IssuerURL:      cfg.Auth.IssuerURL,
			Audience:       cfg.Auth.Audience,
			RequiredScopes: cfg.Auth.RequiredScopes,
			CacheTTL:       cfg.Auth.JWKSCacheTTL,
		}
		reconciler.AuthMiddleware = auth.NewMiddleware(oidcCfg)
		reconciler.BudgetEnforcer = auth.NewTokenBudgetEnforcer()
		setupLog.Info("auth middleware enabled",
			"issuer", cfg.Auth.IssuerURL,
			"audience", cfg.Auth.Audience,
			"requiredScopes", cfg.Auth.RequiredScopes,
		)
	}

	// OTel instrumentation — creates counters/histograms for the operator.
	// Wired into the reconciler (empty_high_dal detection) and auth middleware
	// (anti_execute detection) to emit CKODEX §10 forbidden-tuple metrics.
	inst, err := observability.NewInstrumentation()
	if err != nil {
		setupLog.Error(err, "failed to create OTel instrumentation")
		os.Exit(1)
	}
	reconciler.Inst = inst
	if reconciler.AuthMiddleware != nil {
		reconciler.AuthMiddleware.WithInstrumentation(inst)
	}

	// Audit logger — respects PIIRedaction from AuditSinkConfig.
	reconciler.Audit = observability.NewAuditLoggerWithOptions(
		mgr.GetClient(), mgr.GetScheme(), cfg.AuditSink.PIIRedaction,
	)

	// Set up main controller
	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LLMInferenceService")
		os.Exit(1)
	}

	// Set up LoRA Adapter controller
	if err := (&controller.LLMLoraAdapterReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		HTTPClient: &http.Client{},
		Recorder:   mgr.GetEventRecorderFor("llmloraadapter-controller"),
		Audit:      reconciler.Audit,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LLMLoraAdapter")
		os.Exit(1)
	}

	// Set up LLM Evaluation controller
	if err := (&controller.LLMEvaluationReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		AuditLogger: reconciler.Audit,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LLMEvaluation")
		os.Exit(1)
	}

	// Set up TenantQuota controller — always active; no-ops on unlabeled namespaces.
	if err := (&controller.TenantQuotaReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Defaults: controller.DefaultTenantQuota(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TenantQuota")
		os.Exit(1)
	}

	// Set up LocalModelCache controller
	if err := (&controller.LocalModelCacheReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("localmodelcache-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LocalModelCache")
		os.Exit(1)
	}

	// Set up Agent controller — validates model + skill registry bindings.
	// Experimental: gated behind EnableExperimentalAgents feature flag.
	if cfg.Features.EnableExperimentalAgents {
		if err := (&controller.AgentReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Agent")
			os.Exit(1)
		}

		// Set up SkillRegistry controller — validates skill entries and maintains entryCount.
		if err := (&controller.SkillRegistryReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "SkillRegistry")
			os.Exit(1)
		}
		setupLog.Info("experimental Agent and SkillRegistry controllers enabled")
	}

	// Set up ModelOnboarding controller — drives multi-stage model promotion pipeline.
	// Promotion gates fail closed unless CKODEX_PROMETHEUS_URL is configured or the
	// explicit insecure fallback is enabled for development-only environments.
	modelOnboardingReconciler := &controller.ModelOnboardingReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if cfg.PrometheusURL != "" {
		modelOnboardingReconciler.Metrics = controller.NewPrometheusMetricsQuerier(cfg.PrometheusURL)
		setupLog.Info("Prometheus gate metrics enabled", "url", cfg.PrometheusURL)
	} else if cfg.AllowInsecurePromotionGates {
		modelOnboardingReconciler.Metrics = controller.NewInsecurePassMetricsQuerier()
		setupLog.Info("promotion gates running in insecure compatibility mode", "env", "CKODEX_ALLOW_INSECURE_PROMOTION_GATES")
	} else {
		setupLog.Info("promotion gates will fail closed until Prometheus metrics are configured")
	}
	if err := modelOnboardingReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ModelOnboarding")
		os.Exit(1)
	}

	// Set up ASRInferenceService controller — Whisper-family and custom ASR models.
	if err := (&controller.ASRInferenceServiceReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("asrinferenceservice-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ASRInferenceService")
		os.Exit(1)
	}

	// Set up EmbeddingInferenceService controller — sentence-transformers and BERT embedding models.
	if err := (&controller.EmbeddingInferenceServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EmbeddingInferenceService")
		os.Exit(1)
	}

	// Set up AIPack controller — resolves artifact family and manages the Ready condition.
	if err := (&controller.AIPackReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIPack")
		os.Exit(1)
	}

	// Set up MultimodalInferenceService controller — vision-language and image-generation models.
	if err := (&controller.MultimodalInferenceServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MultimodalInferenceService")
		os.Exit(1)
	}

	// Set up RerankerInferenceService controller — cross-encoder reranking with vLLM --task score.
	if err := (&controller.RerankerInferenceServiceReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		AirGappedMode: cfg.AirGappedMode,
		LocalRegistry: cfg.LocalRegistry,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RerankerInferenceService")
		os.Exit(1)
	}

	// Set up ImagePullSecret controller — distributes registry credentials into tenant namespaces.
	// OperatorNamespace is the namespace where source pull secrets reside (where this operator runs).
	// Detected from POD_NAMESPACE (injected by Helm/Downward API); falls back to "ckodex-system".
	operatorNS := os.Getenv("POD_NAMESPACE")
	if operatorNS == "" {
		operatorNS = "ckodex-system"
	}
	if err := (&controller.ImagePullSecretReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		OperatorNamespace: operatorNS,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ImagePullSecret")
		os.Exit(1)
	}

	// Webhooks — enabled when CKODEX_FEATURE_ENABLE_WEBHOOKS=true.
	// TLS certificates are provisioned by cert-manager (see deploy/helm/templates/cert-manager.yaml).
	// The cert-manager cainjector automatically populates the caBundle field on the
	// MutatingWebhookConfiguration and ValidatingWebhookConfiguration resources.
	//
	// For local development without cert-manager, disable with CKODEX_FEATURE_ENABLE_WEBHOOKS=false
	// (the default) and interact with the operator directly via kubectl.
	if cfg.Features.EnableWebhooks {
		webhookCfg := webhook.WebhookConfig{
			HFMirrorURL: cfg.HuggingFaceMirrorURL,
			FedRAMPMode: cfg.Security.FedRAMPMode,
		}
		if err := webhook.SetupWebhooks(mgr, webhookCfg); err != nil {
			setupLog.Error(err, "unable to set up webhooks")
			os.Exit(1)
		}
		setupLog.Info("webhooks enabled", "fedRAMPMode", cfg.Security.FedRAMPMode)
	}

	// Health checks — standard liveness/readiness pings
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Optional subsystem readiness gates.
	// These are registered as readiness (not liveness) checks: the pod stays not-Ready
	// until the dependency is reachable, preventing the operator from reconciling with
	// a broken security or secret backend.
	if cfg.Features.EnableSecurity {
		// healthz.Checker is a function type; pass method values to satisfy it.
		if err := mgr.AddReadyzCheck("vault", health.NewVaultHealthCheck().Check); err != nil {
			setupLog.Error(err, "unable to register vault readiness check")
			os.Exit(1)
		}
		if err := mgr.AddReadyzCheck("gatekeeper", health.NewGatekeeperHealthCheck().Check); err != nil {
			setupLog.Error(err, "unable to register gatekeeper readiness check")
			os.Exit(1)
		}
		if err := mgr.AddReadyzCheck("spire", health.NewSPIREHealthCheck().Check); err != nil {
			setupLog.Error(err, "unable to register spire readiness check")
			os.Exit(1)
		}
		setupLog.Info("subsystem readiness gates registered", "checks", "vault,gatekeeper,spire")
	}

	// Deprecation check: emit a warning at startup if v1alpha2 resources are present.
	// Runs after the cache is synced (inside a Runnable) so the List returns real data.
	// This is informational — it does not block startup or reconciliation.
	if err := mgr.Add(deprecationChecker{client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "unable to add deprecation checker runnable")
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// deprecationChecker is a manager.Runnable that fires after the cache is synced.
// It lists v1alpha2 LLMInferenceService resources and warns operators that they
// should migrate to the stable v1 API before the v1alpha2 deprecation window closes.
type deprecationChecker struct {
	client client.Client
}

func (d deprecationChecker) Start(ctx context.Context) error {
	var list servingv1alpha2.LLMInferenceServiceList
	if err := d.client.List(ctx, &list); err != nil {
		// List failure is non-fatal — cache may not be ready yet or RBAC may be
		// restricted. Log and return nil so the manager is not blocked.
		setupLog.Info("deprecation check skipped: unable to list v1alpha2 resources", "error", err.Error())
		return nil
	}
	if len(list.Items) > 0 {
		setupLog.Info("DEPRECATION WARNING: v1alpha2 LLMInferenceService resources detected",
			"count", len(list.Items),
			"action", "Migrate to serving.ckodex.com/v1 before the v1alpha2 removal window. See docs/api-deprecation-policy.md",
		)
	}
	return nil
}
