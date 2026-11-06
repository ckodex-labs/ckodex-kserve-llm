/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	"strings"
)

// AcceleratorType selects a hardware accelerator vendor or platform family.
// +kubebuilder:validation:Enum=nvidia;rocm;apple-mlx;apple-metal;google-tpu;aws-neuron;intel-gaudi;cpu;npu
type AcceleratorType string

const (
	// AcceleratorTypeNVIDIA identifies NVIDIA GPUs (CUDA/NVLink/Ada/Blackwell).
	AcceleratorTypeNVIDIA AcceleratorType = "nvidia"
	// AcceleratorTypeROCm identifies AMD ROCm GPUs (CDNA/Instinct).
	AcceleratorTypeROCm AcceleratorType = "rocm"
	// AcceleratorTypeAppleMLX identifies Apple Silicon accelerators (Unified Memory/Metal/MLX).
	AcceleratorTypeAppleMLX AcceleratorType = "apple-mlx"
	// AcceleratorTypeAppleMetal identifies Apple Silicon Metal accelerators.
	AcceleratorTypeAppleMetal AcceleratorType = "apple-metal"
	// AcceleratorTypeGoogleTPU identifies Google Cloud TPUs (v4, v5e, v5p, v6e/Trillium).
	AcceleratorTypeGoogleTPU AcceleratorType = "google-tpu"
	// AcceleratorTypeAWSNeuron identifies AWS Trainium and Inferentia ASIC accelerators.
	AcceleratorTypeAWSNeuron AcceleratorType = "aws-neuron"
	// AcceleratorTypeIntelGaudi identifies Intel Habana Gaudi accelerators (Gaudi2, Gaudi3).
	AcceleratorTypeIntelGaudi AcceleratorType = "intel-gaudi"
	// AcceleratorTypeCPU identifies host CPU/RAM execution (AVX-512/AMX).
	AcceleratorTypeCPU AcceleratorType = "cpu"
	// AcceleratorTypeNPU identifies dedicated Neural Processing Units.
	AcceleratorTypeNPU AcceleratorType = "npu"
)

// AcceleratorModel identifies a specific accelerator hardware model or architecture profile.
// +kubebuilder:validation:Enum=rtx-pro-6000;dgx-spark;sm100;sm103;gb300;mi300x;mi325x;tpu-v4;tpu-v5e;tpu-v5p;tpu-v6e;trn1;trn2;inf2;gaudi2;gaudi3;m3-ultra;m4-ultra
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
	// AcceleratorModelGB300 identifies NVIDIA GB300 Grace Blackwell Ultra Desktop Superchip.
	AcceleratorModelGB300 AcceleratorModel = "gb300"
	// AcceleratorModelMI300X identifies AMD Instinct MI300X accelerator.
	AcceleratorModelMI300X AcceleratorModel = "mi300x"
	// AcceleratorModelMI325X identifies AMD Instinct MI325X accelerator.
	AcceleratorModelMI325X AcceleratorModel = "mi325x"
	// AcceleratorModelTPUv4 identifies Google Cloud TPU v4 accelerator.
	AcceleratorModelTPUv4 AcceleratorModel = "tpu-v4"
	// AcceleratorModelTPUv5e identifies Google Cloud TPU v5e accelerator.
	AcceleratorModelTPUv5e AcceleratorModel = "tpu-v5e"
	// AcceleratorModelTPUv5p identifies Google Cloud TPU v5p accelerator.
	AcceleratorModelTPUv5p AcceleratorModel = "tpu-v5p"
	// AcceleratorModelTPUv6e identifies Google Cloud TPU v6e (Trillium) accelerator.
	AcceleratorModelTPUv6e AcceleratorModel = "tpu-v6e"
	// AcceleratorModelTrn1 identifies AWS Trainium1 accelerator.
	AcceleratorModelTrn1 AcceleratorModel = "trn1"
	// AcceleratorModelTrn2 identifies AWS Trainium2 accelerator.
	AcceleratorModelTrn2 AcceleratorModel = "trn2"
	// AcceleratorModelInf2 identifies AWS Inferentia2 accelerator.
	AcceleratorModelInf2 AcceleratorModel = "inf2"
	// AcceleratorModelGaudi2 identifies Intel Habana Gaudi2 accelerator.
	AcceleratorModelGaudi2 AcceleratorModel = "gaudi2"
	// AcceleratorModelGaudi3 identifies Intel Habana Gaudi3 accelerator.
	AcceleratorModelGaudi3 AcceleratorModel = "gaudi3"
	// AcceleratorModelM3Ultra identifies Apple Silicon M3 Ultra accelerator.
	AcceleratorModelM3Ultra AcceleratorModel = "m3-ultra"
	// AcceleratorModelM4Ultra identifies Apple Silicon M4 Ultra accelerator.
	AcceleratorModelM4Ultra AcceleratorModel = "m4-ultra"
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
		return normalizeModel(string(s.Model))
	}
	return s.Type.Model()
}

// normalizeModel normalizes common model aliases to canonical AcceleratorModel values.
func normalizeModel(m string) AcceleratorModel {
	m = strings.ToLower(m)
	switch m {
	case "v4":
		return AcceleratorModelTPUv4
	case "v5e":
		return AcceleratorModelTPUv5e
	case "v5p":
		return AcceleratorModelTPUv5p
	case "v6e":
		return AcceleratorModelTPUv6e
	case "gaudi-2":
		return AcceleratorModelGaudi2
	case "gaudi-3":
		return AcceleratorModelGaudi3
	case "trn-1":
		return AcceleratorModelTrn1
	case "trn-2":
		return AcceleratorModelTrn2
	case "inf-2":
		return AcceleratorModelInf2
	case "m3ultra":
		return AcceleratorModelM3Ultra
	case "m4ultra":
		return AcceleratorModelM4Ultra
	default:
		return AcceleratorModel(m)
	}
}

// Vendor extracts the vendor portion from an AcceleratorType.
func (t AcceleratorType) Vendor() AcceleratorType {
	raw := string(t)
	rawLower := strings.ToLower(raw)

	if slash := strings.Index(rawLower, "/"); slash != -1 {
		prefix := rawLower[:slash]
		switch prefix {
		case "google", "google-tpu":
			return AcceleratorTypeGoogleTPU
		case "aws", "aws-neuron":
			return AcceleratorTypeAWSNeuron
		case "intel", "intel-gaudi":
			return AcceleratorTypeIntelGaudi
		case "apple", "apple-metal":
			return AcceleratorTypeAppleMetal
		case "apple-mlx":
			return AcceleratorTypeAppleMLX
		case "nvidia":
			return AcceleratorTypeNVIDIA
		case "rocm":
			return AcceleratorTypeROCm
		case "cpu":
			return AcceleratorTypeCPU
		case "npu":
			return AcceleratorTypeNPU
		default:
			return AcceleratorType(prefix)
		}
	}

	// Exact vendor name or alias
	switch rawLower {
	case "google", "google-tpu":
		return AcceleratorTypeGoogleTPU
	case "aws", "aws-neuron":
		return AcceleratorTypeAWSNeuron
	case "intel", "intel-gaudi":
		return AcceleratorTypeIntelGaudi
	case "apple", "apple-metal":
		return AcceleratorTypeAppleMetal
	case "apple-mlx":
		return AcceleratorTypeAppleMLX
	case "nvidia":
		return AcceleratorTypeNVIDIA
	case "rocm":
		return AcceleratorTypeROCm
	case "cpu":
		return AcceleratorTypeCPU
	case "npu":
		return AcceleratorTypeNPU
	}

	// Hyphenated prefix matching
	prefixes := []struct {
		prefix string
		vendor AcceleratorType
	}{
		{"google-tpu-", AcceleratorTypeGoogleTPU},
		{"aws-neuron-", AcceleratorTypeAWSNeuron},
		{"intel-gaudi-", AcceleratorTypeIntelGaudi},
		{"apple-metal-", AcceleratorTypeAppleMetal},
		{"apple-mlx-", AcceleratorTypeAppleMLX},
		{"nvidia-", AcceleratorTypeNVIDIA},
		{"rocm-", AcceleratorTypeROCm},
		{"google-", AcceleratorTypeGoogleTPU},
		{"aws-", AcceleratorTypeAWSNeuron},
		{"intel-", AcceleratorTypeIntelGaudi},
		{"apple-", AcceleratorTypeAppleMetal},
	}
	for _, p := range prefixes {
		if strings.HasPrefix(rawLower, p.prefix) {
			return p.vendor
		}
	}

	return t
}

// Model extracts the model profile if encoded within the AcceleratorType.
func (t AcceleratorType) Model() AcceleratorModel {
	raw := string(t)
	rawLower := strings.ToLower(raw)

	if slash := strings.Index(rawLower, "/"); slash != -1 {
		return normalizeModel(rawLower[slash+1:])
	}

	// If it's just a vendor name without a model, return empty
	switch rawLower {
	case "google", "google-tpu", "aws", "aws-neuron", "intel", "intel-gaudi",
		"apple", "apple-metal", "apple-mlx", "nvidia", "rocm", "cpu", "npu":
		return ""
	}

	prefixes := []string{
		"google-tpu-",
		"aws-neuron-",
		"intel-gaudi-",
		"apple-metal-",
		"apple-mlx-",
		"nvidia-",
		"rocm-",
		"google-",
		"aws-",
		"intel-",
		"apple-",
	}
	for _, pStr := range prefixes {
		if strings.HasPrefix(rawLower, pStr) {
			return normalizeModel(rawLower[len(pStr):])
		}
	}
	return ""
}
