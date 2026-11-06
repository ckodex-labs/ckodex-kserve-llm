/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package accelerator models heterogeneous hardware accelerators and topologies
// for distributed LLM inference workloads.
package accelerator

import (
	"errors"
	"fmt"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// HardwareProfile defines the immutable hardware characteristics of an accelerator device.
type HardwareProfile struct {
	Vendor                 servingv1alpha2.AcceleratorType
	Model                  servingv1alpha2.AcceleratorModel
	Architecture           string
	ComputeCapability      string
	VRAMPerDeviceBytes     int64
	SupportsTensorParallel bool
	SupportsExpertParallel bool
	Interconnect           string
	SupportedQuantizations []string
}

const (
	gibibyte int64 = 1024 * 1024 * 1024
)

// Well-known hardware profiles across heterogeneous fleet accelerators.
var (
	// ProfileRTXPRO6000 defines the NVIDIA RTX PRO 6000 (Ada Lovelace 48GB GDDR6 ECC).
	ProfileRTXPRO6000 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeNVIDIA,
		Model:                  servingv1alpha2.AcceleratorModelRTXPRO6000,
		Architecture:           "Ada Lovelace",
		ComputeCapability:      "sm_89",
		VRAMPerDeviceBytes:     48 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "PCIe-5.0",
		SupportedQuantizations: []string{"exl3", "exl2", "fp8", "awq", "gptq", "bitsandbytes", "bf16", "fp16"},
	}

	// ProfileDGXSpark defines the NVIDIA DGX Spark cluster profile.
	ProfileDGXSpark = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeNVIDIA,
		Model:                  servingv1alpha2.AcceleratorModelDGXSpark,
		Architecture:           "Blackwell Spark",
		ComputeCapability:      "sm_100",
		VRAMPerDeviceBytes:     144 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "NVLink-5",
		SupportedQuantizations: []string{"exl3", "nvfp4", "fp4", "mxfp8", "fp8", "awq", "gptq", "bitsandbytes", "bf16", "fp16"},
	}

	// ProfileSM100 defines the NVIDIA SM100 Blackwell architecture (B100/B200).
	ProfileSM100 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeNVIDIA,
		Model:                  servingv1alpha2.AcceleratorModelSM100,
		Architecture:           "Blackwell",
		ComputeCapability:      "sm_100",
		VRAMPerDeviceBytes:     192 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "NVLink-5",
		SupportedQuantizations: []string{"exl3", "nvfp4", "fp4", "mxfp8", "fp8", "awq", "gptq", "bitsandbytes", "bf16", "fp16"},
	}

	// ProfileSM103 defines the NVIDIA SM103 Blackwell Ultra architecture.
	ProfileSM103 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeNVIDIA,
		Model:                  servingv1alpha2.AcceleratorModelSM103,
		Architecture:           "Blackwell Ultra",
		ComputeCapability:      "sm_103",
		VRAMPerDeviceBytes:     288 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "NVLink-5",
		SupportedQuantizations: []string{"exl3", "nvfp4", "fp4", "mxfp8", "fp8", "awq", "gptq", "bitsandbytes", "bf16", "fp16"},
	}

	// ProfileGB300 defines NVIDIA GB300 Grace Blackwell Ultra Desktop Superchip.
	// Grace 72-core Neoverse V2 CPU with 496 GB LPDDR5X at 396 GB/s,
	// Blackwell Ultra GPU with 252 GB HBM3E at 7.1 TB/s,
	// connected via 900 GB/s NVLink-C2C, PCIe Gen5 slots, and 2x 400 Gb/s ConnectX-8 SuperNIC.
	ProfileGB300 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeNVIDIA,
		Model:                  servingv1alpha2.AcceleratorModelGB300,
		Architecture:           "Blackwell Ultra Superchip",
		ComputeCapability:      "sm_103",
		VRAMPerDeviceBytes:     252 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "NVLink-C2C",
		SupportedQuantizations: []string{"fp4", "nvfp4", "fp8", "bf16", "exl3", "mxfp8", "awq", "gptq", "bitsandbytes", "fp16"},
	}

	// ProfileROCmMI300X defines the AMD ROCm CDNA3 Instinct MI300X accelerator.
	ProfileROCmMI300X = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeROCm,
		Model:                  servingv1alpha2.AcceleratorModelMI300X,
		Architecture:           "CDNA3",
		ComputeCapability:      "gfx942",
		VRAMPerDeviceBytes:     192 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "Infinity-Fabric",
		SupportedQuantizations: []string{"fp8", "awq", "gptq", "bitsandbytes", "bf16", "fp16"},
	}

	// ProfileROCmMI325X defines the AMD ROCm CDNA3 Instinct MI325X accelerator.
	ProfileROCmMI325X = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeROCm,
		Model:                  servingv1alpha2.AcceleratorModelMI325X,
		Architecture:           "CDNA3",
		ComputeCapability:      "gfx942",
		VRAMPerDeviceBytes:     256 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "Infinity-Fabric",
		SupportedQuantizations: []string{"fp8", "awq", "gptq", "bitsandbytes", "bf16", "fp16"},
	}

	// ProfileAppleM4Ultra defines Apple Silicon M4 Ultra unified memory execution.
	ProfileAppleM4Ultra = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeAppleMetal,
		Model:                  servingv1alpha2.AcceleratorModelM4Ultra,
		Architecture:           "Apple Silicon UMA",
		ComputeCapability:      "metal-3",
		VRAMPerDeviceBytes:     192 * gibibyte,
		SupportsTensorParallel: false,
		SupportsExpertParallel: false,
		Interconnect:           "UltraFusion-UMA",
		SupportedQuantizations: []string{"gguf", "mlx", "fp16", "bf16"},
	}

	// ProfileAppleM3Ultra defines Apple Silicon M3 Ultra unified memory execution.
	ProfileAppleM3Ultra = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeAppleMetal,
		Model:                  servingv1alpha2.AcceleratorModelM3Ultra,
		Architecture:           "Apple Silicon UMA",
		ComputeCapability:      "metal-3",
		VRAMPerDeviceBytes:     128 * gibibyte,
		SupportsTensorParallel: false,
		SupportsExpertParallel: false,
		Interconnect:           "UltraFusion-UMA",
		SupportedQuantizations: []string{"gguf", "mlx", "fp16", "bf16"},
	}

	// ProfileAppleMLX defines Apple Silicon unified memory execution.
	ProfileAppleMLX = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeAppleMLX,
		Model:                  "m-series-ultra",
		Architecture:           "Apple Silicon",
		ComputeCapability:      "metal-3",
		VRAMPerDeviceBytes:     128 * gibibyte,
		SupportsTensorParallel: false,
		SupportsExpertParallel: false,
		Interconnect:           "Unified-Memory",
		SupportedQuantizations: []string{"mlx", "bf16", "fp16"},
	}

	// ProfileGoogleTPUv6e defines Google Cloud TPU v6e (Trillium) accelerator.
	ProfileGoogleTPUv6e = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeGoogleTPU,
		Model:                  servingv1alpha2.AcceleratorModelTPUv6e,
		Architecture:           "Trillium",
		ComputeCapability:      "tpu-v6e",
		VRAMPerDeviceBytes:     32 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "ICI-Torus",
		SupportedQuantizations: []string{"bf16", "fp8", "int8"},
	}

	// ProfileGoogleTPUv5p defines Google Cloud TPU v5p accelerator.
	ProfileGoogleTPUv5p = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeGoogleTPU,
		Model:                  servingv1alpha2.AcceleratorModelTPUv5p,
		Architecture:           "TPU-v5p",
		ComputeCapability:      "tpu-v5p",
		VRAMPerDeviceBytes:     96 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "ICI-Torus",
		SupportedQuantizations: []string{"bf16", "fp8"},
	}

	// ProfileGoogleTPUv5e defines Google Cloud TPU v5e (Lite) accelerator.
	ProfileGoogleTPUv5e = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeGoogleTPU,
		Model:                  servingv1alpha2.AcceleratorModelTPUv5e,
		Architecture:           "TPU-v5e",
		ComputeCapability:      "tpu-v5e",
		VRAMPerDeviceBytes:     16 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "ICI-2D-Torus",
		SupportedQuantizations: []string{"bf16", "int8"},
	}

	// ProfileGoogleTPUv4 defines Google Cloud TPU v4 accelerator.
	ProfileGoogleTPUv4 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeGoogleTPU,
		Model:                  servingv1alpha2.AcceleratorModelTPUv4,
		Architecture:           "TPU-v4",
		ComputeCapability:      "tpu-v4",
		VRAMPerDeviceBytes:     32 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "ICI-3D-Torus",
		SupportedQuantizations: []string{"bf16", "int8"},
	}

	// ProfileAWSTrainium2 defines AWS Trainium2 accelerator.
	ProfileAWSTrainium2 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeAWSNeuron,
		Model:                  servingv1alpha2.AcceleratorModelTrn2,
		Architecture:           "NeuronCore-v2",
		ComputeCapability:      "trn2",
		VRAMPerDeviceBytes:     96 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "NeuronLink-v2",
		SupportedQuantizations: []string{"bf16", "fp8"},
	}

	// ProfileAWSTrainium1 defines AWS Trainium1 accelerator.
	ProfileAWSTrainium1 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeAWSNeuron,
		Model:                  servingv1alpha2.AcceleratorModelTrn1,
		Architecture:           "NeuronCore-v2",
		ComputeCapability:      "trn1",
		VRAMPerDeviceBytes:     32 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "NeuronLink-v2",
		SupportedQuantizations: []string{"bf16", "fp16", "fp8"},
	}

	// ProfileAWSInferentia2 defines AWS Inferentia2 accelerator.
	ProfileAWSInferentia2 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeAWSNeuron,
		Model:                  servingv1alpha2.AcceleratorModelInf2,
		Architecture:           "NeuronCore-v2",
		ComputeCapability:      "inf2",
		VRAMPerDeviceBytes:     32 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "NeuronLink-v2",
		SupportedQuantizations: []string{"bf16", "fp16", "fp8"},
	}

	// ProfileIntelGaudi3 defines Intel Habana Gaudi3 accelerator.
	ProfileIntelGaudi3 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeIntelGaudi,
		Model:                  servingv1alpha2.AcceleratorModelGaudi3,
		Architecture:           "G3-DualDie",
		ComputeCapability:      "gaudi3",
		VRAMPerDeviceBytes:     128 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "Integrated-200GbE-RoCE",
		SupportedQuantizations: []string{"fp8", "bf16"},
	}

	// ProfileIntelGaudi2 defines Intel Habana Gaudi2 accelerator.
	ProfileIntelGaudi2 = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeIntelGaudi,
		Model:                  servingv1alpha2.AcceleratorModelGaudi2,
		Architecture:           "Gaudi2",
		ComputeCapability:      "gaudi2",
		VRAMPerDeviceBytes:     96 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "Integrated-100GbE-RoCE",
		SupportedQuantizations: []string{"bf16", "fp8"},
	}

	// ProfileHostCPU defines host CPU AVX-512/AMX execution.
	ProfileHostCPU = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeCPU,
		Model:                  "host-cpu",
		Architecture:           "x86_64",
		ComputeCapability:      "avx512",
		VRAMPerDeviceBytes:     64 * gibibyte,
		SupportsTensorParallel: false,
		SupportsExpertParallel: false,
		Interconnect:           "QPI/UPI",
		SupportedQuantizations: []string{"gguf", "bf16", "fp32"},
	}

	// ProfileGenericNPU defines dedicated neural processing units.
	ProfileGenericNPU = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeNPU,
		Model:                  "generic-npu",
		Architecture:           "NPU",
		ComputeCapability:      "npu-v1",
		VRAMPerDeviceBytes:     32 * gibibyte,
		SupportsTensorParallel: false,
		SupportsExpertParallel: false,
		Interconnect:           "PCIe-4.0",
		SupportedQuantizations: []string{"int8", "fp16"},
	}
)

var defaultFleetProfiles = []HardwareProfile{
	ProfileRTXPRO6000,
	ProfileDGXSpark,
	ProfileSM100,
	ProfileSM103,
	ProfileGB300,
	ProfileROCmMI300X,
	ProfileROCmMI325X,
	ProfileAppleM4Ultra,
	ProfileAppleM3Ultra,
	ProfileAppleMLX,
	ProfileGoogleTPUv6e,
	ProfileGoogleTPUv5p,
	ProfileGoogleTPUv5e,
	ProfileGoogleTPUv4,
	ProfileAWSTrainium2,
	ProfileAWSTrainium1,
	ProfileAWSInferentia2,
	ProfileIntelGaudi3,
	ProfileIntelGaudi2,
	ProfileHostCPU,
	ProfileGenericNPU,
}

// LookupProfile finds a matching HardwareProfile by vendor and model.
func LookupProfile(vendor servingv1alpha2.AcceleratorType, model servingv1alpha2.AcceleratorModel) (HardwareProfile, error) {
	normVendor := strings.ToLower(string(vendor.Vendor()))
	normModel := strings.ToLower(string(model))

	// Normalize model aliases if raw short forms are passed directly
	switch normModel {
	case "v4":
		normModel = string(servingv1alpha2.AcceleratorModelTPUv4)
	case "v5e":
		normModel = string(servingv1alpha2.AcceleratorModelTPUv5e)
	case "v5p":
		normModel = string(servingv1alpha2.AcceleratorModelTPUv5p)
	case "v6e":
		normModel = string(servingv1alpha2.AcceleratorModelTPUv6e)
	case "gaudi-2":
		normModel = string(servingv1alpha2.AcceleratorModelGaudi2)
	case "gaudi-3":
		normModel = string(servingv1alpha2.AcceleratorModelGaudi3)
	case "trn-1":
		normModel = string(servingv1alpha2.AcceleratorModelTrn1)
	case "trn-2":
		normModel = string(servingv1alpha2.AcceleratorModelTrn2)
	case "inf-2":
		normModel = string(servingv1alpha2.AcceleratorModelInf2)
	case "m3ultra":
		normModel = string(servingv1alpha2.AcceleratorModelM3Ultra)
	case "m4ultra":
		normModel = string(servingv1alpha2.AcceleratorModelM4Ultra)
	}

	for _, profile := range defaultFleetProfiles {
		pVendor := strings.ToLower(string(profile.Vendor))
		pModel := strings.ToLower(string(profile.Model))

		if normModel != "" {
			if pModel == normModel && (normVendor == "" || pVendor == normVendor || isAppleFamily(pVendor, normVendor)) {
				return profile, nil
			}
			continue
		}
		if normModel == "" && (pVendor == normVendor || isAppleFamily(pVendor, normVendor)) {
			return profile, nil
		}
	}

	return HardwareProfile{}, fmt.Errorf("no accelerator profile registered for vendor %q, model %q", vendor, model)
}

func isAppleFamily(v1, v2 string) bool {
	apple := func(v string) bool {
		return v == string(servingv1alpha2.AcceleratorTypeAppleMLX) || v == string(servingv1alpha2.AcceleratorTypeAppleMetal)
	}
	return apple(v1) && apple(v2)
}

// HardwareTopology represents the concrete physical or virtual hardware configuration.
type HardwareTopology struct {
	Profile     HardwareProfile
	DeviceCount int32
	TotalVRAM   int64
}

// TotalVRAMGiB returns the total available VRAM in GiB.
func (h HardwareTopology) TotalVRAMGiB() float64 {
	return float64(h.TotalVRAM) / float64(gibibyte)
}

// NewHardwareTopology builds a HardwareTopology for a specified vendor, model, and device count.
func NewHardwareTopology(vendor servingv1alpha2.AcceleratorType, model servingv1alpha2.AcceleratorModel, count int32) (HardwareTopology, error) {
	if count <= 0 {
		return HardwareTopology{}, errors.New("device count must be at least 1")
	}

	profile, err := LookupProfile(vendor, model)
	if err != nil {
		return HardwareTopology{}, fmt.Errorf("resolve hardware profile: %w", err)
	}

	totalVRAM := int64(count) * profile.VRAMPerDeviceBytes
	return HardwareTopology{
		Profile:     profile,
		DeviceCount: count,
		TotalVRAM:   totalVRAM,
	}, nil
}

// NewHardwareTopologyFromSpec builds a HardwareTopology directly from an AcceleratorSpec.
func NewHardwareTopologyFromSpec(spec *servingv1alpha2.AcceleratorSpec) (HardwareTopology, error) {
	if spec == nil {
		return HardwareTopology{}, errors.New("accelerator spec is nil")
	}

	count := int32(1)
	if spec.Count != nil {
		count = *spec.Count
	}

	vendor := spec.Vendor()
	model := spec.ResolvedModel()

	return NewHardwareTopology(vendor, model, count)
}

// ModelWorkload encapsulates the compute and memory requirements of a specific model configuration.
type ModelWorkload struct {
	Name                 string
	ParameterCount       int64
	Quantization         string
	RequiredVRAMBytes    int64
	TensorParallelDegree int32
	RequiresExpert       bool
}

// RequiredVRAMGiB returns the required VRAM in GiB.
func (m ModelWorkload) RequiredVRAMGiB() float64 {
	return float64(m.RequiredVRAMBytes) / float64(gibibyte)
}

// GLM53EXL3Workload returns the standard requirements for GLM-5.3 753B MoE EXL3.
// Quantized via ExLlamaV3 3-bit, requiring TP=4 and 176 GiB VRAM footprint
// (44 GiB per GPU across 4x 48 GiB RTX PRO 6000 devices, leaving 16 GiB headroom).
func GLM53EXL3Workload() ModelWorkload {
	return ModelWorkload{
		Name:                 "GLM-5.3-753B-MoE-EXL3",
		ParameterCount:       753_000_000_000,
		Quantization:         "exl3",
		RequiredVRAMBytes:    176 * gibibyte,
		TensorParallelDegree: 4,
		RequiresExpert:       true,
	}
}
