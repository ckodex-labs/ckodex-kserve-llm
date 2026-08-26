/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

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
	// +optional
	ModelReady bool `json:"modelReady,omitempty"`

	// ModelRevision is the declared Hugging Face revision.
	// +optional
	ModelRevision string `json:"modelRevision,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Optimized indicates whether model-specific optimizations were applied.
	// +optional
	Optimized bool `json:"optimized,omitempty"`

	// DetectedHardware is the hardware type identified for this service.
	// +optional
	DetectedHardware string `json:"detectedHardware,omitempty"`

	// AdaptiveMetrics represents real-time performance and load pressure.
	// +optional
	AdaptiveMetrics *AdaptiveMetrics `json:"adaptiveMetrics,omitempty"`
}

// StatePlanes represents orthogonal views of the system's state.
type StatePlanes struct {
	Lifecycle   string `json:"lifecycle,omitempty"`
	Trust       string `json:"trust,omitempty"`
	Binding     string `json:"binding,omitempty"`
	Composition string `json:"composition,omitempty"`
	Risk        string `json:"risk,omitempty"`
}

// AdaptiveMetrics represents real-time performance data for the service.
type AdaptiveMetrics struct {
	P50Latency string `json:"p50Latency,omitempty"`
	P95Latency string `json:"p95Latency,omitempty"`
	P99Latency string `json:"p99Latency,omitempty"`
	QueueDepth int64  `json:"queueDepth,omitempty"`
	LoadLevel  string `json:"loadLevel,omitempty"`
}

// Condition types.
const (
	ConditionReady           = "Ready"
	ConditionDeploymentReady = "DeploymentReady"
	ConditionGatewayReady    = "GatewayReady"
	ConditionSchedulerReady  = "SchedulerReady"
	ConditionModelLoaded     = "ModelLoaded"
)

// SLOSpec declares the service level objectives for an LLMInferenceService.
type SLOSpec struct {
	// TargetP99LatencyMs is the maximum acceptable P99 end-to-end latency in ms.
	// +kubebuilder:validation:Minimum=1
	TargetP99LatencyMs int64 `json:"targetP99LatencyMs"`

	// TargetAvailability is the minimum acceptable availability ratio (0.0–1.0).
	TargetAvailability float64 `json:"targetAvailability"`

	// ErrorBudgetDays is the rolling window (in days) for error budget calculation.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +optional
	ErrorBudgetDays int `json:"errorBudgetDays,omitempty"`
}

// CanarySpec configures progressive traffic splitting for a canary rollout.
type CanarySpec struct {
	// Weight is the percentage of traffic (0–100) routed to this canary service.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Weight int32 `json:"weight"`

	// BaseModel is the name of the stable LLMInferenceService in the same namespace.
	// +kubebuilder:validation:MinLength=1
	BaseModel string `json:"baseModel"`
}
