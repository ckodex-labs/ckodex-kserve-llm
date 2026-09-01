/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	appversion "github.com/ckodex-labs/kserve-llm-operator/internal/version"
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
	utilruntime.Must(v1.Install(scheme))
}

func main() {
	options := parseOptions()
	if options.showVersion {
		fmt.Println(appversion.Version)
		return
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&options.zapOptions)))
	setupLog.Info("operator version", "version", appversion.Version)

	mgr := createManager(options)
	cfg := loadOperatorConfig()
	reconciler := buildReconciler(mgr, cfg)
	setupInstrumentation(mgr, cfg, reconciler)
	setupControllers(mgr, cfg, reconciler)
	setupWebhooksAndHealth(mgr, cfg, reconciler)
	addDeprecationChecker(mgr)

	setupLog.Info("starting manager")
	startErr := mgr.Start(ctrl.SetupSignalHandler())
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := reconciler.Audit.Shutdown(shutdownContext); err != nil {
		setupLog.Error(err, "failed to flush audit OTLP exporter during shutdown")
	}
	cancel()
	if startErr != nil {
		setupLog.Error(startErr, "problem running manager")
		os.Exit(1)
	}
}

// deprecationChecker is a manager.Runnable that fires after the cache is synced.
type deprecationChecker struct {
	client client.Client
}

func (d deprecationChecker) Start(ctx context.Context) error {
	var list servingv1alpha2.LLMInferenceServiceList
	if err := d.client.List(ctx, &list); err != nil {
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
