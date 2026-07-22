/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckodex-labs/kserve-llm-operator/internal/config"
)

// ---- DefaultOperatorConfig ---------------------------------------------------

func TestDefaultOperatorConfig_FeatureGates(t *testing.T) {
	cfg := config.DefaultOperatorConfig()
	// Core subsystems on by default
	assert.True(t, cfg.Features.EnableScheduler)
	assert.True(t, cfg.Features.EnableGateway)
	assert.True(t, cfg.Features.EnableAutoscaler)
	assert.True(t, cfg.Features.EnableOTelPipeline)
	// Optional / risky subsystems off by default
	assert.False(t, cfg.Features.EnableSecurity)
	assert.False(t, cfg.Features.EnableChaos)
	assert.False(t, cfg.Features.EnableDapr)
	assert.False(t, cfg.Features.EnableLocalModelCache)
	assert.False(t, cfg.Features.EnableAuth)
	assert.False(t, cfg.Features.EnableSessions)
	assert.False(t, cfg.Features.EnableGRPC)
	assert.False(t, cfg.Features.EnableWebhooks)
	assert.False(t, cfg.Features.EnableExperimentalAgents)
}

func TestDefaultOperatorConfig_AuditSink(t *testing.T) {
	cfg := config.DefaultOperatorConfig()
	assert.Equal(t, "stdout", cfg.AuditSink.Type)
	assert.True(t, cfg.AuditSink.PIIRedaction, "PII redaction must be on by default")
	assert.Equal(t, 0, cfg.AuditSink.RetentionDays, "0 = keep forever")
}

func TestDefaultOperatorConfig_SemanticCache(t *testing.T) {
	cfg := config.DefaultOperatorConfig()
	assert.Empty(t, cfg.SemanticCacheAddr, "default addr should be empty (in-memory fallback)")
	assert.Equal(t, 1*time.Hour, cfg.SemanticCacheTTL)
}

func TestDefaultOperatorConfig_SecurityDefaults(t *testing.T) {
	cfg := config.DefaultOperatorConfig()
	assert.NotEmpty(t, cfg.Security.AllowedRegistries)
	assert.Greater(t, cfg.Security.MaxGPUsPerNamespace, int64(0))
	assert.Greater(t, cfg.Security.MaxReplicasPerService, int64(0))
	assert.False(t, cfg.Security.FedRAMPMode)
}

func TestDefaultOperatorConfig_ObservabilityDefaults(t *testing.T) {
	cfg := config.DefaultOperatorConfig()
	assert.NotEmpty(t, cfg.Observability.OTLPEndpoint)
	assert.Greater(t, cfg.Observability.SamplingRate, float64(0))
	assert.NotEmpty(t, cfg.Observability.ServiceName)
}

func TestDefaultOperatorConfig_PrometheusURLEmpty(t *testing.T) {
	cfg := config.DefaultOperatorConfig()
	assert.Empty(t, cfg.PrometheusURL, "no Prometheus URL by default")
	assert.False(t, cfg.AllowInsecurePromotionGates, "promotion gates should fail closed by default")
}

// ---- LoadFromEnv -------------------------------------------------------------

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func TestLoadFromEnv_FeatureGateOverrides(t *testing.T) {
	setEnv(t, "CKODEX_FEATURE_ENABLE_SECURITY", "true")
	setEnv(t, "CKODEX_FEATURE_ENABLE_AUTH", "true")
	setEnv(t, "CKODEX_FEATURE_ENABLE_GRPC", "true")
	setEnv(t, "CKODEX_FEATURE_ENABLE_WEBHOOKS", "true")
	setEnv(t, "CKODEX_FEATURE_ENABLE_SCHEDULER", "false")

	cfg := config.DefaultOperatorConfig()
	cfg.LoadFromEnv()

	assert.True(t, cfg.Features.EnableSecurity)
	assert.True(t, cfg.Features.EnableAuth)
	assert.True(t, cfg.Features.EnableGRPC)
	assert.True(t, cfg.Features.EnableWebhooks)
	assert.False(t, cfg.Features.EnableScheduler)
}

func TestLoadFromEnv_StringFields(t *testing.T) {
	setEnv(t, "CKODEX_HF_MIRROR_URL", "https://hf-mirror.corp.internal")
	setEnv(t, "CKODEX_OTEL_ENDPOINT", "otel-collector.monitoring:4317")
	setEnv(t, "CKODEX_OTEL_SERVICE_NAME", "my-operator")
	setEnv(t, "CKODEX_AUTH_ISSUER_URL", "https://idp.corp.internal")
	setEnv(t, "CKODEX_AUTH_AUDIENCE", "inference-service")
	setEnv(t, "CKODEX_PROMETHEUS_URL", "http://prometheus.monitoring:9090")
	setEnv(t, "CKODEX_HUGGING_FACE_INITIALIZER_IMAGE", "registry.corp/hf-initializer@sha256:1234")

	cfg := config.DefaultOperatorConfig()
	cfg.LoadFromEnv()

	assert.Equal(t, "https://hf-mirror.corp.internal", cfg.HuggingFaceMirrorURL)
	assert.Equal(t, "otel-collector.monitoring:4317", cfg.Observability.OTLPEndpoint)
	assert.Equal(t, "my-operator", cfg.Observability.ServiceName)
	assert.Equal(t, "https://idp.corp.internal", cfg.Auth.IssuerURL)
	assert.Equal(t, "inference-service", cfg.Auth.Audience)
	assert.Equal(t, "http://prometheus.monitoring:9090", cfg.PrometheusURL)
	assert.Equal(t, "registry.corp/hf-initializer@sha256:1234", cfg.Defaults.HuggingFaceInitializerImage)
}

func TestLoadFromEnv_AllowInsecurePromotionGates(t *testing.T) {
	setEnv(t, "CKODEX_ALLOW_INSECURE_PROMOTION_GATES", "true")

	cfg := config.DefaultOperatorConfig()
	cfg.LoadFromEnv()

	assert.True(t, cfg.AllowInsecurePromotionGates)
}

func TestLoadFromEnv_ContractOverrides(t *testing.T) {
	setEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT", "http://contract-otel:4318")
	setEnv(t, "VERSION", "v1.2.3-contract")

	cfg := config.DefaultOperatorConfig()
	cfg.LoadFromEnv()

	assert.Equal(t, "http://contract-otel:4318", cfg.Observability.OTLPEndpoint)
	assert.Equal(t, "v1.2.3-contract", cfg.Version)
}

func TestLoadFromEnv_SemanticCacheFields(t *testing.T) {
	setEnv(t, "CKODEX_SEMANTIC_CACHE_ADDR", "valkey-master.cache.svc:6379")
	setEnv(t, "CKODEX_SEMANTIC_CACHE_TTL", "30m")

	cfg := config.DefaultOperatorConfig()
	cfg.LoadFromEnv()

	assert.Equal(t, "valkey-master.cache.svc:6379", cfg.SemanticCacheAddr)
	assert.Equal(t, 30*time.Minute, cfg.SemanticCacheTTL)
}

func TestLoadFromEnv_InvalidDuration_Ignored(t *testing.T) {
	setEnv(t, "CKODEX_SEMANTIC_CACHE_TTL", "not-a-duration")

	cfg := config.DefaultOperatorConfig()
	original := cfg.SemanticCacheTTL
	cfg.LoadFromEnv()

	assert.Equal(t, original, cfg.SemanticCacheTTL, "invalid duration must be silently ignored")
}

func TestLoadFromEnv_InvalidBool_Ignored(t *testing.T) {
	setEnv(t, "CKODEX_FEATURE_ENABLE_SECURITY", "maybe")

	cfg := config.DefaultOperatorConfig()
	original := cfg.Features.EnableSecurity
	cfg.LoadFromEnv()

	assert.Equal(t, original, cfg.Features.EnableSecurity, "invalid bool must be silently ignored")
}

func TestLoadFromEnv_EmptyStringEnv_NoOverride(t *testing.T) {
	setEnv(t, "CKODEX_HF_MIRROR_URL", "")

	cfg := config.DefaultOperatorConfig()
	cfg.HuggingFaceMirrorURL = "original"
	cfg.LoadFromEnv()

	assert.Equal(t, "original", cfg.HuggingFaceMirrorURL, "empty env var must not overwrite existing value")
}

func TestLoadFromEnv_ProxyFields(t *testing.T) {
	setEnv(t, "HTTP_PROXY", "http://proxy.corp:3128")
	setEnv(t, "HTTPS_PROXY", "http://proxy.corp:3128")
	setEnv(t, "NO_PROXY", "10.0.0.0/8,172.16.0.0/12")

	cfg := config.DefaultOperatorConfig()
	cfg.LoadFromEnv()

	assert.Equal(t, "http://proxy.corp:3128", cfg.Proxy.HTTPProxy)
	assert.Equal(t, "http://proxy.corp:3128", cfg.Proxy.HTTPSProxy)
	assert.Equal(t, "10.0.0.0/8,172.16.0.0/12", cfg.Proxy.NoProxy)
}

func TestLoadFromEnv_FedRAMPMode(t *testing.T) {
	setEnv(t, "CKODEX_SECURITY_FEDRAMP_MODE", "true")

	cfg := config.DefaultOperatorConfig()
	cfg.LoadFromEnv()

	assert.True(t, cfg.Security.FedRAMPMode)
}

func TestLoadFromEnv_OTLPSamplingRate(t *testing.T) {
	setEnv(t, "CKODEX_OTEL_SAMPLING_RATE", "0.5")

	cfg := config.DefaultOperatorConfig()
	cfg.LoadFromEnv()

	assert.InDelta(t, 0.5, cfg.Observability.SamplingRate, 0.001)
}

// ---- DefaultFeatureGates -----------------------------------------------------

func TestDefaultFeatureGates_Idempotent(t *testing.T) {
	a := config.DefaultFeatureGates()
	b := config.DefaultFeatureGates()
	assert.Equal(t, a, b, "DefaultFeatureGates must be deterministic")
}

func TestDefaultFeatureGates_FromEnv_AllGates(t *testing.T) {
	// Flip every gate to the opposite of the default and verify they all change.
	gates := []struct {
		env     string
		getFlag func(config.FeatureGates) bool
	}{
		{"CKODEX_FEATURE_ENABLE_SCHEDULER", func(f config.FeatureGates) bool { return f.EnableScheduler }},
		{"CKODEX_FEATURE_ENABLE_GATEWAY", func(f config.FeatureGates) bool { return f.EnableGateway }},
		{"CKODEX_FEATURE_ENABLE_AUTOSCALER", func(f config.FeatureGates) bool { return f.EnableAutoscaler }},
		{"CKODEX_FEATURE_ENABLE_SECURITY", func(f config.FeatureGates) bool { return f.EnableSecurity }},
		{"CKODEX_FEATURE_ENABLE_CHAOS", func(f config.FeatureGates) bool { return f.EnableChaos }},
		{"CKODEX_FEATURE_ENABLE_DAPR", func(f config.FeatureGates) bool { return f.EnableDapr }},
		{"CKODEX_FEATURE_ENABLE_LOCAL_MODEL_CACHE", func(f config.FeatureGates) bool { return f.EnableLocalModelCache }},
		{"CKODEX_FEATURE_ENABLE_AUTH", func(f config.FeatureGates) bool { return f.EnableAuth }},
		{"CKODEX_FEATURE_ENABLE_OTEL_PIPELINE", func(f config.FeatureGates) bool { return f.EnableOTelPipeline }},
		{"CKODEX_FEATURE_ENABLE_SESSIONS", func(f config.FeatureGates) bool { return f.EnableSessions }},
		{"CKODEX_FEATURE_ENABLE_GRPC", func(f config.FeatureGates) bool { return f.EnableGRPC }},
		{"CKODEX_FEATURE_ENABLE_WEBHOOKS", func(f config.FeatureGates) bool { return f.EnableWebhooks }},
		{"CKODEX_FEATURE_ENABLE_EXPERIMENTAL_AGENTS", func(f config.FeatureGates) bool { return f.EnableExperimentalAgents }},
	}

	defaults := config.DefaultFeatureGates()
	for _, g := range gates {
		t.Run(g.env, func(t *testing.T) {
			defaultVal := g.getFlag(defaults)
			t.Setenv(g.env, boolStr(!defaultVal))

			f := config.DefaultFeatureGates()
			f.FromEnv()
			require.Equal(t, !defaultVal, g.getFlag(f),
				"gate %s: expected %v after setting env to %v", g.env, !defaultVal, !defaultVal)
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
