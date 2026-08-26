/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// ObservabilitySpec configures telemetry for an inference service.
type ObservabilitySpec struct {
	// Sink select the telemetry destination for Vector and Audit signals.
	// +optional
	Sink *TelemetrySink `json:"sink,omitempty"`
}

// TelemetrySink defines a destination for OIS signals.

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
