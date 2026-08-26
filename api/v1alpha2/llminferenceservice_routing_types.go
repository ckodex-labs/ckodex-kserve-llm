/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import corev1 "k8s.io/api/core/v1"

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

// WorkerSpec configures worker nodes for multi-node distributed inference.
// Uses PodTemplateSpec to support serviceAccountName, labels, and annotations.
type WorkerSpec struct {
	// Template defines the pod template for worker pods in LeaderWorkerSet.
	Template corev1.PodTemplateSpec `json:"template"`
}

// RouterSpec configures traffic routing and scheduling.

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

// ManagedGatewaySpec configures a managed Gateway.
type ManagedGatewaySpec struct {
	// GatewayClassName is the name of the GatewayClass to use.
	// +kubebuilder:default="envoy"
	// +optional
	GatewayClassName string `json:"gatewayClassName,omitempty"`
}

// GatewayRef references an existing Gateway.

// GatewayRef references an existing Gateway.
type GatewayRef struct {
	// Name of the Gateway.
	Name string `json:"name"`

	// Namespace of the Gateway.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RouteSpec configures HTTPRoute creation.

// RouteSpec configures HTTPRoute creation.
type RouteSpec struct {
	// HTTPRoute configures HTTP routing.
	// +optional
	HTTPRoute *HTTPRouteSpec `json:"httpRoute,omitempty"`
}

// HTTPRouteSpec configures HTTPRoute details.

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

// InferencePoolSpec configures the InferencePool.
type InferencePoolSpec struct {
	// Selector is the label selector for inference workload pods.
	// Defaults to app.kubernetes.io/component=llminferenceservice-workload.
	// +optional
	Selector map[string]string `json:"selector,omitempty"`
}

// SchedulerConfigSpec defines how to provide the scheduler config.

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

// ConfigReference references an LLMInferenceServiceConfig for composition.
type ConfigReference struct {
	// Name of the LLMInferenceServiceConfig.
	Name string `json:"name"`
}

// ----- Status -----

// LLMInferenceServiceStatus defines the observed state of LLMInferenceService.
