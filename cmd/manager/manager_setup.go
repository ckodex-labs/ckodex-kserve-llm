package main

import (
	"os"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"

	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
)

func createManager(options managerOptions) ctrl.Manager {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: options.metricsAddr,
		},
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port: options.webhookPort,
		}),
		HealthProbeBindAddress: options.probeAddr,
		LeaderElection:         options.enableLeaderElection,
		LeaderElectionID:       "ckodex-kserve-llm.ckodex.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}
	return mgr
}

func loadOperatorConfig() operatorconfig.OperatorConfig {
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.LoadFromEnv()
	applyComplianceProfiles(&cfg)
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
	return cfg
}

func applyComplianceProfiles(cfg *operatorconfig.OperatorConfig) {
	raw := os.Getenv("CKODEX_COMPLIANCE_PROFILES")
	if raw == "" {
		return
	}
	var profiles []servingv1alpha2.ComplianceProfile
	for _, profile := range strings.Split(raw, ",") {
		profiles = append(profiles, servingv1alpha2.ComplianceProfile(strings.TrimSpace(profile)))
	}
	observability.ApplyComplianceDefaults(profiles, cfg)
	if err := observability.EnforceComplianceProfiles(profiles, cfg); err != nil {
		setupLog.Error(err, "compliance profile validation failed — operator cannot start")
		os.Exit(1)
	}
	setupLog.Info("compliance profiles active", "profiles", raw)
}
