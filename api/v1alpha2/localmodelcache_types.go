/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=lmc

// LocalModelCache pre-downloads model weights to node-local PVCs
// so that inference pods can mount them without download latency.
type LocalModelCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LocalModelCacheSpec   `json:"spec,omitempty"`
	Status LocalModelCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LocalModelCacheList contains a list of LocalModelCache.
type LocalModelCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LocalModelCache `json:"items"`
}

// LocalModelCacheSpec defines the desired state of LocalModelCache.
type LocalModelCacheSpec struct {
	// SourceModelURI is the model artifact to cache.
	// Supported schemes: hf://, s3://, swfs://, gs://, pvc://, oci://, ocis://.
	// +kubebuilder:validation:Pattern=`^(hf|s3|swfs|gs|pvc|oci|ocis|https?)://.*$`
	SourceModelURI string `json:"sourceModelUri"`

	// ModelSize is the expected model size for PVC provisioning.
	// +optional
	ModelSize string `json:"modelSize,omitempty"`

	// MaxCacheSize is the maximum total cache size across all nodes.
	// When the sum of cached PVCs exceeds this value, the least-recently-used
	// PVCs are evicted. Uses Kubernetes quantity format (e.g., "200Gi").
	// +optional
	MaxCacheSize string `json:"maxCacheSize,omitempty"`

	// NodeGroup selects which nodes should cache this model.
	// +optional
	NodeGroup *NodeGroupSpec `json:"nodeGroup,omitempty"`

	// WarmNodes is a list of node names to pre-warm with cached model data.
	// PVCs and storage initializer Jobs are created on these nodes before
	// any LLMInferenceService references the model. Warm-up Jobs follow the
	// same "Guaranteed QoS" and 30s termination grace period patterns for
	// optimal scheduling.
	// +optional
	WarmNodes []string `json:"warmNodes,omitempty"`

	// Storage configures credentials for model download.
	// +optional
	Storage *LocalModelStorageSpec `json:"storage,omitempty"`

	// AllowedNamespaces is the list of namespaces permitted to reference this
	// LocalModelCache in LLMInferenceService.spec.model.uri (pvc:// scheme) or via
	// BaseRefs. Empty list = cluster-wide access (backward compatible default).
	// Enforced by the LocalModelCacheReconciler before creating warm-up Jobs.
	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`

	// Env defines custom environment variables for the warm-up pod.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// StorageClassName is the StorageClass to use for cache PVCs.
	// If empty, the cluster default StorageClass is used.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// NodeGroupSpec selects nodes for model caching.
type NodeGroupSpec struct {
	// LabelSelector selects nodes by label.
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}

// LocalModelStorageSpec configures storage credentials.
type LocalModelStorageSpec struct {
	// SecretName references a Secret with storage credentials.
	// The Secret must contain keys appropriate for the storage backend.
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// ServiceAccountName references a ServiceAccount with
	// storage access (e.g., for GCS or S3 via IRSA).
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// StorageContainerRef references a ClusterStorageContainer
	// for custom storage initializer configuration.
	// +optional
	StorageContainerRef string `json:"storageContainerRef,omitempty"`
}

// LocalModelCacheStatus defines the observed state.
type LocalModelCacheStatus struct {
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// NodeStatuses tracks per-node download state.
	// +optional
	NodeStatuses []NodeCacheStatus `json:"nodeStatuses,omitempty"`

	// CachedNodes is the count of nodes with a complete local cache.
	CachedNodes int32 `json:"cachedNodes"`

	// CachedModels tracks which models are cached, where, and when last used.
	// +optional
	CachedModels []CachedModelStatus `json:"cachedModels,omitempty"`

	// TotalCacheSize is the sum of all cached PVC sizes.
	// +optional
	TotalCacheSize string `json:"totalCacheSize,omitempty"`

	// AvailableSpace is MaxCacheSize minus TotalCacheSize.
	// Empty if MaxCacheSize is not set.
	// +optional
	AvailableSpace string `json:"availableSpace,omitempty"`
}

// CachedModelStatus tracks a single cached model across nodes.
type CachedModelStatus struct {
	// ModelURI is the source model URI.
	ModelURI string `json:"modelUri"`

	// NodeNames lists nodes where this model is cached.
	NodeNames []string `json:"nodeNames"`

	// SizeBytes is the size of the cached model in bytes.
	// +optional
	SizeBytes int64 `json:"sizeBytes,omitempty"`

	// LastUsed is when this cached model was last mounted by an inference pod.
	// +optional
	LastUsed *metav1.Time `json:"lastUsed,omitempty"`

	// PVCName is the content-addressable PVC name for this model.
	PVCName string `json:"pvcName"`
}

// NodeCacheStatus tracks model cache status on a single node.
type NodeCacheStatus struct {
	// NodeName identifies the node.
	NodeName string `json:"nodeName"`

	// Phase is the download phase.
	// +kubebuilder:validation:Enum=Pending;Downloading;Ready;Failed
	Phase string `json:"phase"`

	// PVCName is the name of the PVC holding the cached model.
	// +optional
	PVCName string `json:"pvcName,omitempty"`

	// LastTransitionTime is when the phase last changed.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// SizeBytes is the observed size of this cache entry.
	// +optional
	SizeBytes int64 `json:"sizeBytes,omitempty"`

	// LastUsed is when this node cache was last used by an inference pod.
	// +optional
	LastUsed *metav1.Time `json:"lastUsed,omitempty"`

	// ModelURIHash is the content-addressable hash of the model URI.
	// +optional
	ModelURIHash string `json:"modelUriHash,omitempty"`
}

func (s *LocalModelCacheSpec) ModelSizeQuantity() resource.Quantity {
	if s.ModelSize == "" {
		return resource.MustParse("20Gi") // Default for LLM
	}
	return resource.MustParse(s.ModelSize)
}

// MaxCacheSizeQuantity returns the parsed max cache size, or zero if unset.
func (s *LocalModelCacheSpec) MaxCacheSizeQuantity() (resource.Quantity, bool) {
	if s.MaxCacheSize == "" {
		return resource.Quantity{}, false
	}
	return resource.MustParse(s.MaxCacheSize), true
}

func init() {
	SchemeBuilder.Register(&LocalModelCache{}, &LocalModelCacheList{})
}
