/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import corev1 "k8s.io/api/core/v1"

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

// ExternalSecretData defines a single mapping of a remote secret to a local key.
type ExternalSecretData struct {
	// SecretKey is the key in the resulting Kubernetes Secret.
	SecretKey string `json:"secretKey"`

	// RemoteRef defines where to fetch the secret from the provider.
	RemoteRef ExternalSecretRemoteRef `json:"remoteRef"`
}

// ExternalSecretRemoteRef defines the remote key and optional property.

// ExternalSecretRemoteRef defines the remote key and optional property.
type ExternalSecretRemoteRef struct {
	// Key is the name/path of the secret in the external provider.
	Key string `json:"key"`

	// Property is the specific field to extract from the remote secret.
	// +optional
	Property string `json:"property,omitempty"`
}

// ParallelismSpec configures distributed inference parallelism.
