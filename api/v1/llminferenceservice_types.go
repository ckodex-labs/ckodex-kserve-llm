/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=llmisvc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LLMInferenceService is the stable v1 API for LLM inference workloads.
// It manages the full lifecycle of a served LLM including deployment, routing,
// scheduling, and autoscaling.
//
// Compared to v1alpha2:
//   - spec.prefill and spec.worker are moved to spec.experimental.prefill / spec.experimental.worker
//   - All other fields are stable and forward-compatible
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

	// Replicas is the number of model server replicas.
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
	Template corev1.PodTemplateSpec `json:"template"`

	// Router configures gateway, route, and scheduler for traffic management.
	Router RouterSpec `json:"router"`

	// BaseRefs references LLMInferenceServiceConfig resources for configuration composition.
	// +optional
	BaseRefs []ConfigReference `json:"baseRefs,omitempty"`

	// AutoOptimize enables automatic hardware detection and optimization.
	// +optional
	AutoOptimize *bool `json:"autoOptimize,omitempty"`

	// AllowedTenants is the list of tenant IDs permitted to send requests to this service.
	// Enforced by the LLMModelAccess OPA Gatekeeper constraint.
	// +optional
	AllowedTenants []string `json:"allowedTenants,omitempty"`

	// CostAllocationTags are arbitrary key-value labels propagated to OTel metric
	// attributes, Deployment labels, and KEDA ScaledObject annotations so FinOps
	// tooling can group GPU-second and token costs by team, project, or cost-center.
	// Keys and values must match the Kubernetes label value regex: [a-zA-Z0-9_-./]+
	// +optional
	CostAllocationTags map[string]string `json:"costAllocationTags,omitempty"`

	// SLO defines the service level objective for this inference service.
	// +optional
	SLO *SLOSpec `json:"slo,omitempty"`

	// Canary configures weighted traffic splitting between this (canary) service
	// and a stable base service.
	// +optional
	Canary *CanarySpec `json:"canary,omitempty"`

	// Experimental holds fields that were experimental in v1alpha2 and are retained
	// in v1 for workloads that need them, but are not part of the stable surface.
	// Fields in this sub-struct may change or be removed in future versions.
	// +optional
	Experimental *ExperimentalSpec `json:"experimental,omitempty"`
}

// ExperimentalSpec groups fields that were top-level in v1alpha2 but are not yet
// stable. Moving them here preserves forward compatibility: v1 clients that don't
// use these features are unaffected; v1alpha2 clients continue to work via conversion.
type ExperimentalSpec struct {
	// Prefill configures disaggregated prefill workers.
	// Moved from spec.prefill in v1alpha2.
	// +optional
	Prefill *PrefillSpec `json:"prefill,omitempty"`

	// Worker configures worker nodes for multi-node distributed inference (LeaderWorkerSet).
	// Moved from spec.worker in v1alpha2.
	// +optional
	Worker *WorkerSpec `json:"worker,omitempty"`
}

// ModelSpec defines the model to serve.
type ModelSpec struct {
	// URI is the model artifact location.
	// Supported schemes: hf://, hf-mount://, hf-mirror://, s3://, gs://, pvc://, oci://, ocis://, seaweedfs://.
	// +kubebuilder:validation:Pattern=`^(hf|hf-mount|hf-mirror|s3|swfs|gs|pvc|oci|ocis|modelpack|https?)://.*$`
	URI string `json:"uri"`

	// Name is the model identifier used in inference requests.
	Name string `json:"name"`

	// Storage configures credentials for model download.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`
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
}

// ParallelismSpec configures distributed inference parallelism.
type ParallelismSpec struct {
	// Tensor is the tensor parallelism degree.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Tensor *int32 `json:"tensor,omitempty"`

	// Data is the data parallelism degree.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Data *int32 `json:"data,omitempty"`

	// DataLocal controls GPUs per node for optimal NUMA affinity.
	// +kubebuilder:validation:Minimum=1
	// +optional
	DataLocal *int32 `json:"dataLocal,omitempty"`

	// Expert enables expert parallelism for MoE models.
	// +optional
	Expert bool `json:"expert,omitempty"`
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

// ----- Status -----

// LLMInferenceServiceStatus defines the observed state of LLMInferenceService.
type LLMInferenceServiceStatus struct {
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

	// ObservedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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

func init() {
	SchemeBuilder.Register(&LLMInferenceService{}, &LLMInferenceServiceList{})
}
