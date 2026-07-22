/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=llmisvc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LLMInferenceService is the Schema for the LLM inference services API.
// It manages the full lifecycle of LLM inference workloads including
// deployment, routing, scheduling, and autoscaling.
type LLMInferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMInferenceServiceSpec   `json:"spec,omitempty"`
	Status LLMInferenceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LLMInferenceServiceList contains a list of LLMInferenceService.
type LLMInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMInferenceService `json:"items"`
}

// LLMInferenceServiceSpec defines the desired state of LLMInferenceService.
type LLMInferenceServiceSpec struct {
	// Model specifies the model to serve.
	Model ModelSpec `json:"model"`

	// Replicas is the number of model server replicas (decode workers).
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Parallelism configures distributed inference across GPUs/nodes.
	// +optional
	Parallelism *ParallelismSpec `json:"parallelism,omitempty"`

	// Scaling configures autoscaling behavior.
	// +optional
	Scaling *ScalingSpec `json:"scaling,omitempty"`

	// Template defines the pod template for model server pods.
	// The operator enforces "Guaranteed QoS" by synchronizing resource requests
	// to match limits. If TerminationGracePeriodSeconds is not specified, it
	// defaults to 30s for graceful model shutdown.
	Template corev1.PodTemplateSpec `json:"template"`

	// Prefill configures disaggregated prefill workers.
	// When set, the operator creates separate prefill pods that handle
	// the compute-intensive prefill phase independently. Prefill pods also
	// follow the "Guaranteed QoS" and 30s termination grace period patterns.
	// +optional
	Prefill *PrefillSpec `json:"prefill,omitempty"`

	// Worker configures worker nodes for multi-node distributed inference
	// using LeaderWorkerSet.
	// +optional
	Worker *WorkerSpec `json:"worker,omitempty"`

	// Router configures gateway, route, and scheduler for traffic management.
	Router RouterSpec `json:"router"`

	// BaseRefs references LLMInferenceServiceConfig resources for
	// configuration composition. Merge order: WellKnown → BaseRefs → Spec.
	// +optional
	BaseRefs []ConfigReference `json:"baseRefs,omitempty"`

	// AutoOptimize enables automatic hardware detection and optimization.
	// When true, the operator will apply best-practice defaults for the
	// detected hardware (e.g., Apple Silicon / ARM64).
	// +optional
	AutoOptimize *bool `json:"autoOptimize,omitempty"`

	// AllowedTenants is the list of tenant IDs (ckodex.com/tenant-id label values)
	// permitted to send inference requests to this service. Empty list = no restriction.
	// Enforced by the LLMModelAccess OPA Gatekeeper constraint.
	// +optional
	AllowedTenants []string `json:"allowedTenants,omitempty"`

	// CostAllocationTags are arbitrary key-value labels propagated to OTel metric
	// attributes, Deployment labels, and KEDA ScaledObject annotations so FinOps
	// tooling can group GPU-second and token costs by team, project, or cost-center.
	// +optional
	CostAllocationTags map[string]string `json:"costAllocationTags,omitempty"`

	// SLO defines the service level objective for this inference service.
	// The operator annotates the Deployment and emits Prometheus recording rules
	// so alerting can be tied to the declared targets rather than hard-coded thresholds.
	// +optional
	SLO *SLOSpec `json:"slo,omitempty"`

	// Canary configures weighted traffic splitting between this (canary) service
	// and a stable base service. When set, the gateway reconciler produces an
	// HTTPRoute with two weighted backends instead of a single backend.
	// +optional
	Canary *CanarySpec `json:"canary,omitempty"`

	// SpeculativeDecoding configures speculative decoding for vLLM v0.24.0+.
	// MTP (Multi-Token Prediction) provides ~2× throughput on Llama/Mistral without quality loss.
	// +optional
	SpeculativeDecoding *SpeculativeDecodingSpec `json:"speculativeDecoding,omitempty"`

	// KVCache configures KV cache dtype and CPU offload for vLLM v0.24.0+.
	// Use Dtype:"fp8" for ~50% VRAM reduction on Hopper+ GPUs.
	// +optional
	KVCache *KVCacheSpec `json:"kvCache,omitempty"`

	// Quantization configures weight quantization for reduced memory footprint.
	// AWQ and GPTQ require pre-quantized model weights. GGUF routes to the
	// quant-cpp engine automatically. bitsandbytes and fp8 quantize at load time.
	// +optional
	Quantization *QuantizationSpec `json:"quantization,omitempty"`

	// Engine specifies the inference engine to use.
	// Defaults to 'vllm'. Supported: 'vllm', 'quant-cpp'.
	// +kubebuilder:default="vllm"
	// +optional
	Engine string `json:"engine,omitempty"`

	// ToolSurface declares reachable APIs and external connectors for this service.
	// +optional
	ToolSurface *ToolSurface `json:"toolSurface,omitempty"`

	// Observability configures telemetry sinks (logs, traces, metrics).
	// +optional
	Observability *ObservabilitySpec `json:"observability,omitempty"`
}

// ObservabilitySpec configures telemetry for an inference service.
type ObservabilitySpec struct {
	// Sink select the telemetry destination for Vector and Audit signals.
	// +optional
	Sink *TelemetrySink `json:"sink,omitempty"`
}

// TelemetrySink defines a destination for OIS signals.
type TelemetrySink struct {
	// Type of sink: "stdout", "otlp", "loki", "elasticsearch".
	// +kubebuilder:validation:Enum=stdout;otlp;loki;elasticsearch
	Type string `json:"type"`

	// Endpoint for the sink (e.g., "http://otel-collector:4318").
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// ToolSurface defines allowed external reachability.
type ToolSurface struct {
	// AllowedAPIs is a list of FQDNs that the model is permitted to reach.
	// +optional
	AllowedAPIs []string `json:"allowedApis,omitempty"`

	// AllowedCIDRs is a list of network ranges the model is permitted to reach.
	// +optional
	AllowedCIDRs []string `json:"allowedCidrs,omitempty"`
}

// SLOSpec declares the service level objectives for an LLMInferenceService.
type SLOSpec struct {
	// TargetP99LatencyMs is the maximum acceptable P99 end-to-end latency in milliseconds.
	// Violations trigger the LLMServiceSLOLatencyBreach alert.
	// +kubebuilder:validation:Minimum=1
	TargetP99LatencyMs int64 `json:"targetP99LatencyMs"`

	// TargetAvailability is the minimum acceptable availability ratio (0.0–1.0).
	// Example: 0.999 = three nines.
	TargetAvailability float64 `json:"targetAvailability"`

	// ErrorBudgetDays is the rolling window (in days) over which the error budget
	// is calculated. Defaults to 30.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +optional
	ErrorBudgetDays int `json:"errorBudgetDays,omitempty"`
}

// CanarySpec configures progressive traffic splitting for a canary rollout.
// The canary service receives Weight% of traffic; the base service receives
// (100-Weight)% of traffic. Set Weight=100 to promote the canary to stable.
type CanarySpec struct {
	// Weight is the percentage of traffic (0–100) routed to this canary service.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Weight int32 `json:"weight"`

	// BaseModel is the name of the stable LLMInferenceService in the same namespace
	// that receives the remaining (100-Weight)% of traffic.
	// +kubebuilder:validation:MinLength=1
	BaseModel string `json:"baseModel"`
}

// ModelSpec defines the model to serve.
type ModelSpec struct {
	// URI is the model artifact location.
	// Supported schemes: hf:// (HuggingFace Hub), hf-mount:// (HuggingFace CSI lazy mount),
	// hf-mirror://, s3://, swfs://, seaweedfs://, gs://, pvc://, oci://, ocis://, modelpack://.
	// hf-mount:// uses the hf-csi-driver to mount repos as a FUSE/NFS filesystem —
	// only accessed bytes are fetched, eliminating full model downloads.
	// `ocis://` is an explicit secure-OCI alias and follows the same runtime
	// verification path as `oci://`, while making the intent visible at the spec boundary.
	// +kubebuilder:validation:Pattern=`^(hf|hf-mount|hf-mirror|s3|swfs|seaweedfs|gs|pvc|oci|ocis|modelpack|https?)://.*$`
	URI string `json:"uri"`

	// Name is the model identifier used in inference requests.
	// This is the value clients use in the "model" field of chat/completion requests.
	Name string `json:"name"`

	// Storage configures credentials for model download.
	// Mirrors LocalModelCache credential support.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// HardwareAware enables automatic hardware-specific artifact selection.
	// When true, the operator appends hardware suffixes to OCI tags (e.g., -nvidia).
	// Requires ENABLE_EXPERIMENTAL_HARDWARE_SELECTION feature gate.
	// +optional
	HardwareAware bool `json:"hardwareAware,omitempty"`
}

// StorageSpec configures storage credentials for model download.
type StorageSpec struct {
	// SecretRef is a reference to a Secret containing storage credentials.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// ServiceAccountName is the name of a ServiceAccount with storage access.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// StorageContainerRef references a ClusterStorageContainer for download config.
	// +optional
	StorageContainerRef string `json:"storageContainerRef,omitempty"`

	// VaultRef is a reference to a Vault path containing storage credentials.
	// +optional
	VaultRef string `json:"vaultRef,omitempty"`

	// VaultAddr is the address of the HashiCorp Vault server.
	// +optional
	VaultAddr string `json:"vaultAddr,omitempty"`

	// ExternalSecret configures native integration with external-secrets.io.
	// When set, the operator creates an ExternalSecret resource to sync
	// credentials from an external provider (Vault, AWS SM, etc.).
	// +optional
	ExternalSecret *ExternalSecretSpec `json:"externalSecret,omitempty"`
}

// ExternalSecretSpec defines the desired state of the managed ExternalSecret.
type ExternalSecretSpec struct {
	// SecretStoreRef references the SecretStore/ClusterSecretStore to use.
	SecretStoreRef SecretStoreRef `json:"secretStoreRef"`

	// RefreshInterval is how often to re-sync the secret from the provider.
	// +kubebuilder:default="1h"
	// +optional
	RefreshInterval string `json:"refreshInterval,omitempty"`

	// Data defines the mapping of remote keys to local secret keys.
	// +optional
	Data []ExternalSecretData `json:"data,omitempty"`
}

// SecretStoreRef references a SecretStore or ClusterSecretStore.
type SecretStoreRef struct {
	// Name of the SecretStore.
	Name string `json:"name"`

	// Kind of the SecretStore (SecretStore or ClusterSecretStore).
	// +kubebuilder:default="SecretStore"
	// +optional
	Kind string `json:"kind,omitempty"`
}

// ExternalSecretData defines a single mapping of a remote secret to a local key.
type ExternalSecretData struct {
	// SecretKey is the key in the resulting Kubernetes Secret.
	SecretKey string `json:"secretKey"`

	// RemoteRef defines where to fetch the secret from the provider.
	RemoteRef ExternalSecretRemoteRef `json:"remoteRef"`
}

// ExternalSecretRemoteRef defines the remote key and optional property.
type ExternalSecretRemoteRef struct {
	// Key is the name/path of the secret in the external provider.
	Key string `json:"key"`

	// Property is the specific field to extract from the remote secret.
	// +optional
	Property string `json:"property,omitempty"`
}

// ParallelismSpec configures distributed inference parallelism.
type ParallelismSpec struct {
	// Tensor is the tensor parallelism degree — splits model layers across GPUs within a node.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Tensor *int32 `json:"tensor,omitempty"`

	// Data is the data parallelism degree — runs multiple model replicas.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Data *int32 `json:"data,omitempty"`

	// DataLocal controls GPUs per node for optimal NUMA affinity.
	// +kubebuilder:validation:Minimum=1
	// +optional
	DataLocal *int32 `json:"dataLocal,omitempty"`

	// Expert enables expert parallelism for MoE models.
	// When true, distributes Mixture-of-Experts across GPUs.
	// +optional
	Expert bool `json:"expert,omitempty"`

	// Pipeline is the pipeline parallelism degree (--pipeline-parallel-size).
	// Splits model layers sequentially across nodes. Combine with Tensor for 2D parallelism.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Pipeline *int32 `json:"pipeline,omitempty"`

	// EPLBEnabled enables Expert Parallelism Load Balancing for MoE models (--enable-eplb).
	// Dynamically rebalances expert routing — critical for DeepSeek-V4 and Gemma 4 MoE.
	// +optional
	EPLBEnabled bool `json:"eplbEnabled,omitempty"`
}

// SpeculativeDecodingSpec configures speculative decoding (vLLM v0.24.0+).
type SpeculativeDecodingSpec struct {
	// Method selects the draft strategy: "mtp", "eagle", "medusa", "ngram".
	// +kubebuilder:validation:Enum=mtp;eagle;medusa;ngram
	Method string `json:"method"`

	// NumTokens is the number of speculative tokens per step (--spec-tokens).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=16
	// +optional
	NumTokens *int32 `json:"numTokens,omitempty"`

	// DraftModel URI for external drafter (eagle/medusa). Empty for mtp/ngram.
	// +optional
	DraftModel string `json:"draftModel,omitempty"`
}

// KVCacheSpec configures KV cache storage behavior.
type KVCacheSpec struct {
	// Dtype overrides KV cache type: "auto", "fp8", "fp16", "bf16".
	// "fp8" enables FP8 KV quantization (~50% memory reduction, requires Hopper+).
	// +kubebuilder:validation:Enum=auto;fp8;fp16;bf16
	// +kubebuilder:default=auto
	// +optional
	Dtype string `json:"dtype,omitempty"`

	// SwapSpaceGB sets CPU RAM offload in GiB (--cpu-offload-gb).
	// The JSON name is retained for API compatibility with earlier releases.
	// +kubebuilder:validation:Minimum=0
	// +optional
	SwapSpaceGB *int32 `json:"swapSpaceGB,omitempty"`
}

// QuantizationSpec configures weight quantization for reduced memory footprint.
type QuantizationSpec struct {
	// Method selects the quantization algorithm.
	// "awq" and "gptq" require pre-quantized model weights.
	// "gguf" routes to the quant-cpp engine automatically.
	// "bitsandbytes" and "fp8" quantize at load time.
	// +kubebuilder:validation:Enum=awq;gptq;gguf;bitsandbytes;fp8
	Method string `json:"method"`

	// CheckpointPath is retained for API compatibility. vLLM v0.24.0 loads
	// GPTQ metadata from the model and does not accept --gptq-ckpt-path.
	// +optional
	CheckpointPath string `json:"checkpointPath,omitempty"`
}

// ScalingSpec configures autoscaling for the inference service.
type ScalingSpec struct {
	// MinReplicas is the minimum number of replicas.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the maximum number of replicas.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// WVA configures the Workload Variant Autoscaler.
	// +optional
	WVA *WVASpec `json:"wva,omitempty"`

	// KEDA configures KEDA ScaledObject generation.
	// +optional
	KEDA *KEDASpec `json:"keda,omitempty"`

	// HPA configures HorizontalPodAutoscaler generation.
	// +optional
	HPA *HPASpec `json:"hpa,omitempty"`
}

// WVASpec configures the Workload Variant Autoscaler.
type WVASpec struct {
	// VariantCost is the relative cost per replica for this variant.
	// WVA scales cheaper variants first. Default: "10.0".
	// +kubebuilder:default="10.0"
	// +optional
	VariantCost string `json:"variantCost,omitempty"`
}

// KEDASpec configures KEDA integration for autoscaling.
type KEDASpec struct {
	// PollingInterval is how often KEDA checks metrics (seconds).
	// +kubebuilder:default=30
	// +optional
	PollingInterval *int32 `json:"pollingInterval,omitempty"`

	// CooldownPeriod is the wait time after last trigger before scaling down (seconds).
	// +kubebuilder:default=300
	// +optional
	CooldownPeriod *int32 `json:"cooldownPeriod,omitempty"`

	// InitialCooldownPeriod is cooldown before first scale-down after creation (seconds).
	// +kubebuilder:default=120
	// +optional
	InitialCooldownPeriod *int32 `json:"initialCooldownPeriod,omitempty"`

	// IdleReplicaCount is the replica count when idle (enables scale-to-zero).
	// +kubebuilder:validation:Minimum=0
	// +optional
	IdleReplicaCount *int32 `json:"idleReplicaCount,omitempty"`

	// Fallback configures the safety net when metrics pipeline fails.
	// +optional
	Fallback *KEDAFallbackSpec `json:"fallback,omitempty"`
}

// KEDAFallbackSpec defines fallback behavior when metrics are unavailable.
type KEDAFallbackSpec struct {
	// FailureThreshold is how many consecutive metric failures before fallback.
	// +kubebuilder:default=3
	FailureThreshold int32 `json:"failureThreshold"`

	// Replicas is the replica count to use during fallback.
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas"`
}

// HPASpec configures HPA as a fallback autoscaler.
type HPASpec struct {
	// TargetCPUUtilizationPercentage is the target CPU percent for HPA.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=80
	// +optional
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`
}

// PrefillSpec configures disaggregated prefill workers.
type PrefillSpec struct {
	// Replicas is the number of prefill worker replicas.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Template defines the pod template for prefill pods.
	Template corev1.PodTemplateSpec `json:"template"`
}

// WorkerSpec configures worker nodes for multi-node distributed inference.
// Uses PodTemplateSpec to support serviceAccountName, labels, and annotations.
type WorkerSpec struct {
	// Template defines the pod template for worker pods in LeaderWorkerSet.
	Template corev1.PodTemplateSpec `json:"template"`
}

// RouterSpec configures traffic routing and scheduling.
type RouterSpec struct {
	// Gateway configures the Gateway resource.
	Gateway GatewaySpec `json:"gateway"`

	// Route configures the HTTPRoute resource.
	Route RouteSpec `json:"route"`

	// Scheduler configures the KV-cache aware scheduler (EPP).
	Scheduler SchedulerSpec `json:"scheduler"`
}

// GatewaySpec configures Gateway resource creation.
type GatewaySpec struct {
	// Managed indicates the operator should create and manage the Gateway.
	// +optional
	Managed *ManagedGatewaySpec `json:"managed,omitempty"`

	// ExistingRef references an existing Gateway to use.
	// +optional
	ExistingRef *GatewayRef `json:"existingRef,omitempty"`
}

// ManagedGatewaySpec configures a managed Gateway.
type ManagedGatewaySpec struct {
	// GatewayClassName is the name of the GatewayClass to use.
	// +kubebuilder:default="envoy"
	// +optional
	GatewayClassName string `json:"gatewayClassName,omitempty"`
}

// GatewayRef references an existing Gateway.
type GatewayRef struct {
	// Name of the Gateway.
	Name string `json:"name"`

	// Namespace of the Gateway.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RouteSpec configures HTTPRoute creation.
type RouteSpec struct {
	// HTTPRoute configures HTTP routing.
	// +optional
	HTTPRoute *HTTPRouteSpec `json:"httpRoute,omitempty"`
}

// HTTPRouteSpec configures HTTPRoute details.
type HTTPRouteSpec struct {
	// Hostnames are the hostnames to match.
	// +optional
	Hostnames []string `json:"hostnames,omitempty"`

	// Resilience configures timeouts and retries for this route.
	// +optional
	Resilience *ResilienceSpec `json:"resilience,omitempty"`
}

// ResilienceSpec defines timeout and retry parameters for inference routing.
// Based on Gateway API GEP-1735 and implementation-specific extensions (Envoy).
type ResilienceSpec struct {
	// Timeout defines the request timeout.
	// +kubebuilder:default="30s"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// MaxRetries is the maximum number of retry attempts.
	// +kubebuilder:default=3
	// +optional
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// RetryOn specifies the conditions under which to retry.
	// +kubebuilder:default="5xx,connect-failure,refused-stream"
	// +optional
	RetryOn string `json:"retryOn,omitempty"`
}

// SchedulerSpec configures the KV-cache aware scheduler.
type SchedulerSpec struct {
	// Pool configures the InferencePool resource.
	Pool InferencePoolSpec `json:"pool"`

	// Config configures the EndpointPickerConfig for the scheduler.
	// +optional
	Config *SchedulerConfigSpec `json:"config,omitempty"`

	// Replicas is the number of EPP (Endpoint Picker Pod) replicas for HA.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

// InferencePoolSpec configures the InferencePool.
type InferencePoolSpec struct {
	// Selector is the label selector for inference workload pods.
	// Defaults to app.kubernetes.io/component=llminferenceservice-workload.
	// +optional
	Selector map[string]string `json:"selector,omitempty"`
}

// SchedulerConfigSpec defines how to provide the scheduler config.
type SchedulerConfigSpec struct {
	// Inline specifies the EndpointPickerConfig inline.
	// +optional
	Inline *EndpointPickerConfigSpec `json:"inline,omitempty"`

	// Ref references a ConfigMap containing the scheduler config.
	// +optional
	Ref *SchedulerConfigRef `json:"ref,omitempty"`
}

// SchedulerConfigRef references a ConfigMap for scheduler configuration.
type SchedulerConfigRef struct {
	// Name of the ConfigMap.
	Name string `json:"name"`

	// Key in the ConfigMap containing the config YAML.
	// +kubebuilder:default="endpoint-picker-config.yaml"
	// +optional
	Key string `json:"key,omitempty"`
}

// ConfigReference references an LLMInferenceServiceConfig for composition.
type ConfigReference struct {
	// Name of the LLMInferenceServiceConfig.
	Name string `json:"name"`
}

// ----- Status -----

// LLMInferenceServiceStatus defines the observed state of LLMInferenceService.
type LLMInferenceServiceStatus struct {
	// StatePlanes represents the governed composite state of the model system.
	// +optional
	StatePlanes StatePlanes `json:"statePlanes,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// URL is the inference endpoint URL.
	// +optional
	URL string `json:"url,omitempty"`

	// Replicas is the current number of ready replicas.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ModelReady indicates whether the model is loaded and serving.
	// Determined by V2 protocol GET /v2/health/ready.
	// +optional
	ModelReady bool `json:"modelReady,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Optimized indicates whether the model is running with WellKnown optimizations applied.
	// +optional
	Optimized bool `json:"optimized,omitempty"`

	// DetectedHardware is the hardware type identified by the operator for this service.
	// +optional
	DetectedHardware string `json:"detectedHardware,omitempty"`

	// AdaptiveMetrics represents real-time performance and load pressure.
	// +optional
	AdaptiveMetrics *AdaptiveMetrics `json:"adaptiveMetrics,omitempty"`
}

// AdaptiveMetrics represents real-time performance data for the service.
type AdaptiveMetrics struct {
	// P50Latency is the median end-to-end latency.
	P50Latency string `json:"p50Latency,omitempty"`
	// P95Latency is the 95th percentile latency.
	P95Latency string `json:"p95Latency,omitempty"`
	// P99Latency is the 99th percentile latency.
	P99Latency string `json:"p99Latency,omitempty"`
	// QueueDepth is the number of pending requests in the priority queue.
	QueueDepth int64 `json:"queueDepth,omitempty"`
	// LoadLevel is the current graceful degradation state (None, Light, Moderate, Severe).
	LoadLevel string `json:"loadLevel,omitempty"`
}

// Condition types for LLMInferenceService.
const (
	// ConditionReady indicates the service is ready to serve inference requests.
	ConditionReady = "Ready"

	// ConditionDeploymentReady indicates the underlying deployment is ready.
	ConditionDeploymentReady = "DeploymentReady"

	// ConditionGatewayReady indicates the gateway and routes are configured.
	ConditionGatewayReady = "GatewayReady"

	// ConditionSchedulerReady indicates the EPP scheduler is running.
	ConditionSchedulerReady = "SchedulerReady"

	// ConditionModelLoaded indicates the model has been downloaded and loaded.
	ConditionModelLoaded = "ModelLoaded"

	// ConditionModelOptimized indicates model-specific optimizations (WellKnown) were applied.
	ConditionModelOptimized = "ModelOptimized"
)

func init() {
	SchemeBuilder.Register(&LLMInferenceService{}, &LLMInferenceServiceList{})
}
