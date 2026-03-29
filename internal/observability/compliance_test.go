/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// compliantHIPAAConfig returns a config that satisfies all HIPAA constraints.
func compliantHIPAAConfig() operatorconfig.OperatorConfig {
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.Features.EnableAuth = true
	cfg.Features.EnableLocalModelCache = false
	cfg.AuditSink.RetentionDays = 2555
	cfg.AuditSink.PIIRedaction = true
	return cfg
}

// compliantSOC2Config returns a config that satisfies all SOC2 constraints.
func compliantSOC2Config() operatorconfig.OperatorConfig {
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.Features.EnableSecurity = true
	cfg.AuditSink.PIIRedaction = true
	cfg.AuditSink.Type = "file"
	cfg.AuditSink.RetentionDays = 365
	return cfg
}

// compliantFedRAMPConfig returns a config that satisfies FedRAMP constraints.
func compliantFedRAMPConfig() operatorconfig.OperatorConfig {
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.Features.EnableAuth = true
	cfg.Features.EnableSecurity = true
	cfg.AuditSink.PIIRedaction = true
	return cfg
}

// ---- EnforceComplianceProfiles -----------------------------------------------

func TestEnforceComplianceProfiles_NoProfiles_NoError(t *testing.T) {
	cfg := operatorconfig.DefaultOperatorConfig()
	err := observability.EnforceComplianceProfiles(nil, &cfg)
	assert.NoError(t, err)
}

func TestEnforceComplianceProfiles_HIPAA_Compliant(t *testing.T) {
	cfg := compliantHIPAAConfig()
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceHIPAA}, &cfg)
	assert.NoError(t, err)
}

func TestEnforceComplianceProfiles_HIPAA_AuthMissing(t *testing.T) {
	cfg := compliantHIPAAConfig()
	cfg.Features.EnableAuth = false
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceHIPAA}, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableAuth")
}

func TestEnforceComplianceProfiles_HIPAA_LocalCacheEnabled(t *testing.T) {
	cfg := compliantHIPAAConfig()
	cfg.Features.EnableLocalModelCache = true
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceHIPAA}, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableLocalModelCache")
}

func TestEnforceComplianceProfiles_HIPAA_InsufficientRetention(t *testing.T) {
	cfg := compliantHIPAAConfig()
	cfg.AuditSink.RetentionDays = 365 // only 1 year — HIPAA requires 7
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceHIPAA}, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RetentionDays")
}

func TestEnforceComplianceProfiles_HIPAA_RetentionZeroMeansForever(t *testing.T) {
	cfg := compliantHIPAAConfig()
	cfg.AuditSink.RetentionDays = 0 // 0 = keep forever — satisfies HIPAA
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceHIPAA}, &cfg)
	assert.NoError(t, err)
}

func TestEnforceComplianceProfiles_SOC2_Compliant(t *testing.T) {
	cfg := compliantSOC2Config()
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceSOC2}, &cfg)
	assert.NoError(t, err)
}

func TestEnforceComplianceProfiles_SOC2_SecurityDisabled(t *testing.T) {
	cfg := compliantSOC2Config()
	cfg.Features.EnableSecurity = false
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceSOC2}, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableSecurity")
}

func TestEnforceComplianceProfiles_SOC2_StdoutAuditSink(t *testing.T) {
	cfg := compliantSOC2Config()
	cfg.AuditSink.Type = "stdout" // not durable
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceSOC2}, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AuditSink")
}

func TestEnforceComplianceProfiles_SOC2_PIIRedactionOff(t *testing.T) {
	cfg := compliantSOC2Config()
	cfg.AuditSink.PIIRedaction = false
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceSOC2}, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PIIRedaction")
}

func TestEnforceComplianceProfiles_FedRAMP_Compliant(t *testing.T) {
	cfg := compliantFedRAMPConfig()
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceFedRAMP}, &cfg)
	assert.NoError(t, err)
}

func TestEnforceComplianceProfiles_FedRAMP_AuthMissing(t *testing.T) {
	cfg := compliantFedRAMPConfig()
	cfg.Features.EnableAuth = false
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceFedRAMP}, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableAuth")
}

func TestEnforceComplianceProfiles_FedRAMP_SecurityMissing(t *testing.T) {
	cfg := compliantFedRAMPConfig()
	cfg.Features.EnableSecurity = false
	err := observability.EnforceComplianceProfiles(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceFedRAMP}, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableSecurity")
}

func TestEnforceComplianceProfiles_MultipleProfiles_AllViolationsReported(t *testing.T) {
	// Deliberately break both HIPAA and SOC2
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.Features.EnableAuth = false     // breaks HIPAA + FedRAMP
	cfg.Features.EnableSecurity = false // breaks SOC2 + FedRAMP

	err := observability.EnforceComplianceProfiles([]servingv1alpha2.ComplianceProfile{
		servingv1alpha2.ComplianceHIPAA,
		servingv1alpha2.ComplianceSOC2,
	}, &cfg)
	require.Error(t, err)
	// Both HIPAA and SOC2 violations present in a single error
	assert.Contains(t, err.Error(), "hipaa")
	assert.Contains(t, err.Error(), "soc2")
}

// ---- ApplyComplianceDefaults -------------------------------------------------

func TestApplyComplianceDefaults_HIPAA_SetsRequiredFlags(t *testing.T) {
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.Features.EnableAuth = false
	cfg.Features.EnableLocalModelCache = true
	cfg.AuditSink.RetentionDays = 0

	observability.ApplyComplianceDefaults(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceHIPAA}, &cfg)

	assert.True(t, cfg.Features.EnableAuth)
	assert.False(t, cfg.Features.EnableLocalModelCache)
	assert.True(t, cfg.AuditSink.PIIRedaction)
	assert.Equal(t, 2555, cfg.AuditSink.RetentionDays)
}

func TestApplyComplianceDefaults_HIPAA_DoesNotReduceRetention(t *testing.T) {
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.AuditSink.RetentionDays = 9999 // already exceeds 7 years

	observability.ApplyComplianceDefaults(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceHIPAA}, &cfg)

	assert.Equal(t, 9999, cfg.AuditSink.RetentionDays, "higher retention must not be reduced")
}

func TestApplyComplianceDefaults_SOC2_SetsRequiredFlags(t *testing.T) {
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.Features.EnableSecurity = false
	cfg.AuditSink.Type = "stdout"
	cfg.AuditSink.PIIRedaction = false
	cfg.AuditSink.RetentionDays = 0

	observability.ApplyComplianceDefaults(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceSOC2}, &cfg)

	assert.True(t, cfg.Features.EnableSecurity)
	assert.True(t, cfg.AuditSink.PIIRedaction)
	assert.Equal(t, "file", cfg.AuditSink.Type)
	assert.Equal(t, 365, cfg.AuditSink.RetentionDays)
}

func TestApplyComplianceDefaults_SOC2_PreservesNonStdoutAuditType(t *testing.T) {
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.AuditSink.Type = "otlp-log"

	observability.ApplyComplianceDefaults(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceSOC2}, &cfg)

	assert.Equal(t, "otlp-log", cfg.AuditSink.Type, "non-stdout sink must not be overwritten")
}

func TestApplyComplianceDefaults_FedRAMP_SetsRequiredFlags(t *testing.T) {
	cfg := operatorconfig.DefaultOperatorConfig()
	cfg.Features.EnableAuth = false
	cfg.Features.EnableSecurity = false

	observability.ApplyComplianceDefaults(
		[]servingv1alpha2.ComplianceProfile{servingv1alpha2.ComplianceFedRAMP}, &cfg)

	assert.True(t, cfg.Features.EnableAuth)
	assert.True(t, cfg.Features.EnableSecurity)
	assert.True(t, cfg.AuditSink.PIIRedaction)
	// FedRAMP narrows the registry allowlist
	assert.NotEmpty(t, cfg.Security.AllowedRegistries)
}

func TestApplyComplianceDefaults_ThenEnforce_PassesForAllProfiles(t *testing.T) {
	// ApplyComplianceDefaults followed by EnforceComplianceProfiles should always pass.
	profiles := [][]servingv1alpha2.ComplianceProfile{
		{servingv1alpha2.ComplianceHIPAA},
		{servingv1alpha2.ComplianceSOC2},
		{servingv1alpha2.ComplianceFedRAMP},
		{servingv1alpha2.ComplianceHIPAA, servingv1alpha2.ComplianceSOC2},
	}
	for _, pp := range profiles {
		t.Run("", func(t *testing.T) {
			cfg := operatorconfig.DefaultOperatorConfig()
			observability.ApplyComplianceDefaults(pp, &cfg)
			err := observability.EnforceComplianceProfiles(pp, &cfg)
			assert.NoError(t, err, "apply then enforce must always pass for profiles %v", pp)
		})
	}
}
