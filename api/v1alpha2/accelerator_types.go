/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	"strings"
)

// AcceleratorType selects a hardware accelerator vendor or platform family.
// +kubebuilder:validation:Enum=nvidia;rocm;apple-mlx;cpu;npu
type AcceleratorType string

const (
	// AcceleratorTypeNVIDIA identifies NVIDIA GPUs (CUDA/NVLink/Ada/Blackwell).
	AcceleratorTypeNVIDIA AcceleratorType = "nvidia"
	// AcceleratorTypeROCm identifies AMD ROCm GPUs (CDNA/Instinct).
	AcceleratorTypeROCm AcceleratorType = "rocm"
	// AcceleratorTypeAppleMLX identifies Apple Silicon accelerators (Unified Memory/Metal/MLX).
	AcceleratorTypeAppleMLX AcceleratorType = "apple-mlx"
	// AcceleratorTypeCPU identifies host CPU/RAM execution (AVX-512/AMX).
	AcceleratorTypeCPU AcceleratorType = "cpu"
	// AcceleratorTypeNPU identifies dedicated Neural Processing Units.
	AcceleratorTypeNPU AcceleratorType = "npu"
)

// AcceleratorModel identifies a specific accelerator hardware model or architecture profile.
// +kubebuilder:validation:Enum=rtx-pro-6000;dgx-spark;sm100;sm103
type AcceleratorModel string

const (
	// AcceleratorModelRTXPRO6000 identifies NVIDIA RTX PRO 6000 (48GB GDDR6 Ada Lovelace).
	AcceleratorModelRTXPRO6000 AcceleratorModel = "rtx-pro-6000"
	// AcceleratorModelDGXSpark identifies NVIDIA DGX Spark cluster profile.
	AcceleratorModelDGXSpark AcceleratorModel = "dgx-spark"
	// AcceleratorModelSM100 identifies NVIDIA SM100 Blackwell architecture (B100/B200).
	AcceleratorModelSM100 AcceleratorModel = "sm100"
	// AcceleratorModelSM103 identifies NVIDIA SM103 Blackwell Ultra architecture.
	AcceleratorModelSM103 AcceleratorModel = "sm103"
)

// AcceleratorSpec requests matching accelerator resources for a specialized
// inference runtime.
type AcceleratorSpec struct {
	// Type is the accelerator vendor or platform family.
	Type AcceleratorType `json:"type"`
	// Model optionally identifies a specific hardware model or architecture profile.
	// When omitted, vendor defaults are applied.
	// +optional
	Model AcceleratorModel `json:"model,omitempty"`
	// Count defaults to one device.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Count *int32 `json:"count,omitempty"`
}

// Vendor parses and normalizes the accelerator vendor from the spec.
func (s *AcceleratorSpec) Vendor() AcceleratorType {
	if s == nil {
		return ""
	}
	return s.Type.Vendor()
}

// ResolvedModel returns the model profile from the spec or parsed from the Type.
func (s *AcceleratorSpec) ResolvedModel() AcceleratorModel {
	if s == nil {
		return ""
	}
	if s.Model != "" {
		return s.Model
	}
	return s.Type.Model()
}

// Vendor extracts the vendor portion from an AcceleratorType.
func (t AcceleratorType) Vendor() AcceleratorType {
	raw := string(t)
	if slash := strings.Index(raw, "/"); slash != -1 {
		return AcceleratorType(strings.ToLower(raw[:slash]))
	}
	if dash := strings.Index(raw, "-"); dash != -1 {
		prefix := strings.ToLower(raw[:dash])
		if prefix == string(AcceleratorTypeNVIDIA) || prefix == string(AcceleratorTypeROCm) || prefix == string(AcceleratorTypeCPU) || prefix == string(AcceleratorTypeNPU) {
			return AcceleratorType(prefix)
		}
	}
	return t
}

// Model extracts the model profile if encoded within the AcceleratorType.
func (t AcceleratorType) Model() AcceleratorModel {
	raw := string(t)
	if slash := strings.Index(raw, "/"); slash != -1 {
		return AcceleratorModel(strings.ToLower(raw[slash+1:]))
	}
	if dash := strings.Index(raw, "-"); dash != -1 {
		prefix := strings.ToLower(raw[:dash])
		if prefix == string(AcceleratorTypeNVIDIA) || prefix == string(AcceleratorTypeROCm) {
			return AcceleratorModel(strings.ToLower(raw[dash+1:]))
		}
	}
	return ""
}
