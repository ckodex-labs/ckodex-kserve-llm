/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	// ModelRevision is the declared Hugging Face revision.
	// +optional
	ModelRevision string `json:"modelRevision,omitempty"`

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

	// ConditionKVTransferConfigured indicates that a distributed KV connector
	// is declared and rendered into the runtime configuration. It does not
	// claim that the connector is reachable or producing cache hits.
	ConditionKVTransferConfigured = "KVTransferConfigured"

	// ConditionPrefillReady indicates that all declared prefill replicas are ready.
	ConditionPrefillReady = "PrefillReady"
)
