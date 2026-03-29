/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"fmt"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
)

// ComplianceViolation describes a single constraint that is not satisfied.
type ComplianceViolation struct {
	Profile     servingv1alpha2.ComplianceProfile
	Constraint  string
	Remediation string
}

func (v ComplianceViolation) Error() string {
	return fmt.Sprintf("[%s] %s — remediation: %s", v.Profile, v.Constraint, v.Remediation)
}

// EnforceComplianceProfiles validates that the OperatorConfig satisfies all
// constraints implied by the given compliance profiles. Call this at operator
// startup so misconfigurations are caught before any workload is scheduled.
//
// Returns a non-nil error listing every violated constraint (not just the first).
func EnforceComplianceProfiles(
	profiles []servingv1alpha2.ComplianceProfile,
	cfg *operatorconfig.OperatorConfig,
) error {
	var violations []string

	for _, p := range profiles {
		switch p {
		case servingv1alpha2.ComplianceHIPAA:
			violations = append(violations, hipaaViolations(cfg)...)
		case servingv1alpha2.ComplianceSOC2:
			violations = append(violations, soc2Violations(cfg)...)
		case servingv1alpha2.ComplianceFedRAMP:
			violations = append(violations, fedRampViolations(cfg)...)
		}
	}

	if len(violations) == 0 {
		return nil
	}
	return fmt.Errorf("compliance profile violations:\n  %s", strings.Join(violations, "\n  "))
}

// ApplyComplianceDefaults mutates cfg to satisfy the given compliance profiles.
// It is called before EnforceComplianceProfiles so that auto-correctable settings
// are applied without operator intervention.
func ApplyComplianceDefaults(
	profiles []servingv1alpha2.ComplianceProfile,
	cfg *operatorconfig.OperatorConfig,
) {
	for _, p := range profiles {
		switch p {
		case servingv1alpha2.ComplianceHIPAA:
			// HIPAA: auth required, model caching disabled, 7-year audit retention.
			cfg.Features.EnableAuth = true
			cfg.Features.EnableLocalModelCache = false
			if cfg.AuditSink.RetentionDays < 2555 {
				cfg.AuditSink.RetentionDays = 2555
			}
			cfg.AuditSink.PIIRedaction = true

		case servingv1alpha2.ComplianceSOC2:
			// SOC2: security gate active, durable audit sink, PII scrubbing on.
			cfg.Features.EnableSecurity = true
			cfg.AuditSink.PIIRedaction = true
			if cfg.AuditSink.Type == "stdout" {
				cfg.AuditSink.Type = "file"
			}
			if cfg.AuditSink.RetentionDays < 365 {
				cfg.AuditSink.RetentionDays = 365
			}

		case servingv1alpha2.ComplianceFedRAMP:
			// FedRAMP: auth + security both required, restrict to approved registries.
			cfg.Features.EnableAuth = true
			cfg.Features.EnableSecurity = true
			cfg.AuditSink.PIIRedaction = true
			// Replace allowedRegistries with FedRAMP-authorized set only.
			cfg.Security.AllowedRegistries = fedRampApprovedRegistries()
		}
	}
}

func hipaaViolations(cfg *operatorconfig.OperatorConfig) []string {
	var v []string
	if !cfg.Features.EnableAuth {
		v = append(v, "[hipaa] EnableAuth must be true — set CKODEX_FEATURE_ENABLE_AUTH=true")
	}
	if cfg.Features.EnableLocalModelCache {
		v = append(v, "[hipaa] EnableLocalModelCache must be false — PHI must not be cached on disk")
	}
	if cfg.AuditSink.RetentionDays > 0 && cfg.AuditSink.RetentionDays < 2555 {
		v = append(v, "[hipaa] AuditSink.RetentionDays must be >= 2555 (7 years) — currently %d"+
			fmt.Sprintf(" (%d)", cfg.AuditSink.RetentionDays))
	}
	return v
}

func soc2Violations(cfg *operatorconfig.OperatorConfig) []string {
	var v []string
	if !cfg.Features.EnableSecurity {
		v = append(v, "[soc2] EnableSecurity must be true — set CKODEX_FEATURE_ENABLE_SECURITY=true")
	}
	if cfg.AuditSink.Type == "stdout" {
		v = append(v, "[soc2] AuditSink.Type must not be 'stdout' — use 'file' or 'otlp-log' for durable audit trail")
	}
	if !cfg.AuditSink.PIIRedaction {
		v = append(v, "[soc2] AuditSink.PIIRedaction must be true")
	}
	return v
}

func fedRampViolations(cfg *operatorconfig.OperatorConfig) []string {
	var v []string
	if !cfg.Features.EnableAuth {
		v = append(v, "[fedramp] EnableAuth must be true")
	}
	if !cfg.Features.EnableSecurity {
		v = append(v, "[fedramp] EnableSecurity must be true")
	}
	return v
}

// fedRampApprovedRegistries returns the registry prefix allowlist for FedRAMP High.
// Only FIPS-validated, FedRAMP-authorized image sources are permitted.
func fedRampApprovedRegistries() []string {
	return []string{
		"ghcr.io/ckodex/",
		"gcr.io/distroless/",
		// Add your FedRAMP-authorized mirror here, e.g.:
		// "registry.internal.agency.gov/",
	}
}
