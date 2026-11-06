/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package accelerator

import (
	"fmt"
	"strings"
)

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
