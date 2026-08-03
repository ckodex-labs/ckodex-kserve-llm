/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// AcceleratorType selects a hardware accelerator vendor.
// +kubebuilder:validation:Enum=nvidia
type AcceleratorType string

const AcceleratorTypeNVIDIA AcceleratorType = "nvidia"

// AcceleratorSpec requests matching accelerator resources for a specialized
// inference runtime.
type AcceleratorSpec struct {
	// Type is the accelerator vendor.
	Type AcceleratorType `json:"type"`
	// Count defaults to one device.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Count *int32 `json:"count,omitempty"`
}
