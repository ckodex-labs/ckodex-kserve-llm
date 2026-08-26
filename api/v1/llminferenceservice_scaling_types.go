/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

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

	// Pipeline is the pipeline parallelism degree.
	// +optional
	Pipeline *int32 `json:"pipeline,omitempty"`

	// EPLBEnabled enables expert parallelism load balancing.
	// +optional
	EPLBEnabled bool `json:"eplbEnabled,omitempty"`
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
