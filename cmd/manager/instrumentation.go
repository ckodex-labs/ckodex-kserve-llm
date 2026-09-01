package main

import (
	"os"

	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"

	ctrl "sigs.k8s.io/controller-runtime"
)

func setupInstrumentation(mgr ctrl.Manager, cfg operatorconfig.OperatorConfig, reconciler *controller.LLMInferenceServiceReconciler) {
	inst, err := observability.NewInstrumentation()
	if err != nil {
		setupLog.Error(err, "failed to create OTel instrumentation")
		os.Exit(1)
	}
	reconciler.Inst = inst
	if reconciler.AuthMiddleware != nil {
		reconciler.AuthMiddleware.WithInstrumentation(inst)
	}
	audit, err := observability.NewAuditLoggerWithOptionsAndEndpoint(
		mgr.GetClient(), mgr.GetScheme(), cfg.AuditSink.PIIRedaction, cfg.AuditSink.OTLPEndpoint,
	)
	if err != nil {
		setupLog.Error(err, "failed to configure audit OTLP export")
		os.Exit(1)
	}
	reconciler.Audit = audit
}
