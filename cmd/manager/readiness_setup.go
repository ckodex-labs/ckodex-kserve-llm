package main

import (
	"os"

	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller"
	"github.com/ckodex-labs/kserve-llm-operator/internal/health"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/ckodex-labs/kserve-llm-operator/internal/webhook"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

func setupWebhooksAndHealth(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig, reconciler *controller.LLMInferenceServiceReconciler) {
	setupWebhooks(mgr, cfg)
	mustAddHealthCheck("healthz", mgr.AddHealthzCheck("healthz", healthz.Ping), "unable to set up health check")
	mustAddReadyCheck("readyz", mgr.AddReadyzCheck("readyz", healthz.Ping), "unable to set up ready check")
	if reconciler != nil {
		mustAddReadyCheck("evidence", registerEvidenceReadiness(mgr, reconciler.Audit), "unable to register evidence readiness check")
	}
	setupSubsystemReadiness(mgr, cfg)
}

type readinessRegistrar interface {
	AddReadyzCheck(string, healthz.Checker) error
}

func registerEvidenceReadiness(registrar readinessRegistrar, audit *observability.AuditLogger) error {
	if audit == nil {
		return nil
	}
	return registrar.AddReadyzCheck("evidence", audit.EvidenceHealthCheck)
}

func setupWebhooks(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig) {
	if !cfg.Features.EnableWebhooks {
		return
	}
	err := webhook.SetupWebhooks(mgr, webhook.WebhookConfig{HFMirrorURL: cfg.HuggingFaceMirrorURL, FedRAMPMode: cfg.Security.FedRAMPMode})
	if err != nil {
		setupLog.Error(err, "unable to set up webhooks")
		os.Exit(1)
	}
	setupLog.Info("webhooks enabled", "fedRAMPMode", cfg.Security.FedRAMPMode)
}

func setupSubsystemReadiness(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig) {
	if !cfg.Features.EnableSecurity {
		return
	}
	mustAddReadyCheck("vault", mgr.AddReadyzCheck("vault", health.NewVaultHealthCheck().Check), "unable to register vault readiness check")
	mustAddReadyCheck("gatekeeper", mgr.AddReadyzCheck("gatekeeper", health.NewGatekeeperHealthCheck().Check), "unable to register gatekeeper readiness check")
	mustAddReadyCheck("spire", mgr.AddReadyzCheck("spire", health.NewSPIREHealthCheck().Check), "unable to register spire readiness check")
	setupLog.Info("subsystem readiness gates registered", "checks", "vault,gatekeeper,spire")
}

func mustAddHealthCheck(name string, err error, message string) {
	if err == nil {
		return
	}
	setupLog.Error(err, message)
	os.Exit(1)
}

func mustAddReadyCheck(name string, err error, message string) {
	if err == nil {
		return
	}
	setupLog.Error(err, message)
	os.Exit(1)
}

func addDeprecationChecker(mgr ctrl.Manager) {
	if err := mgr.Add(deprecationChecker{client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "unable to add deprecation checker runnable")
	}
}
