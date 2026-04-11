/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package config provides operator-level configuration and feature gates.
package config

import (
	"os"
	"strconv"
	"time"
)

// FeatureGates controls which subsystems are enabled at startup.
// Each gate defaults to a production-safe value.
// Override via environment variables: CKODEX_FEATURE_{GATE_NAME}=true|false.
type FeatureGates struct {
	// EnableScheduler enables the EPP scheduler controller.
	EnableScheduler bool `json:"enableScheduler"`

	// EnableGateway enables Gateway API resource management.
	EnableGateway bool `json:"enableGateway"`

	// EnableAutoscaler enables HPA/KEDA/WVA autoscaling.
	EnableAutoscaler bool `json:"enableAutoscaler"`

	// EnableSecurity enables SPIFFE/SPIRE infrastructure management.
	EnableSecurity bool `json:"enableSecurity"`

	// EnableChaos enables the chaos engine controller.
	EnableChaos bool `json:"enableChaos"`

	// EnableDapr enables Dapr workflow/actor integration.
	EnableDapr bool `json:"enableDapr"`

	// EnableLocalModelCache enables the LocalModelCache controller.
	EnableLocalModelCache bool `json:"enableLocalModelCache"`

	// EnableAuth enables JWT/OIDC authentication middleware.
	EnableAuth bool `json:"enableAuth"`

	// EnableOTelPipeline enables end-to-end OpenTelemetry tracing.
	EnableOTelPipeline bool `json:"enableOTelPipeline"`

	// EnableSessions enables InferenceSession/Actor/CoactorGroup controllers.
	EnableSessions bool `json:"enableSessions"`

	// EnableGRPC enables gRPC listener and GRPCRoute reconciliation.
	// Requires a backend that serves the V2 gRPC Inference Protocol (e.g., Triton).
	// vLLM's OpenAI-compatible server does NOT serve gRPC, so this defaults to false.
	EnableGRPC bool `json:"enableGRPC"`

	// EnableWebhooks enables the mutating and validating admission webhooks.
	// Requires cert-manager (or manual TLS cert provisioning) to be active in the cluster.
	// Set CKODEX_FEATURE_ENABLE_WEBHOOKS=true once cert-manager is installed.
	EnableWebhooks bool `json:"enableWebhooks"`

	// EnableExperimentalHardwareSelection enables automatic model artifact selection
	// based on detected hardware (e.g., appending -nvidia to OCI tags).
	EnableExperimentalHardwareSelection bool `json:"enableExperimentalHardwareSelection"`

	// EnableExperimentalStatusHardening enables the new DeploymentReady status condition
	// and stricter status reconciliation logic.
	EnableExperimentalStatusHardening bool `json:"enableExperimentalStatusHardening"`
}

// DefaultFeatureGates returns production-safe defaults.
// Core reconciliation is always on. Optional subsystems default to off.
func DefaultFeatureGates() FeatureGates {
	return FeatureGates{
		EnableScheduler:                     true,
		EnableGateway:                       true,
		EnableAutoscaler:                    true,
		EnableSecurity:                      false,
		EnableChaos:                         false,
		EnableDapr:                          false,
		EnableLocalModelCache:               false,
		EnableAuth:                          false,
		EnableOTelPipeline:                  true,
		EnableSessions:                      false,
		EnableGRPC:                          false,
		EnableWebhooks:                      false, // requires cert-manager in cluster; opt-in
		EnableExperimentalHardwareSelection: false,
		EnableExperimentalStatusHardening:   false,
	}
}

// FromEnv loads feature gate overrides from environment variables.
// Format: CKODEX_FEATURE_ENABLE_SCHEDULER=true.
func (f *FeatureGates) FromEnv() {
	envBool("CKODEX_FEATURE_ENABLE_SCHEDULER", &f.EnableScheduler)
	envBool("CKODEX_FEATURE_ENABLE_GATEWAY", &f.EnableGateway)
	envBool("CKODEX_FEATURE_ENABLE_AUTOSCALER", &f.EnableAutoscaler)
	envBool("CKODEX_FEATURE_ENABLE_SECURITY", &f.EnableSecurity)
	envBool("CKODEX_FEATURE_ENABLE_CHAOS", &f.EnableChaos)
	envBool("CKODEX_FEATURE_ENABLE_DAPR", &f.EnableDapr)
	envBool("CKODEX_FEATURE_ENABLE_LOCAL_MODEL_CACHE", &f.EnableLocalModelCache)
	envBool("CKODEX_FEATURE_ENABLE_AUTH", &f.EnableAuth)
	envBool("CKODEX_FEATURE_ENABLE_OTEL_PIPELINE", &f.EnableOTelPipeline)
	envBool("CKODEX_FEATURE_ENABLE_SESSIONS", &f.EnableSessions)
	envBool("CKODEX_FEATURE_ENABLE_GRPC", &f.EnableGRPC)
	envBool("CKODEX_FEATURE_ENABLE_WEBHOOKS", &f.EnableWebhooks)
	envBool("CKODEX_FEATURE_ENABLE_EXPERIMENTAL_HARDWARE_SELECTION", &f.EnableExperimentalHardwareSelection)
	envBool("CKODEX_FEATURE_ENABLE_EXPERIMENTAL_STATUS_HARDENING", &f.EnableExperimentalStatusHardening)
}

// OperatorConfig holds all operator-level configuration.
type OperatorConfig struct {
	// Features controls which subsystems are enabled.
	Features FeatureGates `json:"features"`

	// Defaults configures default values for workload resources.
	Defaults DefaultsConfig `json:"defaults"`

	// Scheduler configures EPP scheduler defaults.
	Scheduler SchedulerDefaults `json:"scheduler"`

	// Gateway configures gateway defaults.
	Gateway GatewayDefaults `json:"gateway"`

	// Auth configures OIDC authentication.
	Auth AuthConfig `json:"auth"`

	// Security configures OPA/Gatekeeper policy parameters.
	Security SecurityConfig `json:"security"`

	// AuditSink configures audit event persistence.
	AuditSink AuditSinkConfig `json:"auditSink"`

	// HuggingFaceMirrorURL overrides the public huggingface.co base URL for all
	// HF API and file download requests. Use this for air-gapped or on-premise
	// HuggingFace Hub proxies (Harbor, self-hosted HF). Must include scheme,
	// e.g. "https://hf-mirror.corp.internal".
	// When empty, public https://huggingface.co is used.
	HuggingFaceMirrorURL string `json:"huggingFaceMirrorURL,omitempty"`

	// Proxy configures outbound HTTP/HTTPS proxy for storage and OIDC requests.
	Proxy ProxyConfig `json:"proxy,omitempty"`

	// Observability configures OpenTelemetry collection.
	Observability ObservabilityConfig `json:"observability"`

	// SemanticCacheAddr is the Redis/Valkey address for the distributed SemanticCache.
	// Format: "host:port" (e.g., "valkey:6379" or "redis-master.cache.svc:6379").
	// When empty, the operator falls back to a local in-memory cache (suitable for
	// single-replica deployments and CI). For HA production deployments, always
	// set this to a replicated Redis/Valkey endpoint.
	SemanticCacheAddr string `json:"semanticCacheAddr,omitempty"`

	// SemanticCacheTTL controls how long inference responses are cached.
	// Default: 1 hour. Use shorter TTLs for rapidly-changing model outputs.
	SemanticCacheTTL time.Duration `json:"semanticCacheTTL,omitempty"`

	// PrometheusURL is the base URL of the Prometheus server used for promotion gate
	// metric queries by the ModelOnboarding controller.
	// Format: "http://prometheus.monitoring.svc:9090" (no trailing slash).
	// When empty, the noopMetricsQuerier is used and all gate metric checks pass
	// unconditionally (backward-compatible default for clusters without Prometheus).
	// Override via CKODEX_PROMETHEUS_URL.
	PrometheusURL string `json:"prometheusURL,omitempty"`

	// Version is the operator version, injected at build time.
	// Defaults to "dev". Override via VERSION environment variable (contract).
	Version string `json:"version"`
}

// DefaultsConfig holds default workload configuration.
type DefaultsConfig struct {
	// RuntimeImage is the default vLLM runtime image.
	RuntimeImage string `json:"runtimeImage"`

	// SchedulerImage is the default EPP scheduler image.
	SchedulerImage string `json:"schedulerImage"`

	// StorageInitializerImage is the default storage initializer image.
	StorageInitializerImage string `json:"storageInitializerImage"`

	// DefaultReplicas is the default number of model server replicas.
	DefaultReplicas int32 `json:"defaultReplicas"`
}

// SchedulerDefaults configures the EPP scheduler.
type SchedulerDefaults struct {
	// Image is the EPP container image.
	Image string `json:"image"`

	// DefaultPlugins is the default plugin pipeline for new EndpointPickerConfigs.
	DefaultPlugins []string `json:"defaultPlugins"`
}

// GatewayDefaults configures gateway creation.
type GatewayDefaults struct {
	// GatewayClassName is the default GatewayClass.
	GatewayClassName string `json:"gatewayClassName"`

	// ListenerPort is the default HTTP listener port.
	ListenerPort int32 `json:"listenerPort"`
}

// ProxyConfig carries HTTP/HTTPS proxy settings for all outbound storage and
// OIDC provider requests made by the operator. Mirrors Go's standard proxy env vars.
type ProxyConfig struct {
	// HTTPProxy is used for plain HTTP requests (e.g., http://proxy.corp:3128).
	HTTPProxy string `json:"httpProxy,omitempty"`
	// HTTPSProxy is used for HTTPS requests.
	HTTPSProxy string `json:"httpsProxy,omitempty"`
	// NoProxy is a comma-separated list of hosts/CIDRs that bypass the proxy.
	NoProxy string `json:"noProxy,omitempty"`
}

// SecurityConfig configures OPA/Gatekeeper policy enforcement.
type SecurityConfig struct {
	// AllowedRegistries are container image registry prefixes accepted by the
	// LLMImageAllowlist OPA constraint. Override via operator config or env.
	// Default list includes ckodex, vllm, kserve, and distroless images.
	AllowedRegistries []string `json:"allowedRegistries"`

	// MaxGPUsPerNamespace is the GPU ceiling enforced by LLMResourceQuota constraint.
	MaxGPUsPerNamespace int64 `json:"maxGPUsPerNamespace"`

	// MaxReplicasPerService is the replica ceiling per LLMInferenceService.
	MaxReplicasPerService int64 `json:"maxReplicasPerService"`

	// FedRAMPMode, when true, rejects hf:// model URIs via the admission webhook.
	// In FedRAMP environments all model artifacts must originate from an authorized registry.
	// Requires EnableWebhooks=true to be enforced at admission time.
	FedRAMPMode bool `json:"fedRAMPMode"`
}

// AuthConfig configures OIDC authentication.
type AuthConfig struct {
	// IssuerURL is the OIDC issuer URL.
	IssuerURL string `json:"issuerURL"`

	// Audience is the expected JWT audience.
	Audience string `json:"audience"`

	// RequiredScopes are the scopes required for inference access.
	RequiredScopes []string `json:"requiredScopes"`

	// JWKSCacheTTL is how long to cache JWKS keys.
	JWKSCacheTTL time.Duration `json:"jwksCacheTTL"`
}

// AuditSinkConfig configures where and how audit events are persisted.
type AuditSinkConfig struct {
	// Type selects the audit sink backend.
	// Supported values: "stdout" (default), "file", "otlp-log".
	// +kubebuilder:validation:Enum=stdout;file;otlp-log
	Type string `json:"type"`

	// FilePath is the log file path when Type="file".
	// The file is appended-to and rotated at midnight UTC.
	// +optional
	FilePath string `json:"filePath,omitempty"`

	// RetentionDays is how long audit log files are retained before deletion.
	// HIPAA requires 7 years (2555 days). SOC2 requires 1 year (365 days).
	// 0 = keep forever (default).
	RetentionDays int `json:"retentionDays"`

	// PIIRedaction enables regex-based PII detection and redaction in audit
	// event details before they are written to any sink.
	// Should be true in all production environments.
	PIIRedaction bool `json:"piiRedaction"`
}

// ObservabilityConfig configures OpenTelemetry.
type ObservabilityConfig struct {
	// OTLPEndpoint is the OTLP collector gRPC endpoint.
	OTLPEndpoint string `json:"otlpEndpoint"`

	// SamplingRate is the trace sampling rate (0.0 to 1.0).
	SamplingRate float64 `json:"samplingRate"`

	// ServiceName is the OTel service name.
	ServiceName string `json:"serviceName"`
}

// DefaultOperatorConfig returns production defaults.
func DefaultOperatorConfig() OperatorConfig {
	return OperatorConfig{
		Features: DefaultFeatureGates(),
		Defaults: DefaultsConfig{
			// CPU-optimized vLLM image for ARM64/x86 nodes (no GPU).
			// Pinned to a specific version — :latest is a supply chain risk and air-gapped blocker.
			RuntimeImage:            "public.ecr.aws/q9t5s3a7/vllm-cpu-release-repo:v0.17.1",
			SchedulerImage:          "us-central1-docker.pkg.dev/k8s-staging-gateway-api/gateway-api-inference-extension/epp:main",
			StorageInitializerImage: "kserve/storage-initializer:v0.14.1",
			DefaultReplicas:         1,
		},
		Scheduler: SchedulerDefaults{
			Image: "us-central1-docker.pkg.dev/k8s-staging-gateway-api/gateway-api-inference-extension/epp:main",
			DefaultPlugins: []string{
				"prefix-cache-scorer",
				"queue-scorer",
				"kv-cache-utilization-scorer",
				"max-score-picker",
			},
		},
		Gateway: GatewayDefaults{
			GatewayClassName: "envoy",
			ListenerPort:     80,
		},
		Auth: AuthConfig{
			RequiredScopes: []string{"inference"},
			JWKSCacheTTL:   1 * time.Hour,
		},
		Security: SecurityConfig{
			AllowedRegistries: []string{
				"ghcr.io/ckodex/",
				"vllm/",
				"kserve/",
				"gcr.io/distroless/",
				"public.ecr.aws/q9t5s3a7/",
			},
			MaxGPUsPerNamespace:   8,
			MaxReplicasPerService: 16,
		},
		AuditSink: AuditSinkConfig{
			Type:          "stdout",
			RetentionDays: 0,
			PIIRedaction:  true, // on by default — opt out explicitly if needed
		},
		Observability: ObservabilityConfig{
			OTLPEndpoint: "localhost:4317",
			SamplingRate: 0.1,
			ServiceName:  "ckodex-kserve-llm-operator",
		},
		// Empty addr → in-memory fallback; override with CKODEX_SEMANTIC_CACHE_ADDR in prod.
		SemanticCacheAddr: "",
		SemanticCacheTTL:  1 * time.Hour,
		Version:           "dev",
	}
}

// LoadFromEnv populates the config from environment variables.
func (c *OperatorConfig) LoadFromEnv() {
	c.Features.FromEnv()

	envStr("CKODEX_RUNTIME_IMAGE", &c.Defaults.RuntimeImage)
	envStr("CKODEX_SCHEDULER_IMAGE", &c.Scheduler.Image)
	envStr("CKODEX_AUTH_ISSUER_URL", &c.Auth.IssuerURL)
	envStr("CKODEX_AUTH_AUDIENCE", &c.Auth.Audience)
	envStr("CKODEX_OTEL_ENDPOINT", &c.Observability.OTLPEndpoint)
	envStr("CKODEX_OTEL_SERVICE_NAME", &c.Observability.ServiceName)
	envFloat("CKODEX_OTEL_SAMPLING_RATE", &c.Observability.SamplingRate)

	envStr("CKODEX_HF_MIRROR_URL", &c.HuggingFaceMirrorURL)
	envStr("HTTP_PROXY", &c.Proxy.HTTPProxy)
	envStr("HTTPS_PROXY", &c.Proxy.HTTPSProxy)
	envStr("NO_PROXY", &c.Proxy.NoProxy)
	envBool("CKODEX_SECURITY_FEDRAMP_MODE", &c.Security.FedRAMPMode)
	envStr("CKODEX_SEMANTIC_CACHE_ADDR", &c.SemanticCacheAddr)
	envDuration("CKODEX_SEMANTIC_CACHE_TTL", &c.SemanticCacheTTL)
	envStr("CKODEX_PROMETHEUS_URL", &c.PrometheusURL)

	// OIS / OTel Contract Overrides
	envStr("VERSION", &c.Version)
	envStr("OTEL_EXPORTER_OTLP_ENDPOINT", &c.Observability.OTLPEndpoint)
}

func envBool(key string, target *bool) {
	if v, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseBool(v); err == nil {
			*target = parsed
		}
	}
}

func envStr(key string, target *string) {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		*target = v
	}
}

func envFloat(key string, target *float64) {
	if v, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			*target = parsed
		}
	}
}

func envDuration(key string, target *time.Duration) {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			*target = parsed
		}
	}
}
