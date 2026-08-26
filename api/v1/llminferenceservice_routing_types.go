/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

import corev1 "k8s.io/api/core/v1"

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
	// +optional
	Scheduler *SchedulerSpec `json:"scheduler,omitempty"`
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

	// Resilience configures route timeouts and retries.
	// +optional
	Resilience *ResilienceSpec `json:"resilience,omitempty"`
}

// ResilienceSpec defines timeout and retry parameters for inference routing.
type ResilienceSpec struct {
	Timeout    string `json:"timeout,omitempty"`
	MaxRetries int32  `json:"maxRetries,omitempty"`
	RetryOn    string `json:"retryOn,omitempty"`
}

// SchedulerSpec configures the KV-cache aware scheduler.
type SchedulerSpec struct {
	// Pool configures the InferencePool resource.
	Pool InferencePoolSpec `json:"pool"`

	// Config configures the EndpointPickerConfig for the scheduler.
	// +optional
	Config *SchedulerConfigSpec `json:"config,omitempty"`

	// Replicas is the number of EPP replicas for HA.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

// InferencePoolSpec configures the InferencePool.
type InferencePoolSpec struct {
	// Selector is the label selector for inference workload pods.
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

// EndpointPickerConfigSpec holds inline scheduler plugin configuration.
type EndpointPickerConfigSpec struct {
	// Plugins is the ordered list of scoring/picking plugins.
	Plugins []string `json:"plugins,omitempty"`
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
