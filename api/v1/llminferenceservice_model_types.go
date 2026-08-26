/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

import corev1 "k8s.io/api/core/v1"

// ModelSpec defines the model to serve.
type ModelSpec struct {
	// URI is the model artifact location.
	// Supported schemes: hf://, hf-mount://, hf-mirror://, s3://, gs://, pvc://, oci://, ocis://, seaweedfs://.
	// +kubebuilder:validation:Pattern=`^(hf|hf-mount|hf-mirror|s3|swfs|seaweedfs|gs|pvc|oci|ocis|modelpack|https?)://.*$`
	URI string `json:"uri"`

	// Revision pins a Hugging Face branch, tag, or commit. It is valid only
	// with hf://, hf-mount://, or hf-mirror:// URIs.
	// +optional
	Revision string `json:"revision,omitempty"`

	// Name is the model identifier used in inference requests.
	Name string `json:"name"`

	// Storage configures credentials for model download.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// HardwareAware enables hardware-specific artifact selection.
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

	// ExternalSecret configures external-secrets.io credential synchronization.
	// +optional
	ExternalSecret *ExternalSecretSpec `json:"externalSecret,omitempty"`
}

// ExternalSecretSpec defines the desired external secret synchronization.
type ExternalSecretSpec struct {
	SecretStoreRef  SecretStoreRef       `json:"secretStoreRef"`
	RefreshInterval string               `json:"refreshInterval,omitempty"`
	Data            []ExternalSecretData `json:"data,omitempty"`
}

// SecretStoreRef identifies a SecretStore or ClusterSecretStore.
type SecretStoreRef struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

// ExternalSecretData maps a remote provider key to a Kubernetes Secret key.
type ExternalSecretData struct {
	SecretKey string                  `json:"secretKey"`
	RemoteRef ExternalSecretRemoteRef `json:"remoteRef"`
}

// ExternalSecretRemoteRef identifies a remote key and optional property.
type ExternalSecretRemoteRef struct {
	Key      string `json:"key"`
	Property string `json:"property,omitempty"`
}
