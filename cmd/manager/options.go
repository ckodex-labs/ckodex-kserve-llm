package main

import (
	"flag"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type managerOptions struct {
	metricsAddr          string
	probeAddr            string
	enableLeaderElection bool
	webhookPort          int
	developmentMode      bool
	showVersion          bool
	zapOptions           zap.Options
}

func parseOptions() managerOptions {
	var options managerOptions
	flag.StringVar(&options.metricsAddr, "metrics-bind-address", ":8091", "The address the metric endpoint binds to.")
	flag.StringVar(&options.probeAddr, "health-probe-bind-address", ":8087", "The address the probe endpoint binds to.")
	flag.BoolVar(&options.enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.IntVar(&options.webhookPort, "webhook-port", 9443, "Webhook server port.")
	flag.BoolVar(&options.showVersion, "version", false, "Print the operator version and exit.")
	flag.BoolVar(&options.developmentMode, "development", false, "Enable development-mode logging (console output, DPanic, no sampling). Production defaults to JSON.")
	options.zapOptions = zap.Options{Development: options.developmentMode}
	options.zapOptions.BindFlags(flag.CommandLine)
	flag.Parse()
	return options
}
