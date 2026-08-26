/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// ModelSpec defines the model to serve.
type ModelSpec struct {
	// URI is the model artifact location.
	// Supported schemes: hf:// (HuggingFace Hub), hf-mount:// (HuggingFace CSI lazy mount),
	// hf-mirror://, s3://, swfs://, seaweedfs://, gs://, pvc://, oci://, ocis://, modelpack://.
	// hf-mount:// uses the hf-csi-driver to mount repos as a FUSE/NFS filesystem —
	// only accessed bytes are fetched, eliminating full model downloads.
	// `ocis://` is an explicit secure-OCI alias and follows the same runtime
	// verification path as `oci://`, while making the intent visible at the spec boundary.
	// +kubebuilder:validation:Pattern=`^(hf|hf-mount|hf-mirror|s3|swfs|seaweedfs|gs|pvc|oci|ocis|modelpack|https?)://.*$`
	URI string `json:"uri"`

	// Revision pins a Hugging Face branch, tag, or commit. It is valid only
	// with hf://, hf-mount://, or hf-mirror:// URIs.
	// +optional
	Revision string `json:"revision,omitempty"`

	// Name is the model identifier used in inference requests.
	// This is the value clients use in the "model" field of chat/completion requests.
	Name string `json:"name"`

	// Storage configures credentials for model download.
	// Mirrors LocalModelCache credential support.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// HardwareAware enables automatic hardware-specific artifact selection.
	// When true, the operator appends hardware suffixes to OCI tags (e.g., -nvidia).
	// Requires ENABLE_EXPERIMENTAL_HARDWARE_SELECTION feature gate.
	// +optional
	HardwareAware bool `json:"hardwareAware,omitempty"`
}

// StorageSpec configures storage credentials for model download.
