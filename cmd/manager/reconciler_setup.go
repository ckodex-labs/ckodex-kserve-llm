package main

import (
	"os"

	"github.com/ckodex-labs/kserve-llm-operator/internal/auth"
	"github.com/ckodex-labs/kserve-llm-operator/internal/autoscaler"
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller"
	"github.com/ckodex-labs/kserve-llm-operator/internal/gateway"
	"github.com/ckodex-labs/kserve-llm-operator/internal/scheduler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/security"

	ctrl "sigs.k8s.io/controller-runtime"
)

func buildReconciler(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig) *controller.LLMInferenceServiceReconciler {
	reconciler := &controller.LLMInferenceServiceReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		Recorder:               mgr.GetEventRecorderFor("llminferenceservice-controller"),
		OTEL_Endpoint:          cfg.Observability.OTLPEndpoint,
		AirGappedMode:          cfg.AirGappedMode,
		LocalRegistry:          cfg.LocalRegistry,
		LocalCosignKeyPath:     cfg.LocalCosignKeyPath,
		RuntimeImage:           cfg.Defaults.RuntimeImage,
		KServeMultiNodeRuntime: cfg.Defaults.KServeMultiNodeRuntime,
		HFInitializerImage:     cfg.Defaults.HuggingFaceInitializerImage,
		HFMirrorURL:            cfg.HuggingFaceMirrorURL,
	}
	reconciler.EnableGRPC = cfg.Features.EnableGRPC
	reconciler.EnableHardwareSelection = cfg.Features.EnableExperimentalHardwareSelection
	reconciler.EnableExperimentalStatusHardening = cfg.Features.EnableExperimentalStatusHardening
	configureGateway(mgr, cfg, reconciler)
	configureScheduler(mgr, cfg, reconciler)
	configureAutoscaler(mgr, cfg, reconciler)
	configureSecurity(mgr, cfg, reconciler)
	configureAuth(cfg, reconciler)
	return reconciler
}

func configureGateway(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig, reconciler *controller.LLMInferenceServiceReconciler) {
	if !cfg.Features.EnableGateway {
		return
	}
	reconciler.Gateway = &gateway.Reconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), EnableGRPC: cfg.Features.EnableGRPC}
	setupLog.Info("gateway reconciler enabled", "grpc", cfg.Features.EnableGRPC)
}

func configureScheduler(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig, reconciler *controller.LLMInferenceServiceReconciler) {
	if !cfg.Features.EnableScheduler {
		return
	}
	reconciler.Scheduler = &scheduler.Reconciler{
		Config: &scheduler.ConfigReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		EPP:    &scheduler.EPPManager{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Image: cfg.Scheduler.Image},
		Pool:   &scheduler.InferencePoolManager{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
	}
	setupLog.Info("scheduler reconciler enabled")
}

func configureAutoscaler(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig, reconciler *controller.LLMInferenceServiceReconciler) {
	if !cfg.Features.EnableAutoscaler {
		return
	}
	reconciler.Autoscaler = &autoscaler.Reconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	setupLog.Info("autoscaler reconciler enabled")
}

func configureSecurity(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig, reconciler *controller.LLMInferenceServiceReconciler) {
	if !cfg.Features.EnableSecurity {
		return
	}
	reconciler.NetworkPolicy = &security.NetworkPolicyReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), GatewayDataPlaneNamespace: os.Getenv("CKODEX_ENVOY_GATEWAY_NAMESPACE")}
	reconciler.ToolSurface = &security.ToolSurfaceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	reconciler.ExternalSecret = &security.ExternalSecretReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	reconciler.Vault = &security.VaultReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Config: security.DefaultVaultConfig()}
	reconciler.OPA = &security.OPAReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	reconciler.OPAConfig = security.OPAConfig{AllowedRegistries: cfg.Security.AllowedRegistries, MaxGPUsPerNamespace: cfg.Security.MaxGPUsPerNamespace, MaxReplicasPerService: cfg.Security.MaxReplicasPerService}
	reconciler.Ebpf = &security.EbpfReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	configureSPIRE(mgr, cfg, reconciler)
	setupLog.Info("security reconcilers enabled", "components", "NetworkPolicy+Vault+OPA+eBPF+SPIRE", "allowedRegistries", len(cfg.Security.AllowedRegistries))
}

func configureSPIRE(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig, reconciler *controller.LLMInferenceServiceReconciler) {
	reconciler.SPIRE = &security.SPIREReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), VClusterMode: cfg.VClusterMode, HostNamespace: cfg.HostNamespace}
	reconciler.SPIRERegistration = &security.SPIRERegistrationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), VClusterMode: cfg.VClusterMode, HostNamespace: cfg.HostNamespace, SpireReconciler: reconciler.SPIRE}
}

func configureAuth(cfg operatorconfig.OperatorConfig, reconciler *controller.LLMInferenceServiceReconciler) {
	if !cfg.Features.EnableAuth {
		return
	}
	if cfg.Auth.IssuerURL == "" {
		setupLog.Error(nil, "EnableAuth=true but CKODEX_AUTH_ISSUER_URL is not set; auth cannot be initialised")
		os.Exit(1)
	}
	reconciler.AuthMiddleware = auth.NewMiddleware(auth.OIDCConfig{IssuerURL: cfg.Auth.IssuerURL, Audience: cfg.Auth.Audience, RequiredScopes: cfg.Auth.RequiredScopes, CacheTTL: cfg.Auth.JWKSCacheTTL})
	reconciler.BudgetEnforcer = auth.NewTokenBudgetEnforcer()
	setupLog.Info("auth middleware enabled", "issuer", cfg.Auth.IssuerURL, "audience", cfg.Auth.Audience, "requiredScopes", cfg.Auth.RequiredScopes)
}
