package main

import (
	"net/http"
	"os"

	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller"

	ctrl "sigs.k8s.io/controller-runtime"
)

func setupControllers(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig, reconciler *controller.LLMInferenceServiceReconciler) {
	mustSetupController("LLMInferenceService", reconciler.SetupWithManager(mgr))
	setupAdapterController(mgr, reconciler)
	setupEvaluationController(mgr, reconciler)
	setupTenantQuotaController(mgr)
	setupLocalModelCacheController(mgr)
	setupExperimentalControllers(mgr, cfg)
	setupModelOnboardingController(mgr, cfg)
	setupSpecializedControllers(mgr, cfg)
	setupImagePullSecretController(mgr)
}

func setupAdapterController(mgr ctrl.Manager, reconciler *controller.LLMInferenceServiceReconciler) {
	instance := &controller.LLMLoraAdapterReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), HTTPClient: &http.Client{}, Recorder: mgr.GetEventRecorderFor("llmloraadapter-controller"), Audit: reconciler.Audit}
	mustSetupController("LLMLoraAdapter", instance.SetupWithManager(mgr))
}

func setupEvaluationController(mgr ctrl.Manager, reconciler *controller.LLMInferenceServiceReconciler) {
	instance := &controller.LLMEvaluationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), AuditLogger: reconciler.Audit}
	mustSetupController("LLMEvaluation", instance.SetupWithManager(mgr))
}

func setupTenantQuotaController(mgr ctrl.Manager) {
	instance := &controller.TenantQuotaReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Defaults: controller.DefaultTenantQuota()}
	mustSetupController("TenantQuota", instance.SetupWithManager(mgr))
}

func setupLocalModelCacheController(mgr ctrl.Manager) {
	instance := &controller.LocalModelCacheReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Recorder: mgr.GetEventRecorderFor("localmodelcache-controller")}
	mustSetupController("LocalModelCache", instance.SetupWithManager(mgr))
}

func setupExperimentalControllers(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig) {
	if !cfg.Features.EnableExperimentalAgents {
		return
	}
	agent := &controller.AgentReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	mustSetupController("Agent", agent.SetupWithManager(mgr))
	skillRegistry := &controller.SkillRegistryReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	mustSetupController("SkillRegistry", skillRegistry.SetupWithManager(mgr))
	setupLog.Info("experimental Agent and SkillRegistry controllers enabled")
}

func setupModelOnboardingController(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig) {
	instance := &controller.ModelOnboardingReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	configurePromotionMetrics(instance, cfg)
	mustSetupController("ModelOnboarding", instance.SetupWithManager(mgr))
}

func configurePromotionMetrics(instance *controller.ModelOnboardingReconciler, cfg operatorconfig.OperatorConfig) {
	if cfg.PrometheusURL != "" {
		instance.Metrics = controller.NewPrometheusMetricsQuerier(cfg.PrometheusURL)
		setupLog.Info("Prometheus gate metrics enabled", "url", cfg.PrometheusURL)
		return
	}
	if cfg.AllowInsecurePromotionGates {
		instance.Metrics = controller.NewInsecurePassMetricsQuerier()
		setupLog.Info("promotion gates running in insecure compatibility mode", "env", "CKODEX_ALLOW_INSECURE_PROMOTION_GATES")
		return
	}
	setupLog.Info("promotion gates will fail closed until Prometheus metrics are configured")
}

func setupSpecializedControllers(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig) {
	asr := &controller.ASRInferenceServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Recorder: mgr.GetEventRecorderFor("asrinferenceservice-controller")}
	mustSetupController("ASRInferenceService", asr.SetupWithManager(mgr))
	embedding := &controller.EmbeddingInferenceServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	mustSetupController("EmbeddingInferenceService", embedding.SetupWithManager(mgr))
	aipack := &controller.AIPackReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	mustSetupController("AIPack", aipack.SetupWithManager(mgr))
	multimodal := &controller.MultimodalInferenceServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	mustSetupController("MultimodalInferenceService", multimodal.SetupWithManager(mgr))
	reranker := &controller.RerankerInferenceServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), AirGappedMode: cfg.AirGappedMode, LocalRegistry: cfg.LocalRegistry}
	mustSetupController("RerankerInferenceService", reranker.SetupWithManager(mgr))
}

func setupImagePullSecretController(mgr ctrl.Manager) {
	operatorNamespace := os.Getenv("POD_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = "ckodex-system"
	}
	instance := &controller.ImagePullSecretReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), OperatorNamespace: operatorNamespace}
	mustSetupController("ImagePullSecret", instance.SetupWithManager(mgr))
}

func mustSetupController(name string, err error) {
	if err == nil {
		return
	}
	setupLog.Error(err, "unable to create controller", "controller", name)
	os.Exit(1)
}
