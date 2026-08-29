/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"testing"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

type capturingReadinessRegistrar struct {
	name    string
	checker healthz.Checker
}

func (r *capturingReadinessRegistrar) AddReadyzCheck(name string, checker healthz.Checker) error {
	r.name = name
	r.checker = checker
	return nil
}

func TestEvidenceReadinessRuntimeToSpec_RegistersAuditHealthChecker(t *testing.T) {
	t.Setenv("CKODEX_AUDIT_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	registrar := &capturingReadinessRegistrar{}
	audit := observability.NewAuditLoggerWithOptions(nil, nil, false)
	if err := registerEvidenceReadiness(registrar, audit); err != nil {
		t.Fatalf("register evidence readiness: %v", err)
	}
	if registrar.name != "evidence" || registrar.checker == nil {
		t.Fatalf("registered name=%q checker=%v", registrar.name, registrar.checker)
	}
	if err := registrar.checker(nil); err != nil {
		t.Fatalf("initial evidence readiness: %v", err)
	}
}

func TestEvidenceReadinessSpecToRuntime_SkipsAbsentAuditLogger(t *testing.T) {
	registrar := &capturingReadinessRegistrar{}
	if err := registerEvidenceReadiness(registrar, nil); err != nil {
		t.Fatalf("register nil evidence readiness: %v", err)
	}
	if registrar.name != "" || registrar.checker != nil {
		t.Fatalf("nil audit logger registered name=%q checker=%v", registrar.name, registrar.checker)
	}
}
