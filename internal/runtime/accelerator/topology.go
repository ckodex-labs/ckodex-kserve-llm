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

	// ProfileROCmMI300X defines the AMD ROCm CDNA3 Instinct accelerator.
	ProfileROCmMI300X = HardwareProfile{
		Vendor:                 servingv1alpha2.AcceleratorTypeROCm,
		Model:                  "mi300x",
		Architecture:           "CDNA3",
		ComputeCapability:      "gfx942",
		VRAMPerDeviceBytes:     192 * gibibyte,
		SupportsTensorParallel: true,
		SupportsExpertParallel: true,
		Interconnect:           "Infinity-Fabric",
		SupportedQuantizations: []string{"fp8", "awq", "gptq", "bitsandbytes", "bf16", "fp16"},
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
	ProfileROCmMI300X,
	ProfileAppleMLX,
	ProfileHostCPU,
	ProfileGenericNPU,
}

// LookupProfile finds a matching HardwareProfile by vendor and model.
func LookupProfile(vendor servingv1alpha2.AcceleratorType, model servingv1alpha2.AcceleratorModel) (HardwareProfile, error) {
	normVendor := strings.ToLower(string(vendor.Vendor()))
	normModel := strings.ToLower(string(model))

	for _, profile := range defaultFleetProfiles {
		pVendor := strings.ToLower(string(profile.Vendor))
		pModel := strings.ToLower(string(profile.Model))

		if normModel != "" {
			if pModel == normModel && (normVendor == "" || pVendor == normVendor) {
				return profile, nil
			}
			continue
		}
		if normModel == "" && pVendor == normVendor {
			return profile, nil
		}
	}

	return HardwareProfile{}, fmt.Errorf("no accelerator profile registered for vendor %q, model %q", vendor, model)
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

// VRAMState represents memory capacity sufficiency.
type VRAMState string

const (
	VRAMStateSufficient   VRAMState = "SUFFICIENT"
	VRAMStateDegraded     VRAMState = "DEGRADED"
	VRAMStateInsufficient VRAMState = "INSUFFICIENT"
)

// ParallelismState represents parallel configuration alignment.
type ParallelismState string

const (
	ParallelismStateOptimal     ParallelismState = "OPTIMAL"
	ParallelismStateCompatible  ParallelismState = "COMPATIBLE"
	ParallelismStateMisaligned  ParallelismState = "MISALIGNED"
	ParallelismStateUnsupported ParallelismState = "UNSUPPORTED"
)

// VendorCompatibilityState represents platform and kernel support.
type VendorCompatibilityState string

const (
	VendorCompatibilityNative       VendorCompatibilityState = "NATIVE"
	VendorCompatibilityEmulated     VendorCompatibilityState = "EMULATED"
	VendorCompatibilityIncompatible VendorCompatibilityState = "INCOMPATIBLE"
)

// TopologyVectorState encapsulates multi-dimensional state vectors for hardware governance.
type TopologyVectorState struct {
	VRAM        VRAMState
	Parallelism ParallelismState
	Vendor      VendorCompatibilityState
}

// TopologyPhase represents the aggregate qualification phase.
type TopologyPhase string

const (
	TopologyPhaseCompliant    TopologyPhase = "COMPLIANT"
	TopologyPhaseNonCompliant TopologyPhase = "NON_COMPLIANT"
	TopologyPhaseDegraded     TopologyPhase = "DEGRADED"
)

// TopologyValidationResult contains the full deterministic outcome of topology evaluation.
type TopologyValidationResult struct {
	Phase              TopologyPhase
	Vector             TopologyVectorState
	TotalAvailableVRAM int64
	RequiredVRAM       int64
	HeadroomBytes      int64
	HeadroomRatio      float64
	Reasons            []string
}

// IsCompliant reports whether the hardware topology satisfies the workload requirements.
func (r TopologyValidationResult) IsCompliant() bool {
	return r.Phase == TopologyPhaseCompliant || r.Phase == TopologyPhaseDegraded
}

// ValidateTopology performs multi-dimensional vector validation of a hardware topology against a workload.
func ValidateTopology(topo HardwareTopology, workload ModelWorkload) TopologyValidationResult {
	result := TopologyValidationResult{
		TotalAvailableVRAM: topo.TotalVRAM,
		RequiredVRAM:       workload.RequiredVRAMBytes,
		HeadroomBytes:      topo.TotalVRAM - workload.RequiredVRAMBytes,
		Reasons:            make([]string, 0, 4),
	}

	if topo.TotalVRAM > 0 {
		result.HeadroomRatio = float64(result.HeadroomBytes) / float64(topo.TotalVRAM)
	}

	result.Vector.Vendor = evaluateVendorCompatibility(topo.Profile, workload)
	result.Vector.Parallelism = evaluateParallelism(topo, workload)
	result.Vector.VRAM = evaluateVRAM(result.HeadroomBytes, result.HeadroomRatio)

	finalizeResult(&result, topo, workload)
	return result
}

func evaluateVendorCompatibility(p HardwareProfile, w ModelWorkload) VendorCompatibilityState {
	if w.Quantization == "" {
		return VendorCompatibilityNative
	}
	quantLower := strings.ToLower(w.Quantization)
	for _, supported := range p.SupportedQuantizations {
		if strings.ToLower(supported) == quantLower {
			return VendorCompatibilityNative
		}
	}
	return VendorCompatibilityIncompatible
}

func evaluateParallelism(topo HardwareTopology, w ModelWorkload) ParallelismState {
	tp := w.TensorParallelDegree
	if tp <= 1 {
		return ParallelismStateOptimal
	}

	if !topo.Profile.SupportsTensorParallel {
		return ParallelismStateUnsupported
	}

	if topo.DeviceCount < tp || topo.DeviceCount%tp != 0 {
		return ParallelismStateMisaligned
	}

	if topo.DeviceCount == tp {
		return ParallelismStateOptimal
	}

	return ParallelismStateCompatible
}

func evaluateVRAM(headroomBytes int64, headroomRatio float64) VRAMState {
	if headroomBytes < 0 {
		return VRAMStateInsufficient
	}
	if headroomRatio < 0.05 {
		return VRAMStateDegraded
	}
	return VRAMStateSufficient
}

func finalizeResult(res *TopologyValidationResult, topo HardwareTopology, w ModelWorkload) {
	if res.Vector.Vendor == VendorCompatibilityIncompatible {
		res.Reasons = append(res.Reasons, fmt.Sprintf("hardware %s does not support %s quantization kernels", topo.Profile.Model, w.Quantization))
	}
	if res.Vector.Parallelism == ParallelismStateUnsupported {
		res.Reasons = append(res.Reasons, fmt.Sprintf("accelerator vendor %s lacks tensor parallel interconnect support", topo.Profile.Vendor))
	}
	if res.Vector.Parallelism == ParallelismStateMisaligned {
		res.Reasons = append(res.Reasons, fmt.Sprintf("device count %d is misaligned with required tensor parallel degree %d", topo.DeviceCount, w.TensorParallelDegree))
	}
	if res.Vector.VRAM == VRAMStateInsufficient {
		res.Reasons = append(res.Reasons, fmt.Sprintf("available VRAM (%.2f GiB) is less than required workload VRAM (%.2f GiB)", float64(res.TotalAvailableVRAM)/float64(gibibyte), float64(res.RequiredVRAM)/float64(gibibyte)))
	}
	if res.Vector.VRAM == VRAMStateDegraded {
		res.Reasons = append(res.Reasons, fmt.Sprintf("available VRAM headroom (%.1f%%) is constrained under 5%% KV cache threshold", res.HeadroomRatio*100.0))
	}

	if res.Vector.Vendor == VendorCompatibilityIncompatible ||
		res.Vector.Parallelism == ParallelismStateUnsupported ||
		res.Vector.Parallelism == ParallelismStateMisaligned ||
		res.Vector.VRAM == VRAMStateInsufficient {
		res.Phase = TopologyPhaseNonCompliant
	} else if res.Vector.VRAM == VRAMStateDegraded {
		res.Phase = TopologyPhaseDegraded
	} else {
		res.Phase = TopologyPhaseCompliant
	}
}
