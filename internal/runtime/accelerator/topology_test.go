/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package accelerator

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"k8s.io/utils/ptr"
)

func TestGLM53On4xRTXPRO6000_Compliant(t *testing.T) {
	spec := &servingv1alpha2.AcceleratorSpec{
		Type:  servingv1alpha2.AcceleratorTypeNVIDIA,
		Model: servingv1alpha2.AcceleratorModelRTXPRO6000,
		Count: ptr.To(int32(4)),
	}

	topo, err := NewHardwareTopologyFromSpec(spec)
	if err != nil {
		t.Fatalf("unexpected error creating topology: %v", err)
	}

	if got, want := topo.DeviceCount, int32(4); got != want {
		t.Errorf("DeviceCount = %d, want %d", got, want)
	}
	expectedVRAM := int64(4) * 48 * 1024 * 1024 * 1024
	if got, want := topo.TotalVRAM, expectedVRAM; got != want {
		t.Errorf("TotalVRAM = %d, want %d", got, want)
	}
	if got, want := topo.TotalVRAMGiB(), 192.0; got != want {
		t.Errorf("TotalVRAMGiB = %f, want %f", got, want)
	}

	workload := GLM53EXL3Workload()
	result := ValidateTopology(topo, workload)

	if !result.IsCompliant() {
		t.Errorf("expected compliant topology, got non-compliant. Reasons: %v", result.Reasons)
	}
	if result.Phase != TopologyPhaseCompliant {
		t.Errorf("Phase = %v, want %v", result.Phase, TopologyPhaseCompliant)
	}
	if result.Vector.VRAM != VRAMStateSufficient {
		t.Errorf("VRAMState = %v, want %v", result.Vector.VRAM, VRAMStateSufficient)
	}
	if result.Vector.Parallelism != ParallelismStateOptimal {
		t.Errorf("ParallelismState = %v, want %v", result.Vector.Parallelism, ParallelismStateOptimal)
	}
	if result.Vector.Vendor != VendorCompatibilityNative {
		t.Errorf("VendorState = %v, want %v", result.Vector.Vendor, VendorCompatibilityNative)
	}

	// 192 GiB - 176 GiB = 16 GiB headroom
	expectedHeadroom := int64(16) * 1024 * 1024 * 1024
	if result.HeadroomBytes != expectedHeadroom {
		t.Errorf("HeadroomBytes = %d, want %d", result.HeadroomBytes, expectedHeadroom)
	}
}

func TestGLM53OnInsufficientGPUs_NonCompliant(t *testing.T) {
	// Only 2x RTX PRO 6000 = 96 GiB, cannot fit 185 GiB workload and TP=4 requires 4 GPUs
	spec := &servingv1alpha2.AcceleratorSpec{
		Type:  servingv1alpha2.AcceleratorTypeNVIDIA,
		Model: servingv1alpha2.AcceleratorModelRTXPRO6000,
		Count: ptr.To(int32(2)),
	}

	topo, err := NewHardwareTopologyFromSpec(spec)
	if err != nil {
		t.Fatalf("unexpected error creating topology: %v", err)
	}

	workload := GLM53EXL3Workload()
	result := ValidateTopology(topo, workload)

	if result.IsCompliant() {
		t.Fatal("expected topology to be non-compliant for 2x RTX PRO 6000")
	}
	if result.Phase != TopologyPhaseNonCompliant {
		t.Errorf("Phase = %v, want %v", result.Phase, TopologyPhaseNonCompliant)
	}
	if result.Vector.VRAM != VRAMStateInsufficient {
		t.Errorf("VRAMState = %v, want %v", result.Vector.VRAM, VRAMStateInsufficient)
	}
	if result.Vector.Parallelism != ParallelismStateMisaligned {
		t.Errorf("ParallelismState = %v, want %v", result.Vector.Parallelism, ParallelismStateMisaligned)
	}
	if len(result.Reasons) < 2 {
		t.Errorf("expected at least 2 diagnostic reasons, got %d: %v", len(result.Reasons), result.Reasons)
	}
}

func TestGLM53OnMisalignedGPUCount(t *testing.T) {
	// 3x RTX PRO 6000 does not align with TP=4
	spec := &servingv1alpha2.AcceleratorSpec{
		Type:  servingv1alpha2.AcceleratorTypeNVIDIA,
		Model: servingv1alpha2.AcceleratorModelRTXPRO6000,
		Count: ptr.To(int32(3)),
	}

	topo, err := NewHardwareTopologyFromSpec(spec)
	if err != nil {
		t.Fatalf("create topology: %v", err)
	}

	workload := GLM53EXL3Workload()
	result := ValidateTopology(topo, workload)

	if result.Vector.Parallelism != ParallelismStateMisaligned {
		t.Errorf("ParallelismState = %v, want %v", result.Vector.Parallelism, ParallelismStateMisaligned)
	}
	if result.IsCompliant() {
		t.Errorf("expected non-compliant result for misaligned count")
	}
}

func TestGLM53On8xRTXPRO6000_Compatible(t *testing.T) {
	// 8x RTX PRO 6000 (TP=4, DP=2)
	spec := &servingv1alpha2.AcceleratorSpec{
		Type:  servingv1alpha2.AcceleratorTypeNVIDIA,
		Model: servingv1alpha2.AcceleratorModelRTXPRO6000,
		Count: ptr.To(int32(8)),
	}

	topo, err := NewHardwareTopologyFromSpec(spec)
	if err != nil {
		t.Fatalf("create topology: %v", err)
	}

	workload := GLM53EXL3Workload()
	result := ValidateTopology(topo, workload)

	if !result.IsCompliant() {
		t.Fatalf("expected 8 GPUs to be compliant: %v", result.Reasons)
	}
	if result.Vector.Parallelism != ParallelismStateCompatible {
		t.Errorf("ParallelismState = %v, want %v", result.Vector.Parallelism, ParallelismStateCompatible)
	}
}

func TestHeterogeneousFleetProfiles(t *testing.T) {
	tests := []struct {
		name        string
		vendor      servingv1alpha2.AcceleratorType
		model       servingv1alpha2.AcceleratorModel
		wantVRAMGiB float64
		wantArch    string
		supportsTP  bool
	}{
		{
			name:        "NVIDIA RTX PRO 6000",
			vendor:      servingv1alpha2.AcceleratorTypeNVIDIA,
			model:       servingv1alpha2.AcceleratorModelRTXPRO6000,
			wantVRAMGiB: 48,
			wantArch:    "Ada Lovelace",
			supportsTP:  true,
		},
		{
			name:        "NVIDIA DGX Spark",
			vendor:      servingv1alpha2.AcceleratorTypeNVIDIA,
			model:       servingv1alpha2.AcceleratorModelDGXSpark,
			wantVRAMGiB: 144,
			wantArch:    "Blackwell Spark",
			supportsTP:  true,
		},
		{
			name:        "NVIDIA SM100 Blackwell",
			vendor:      servingv1alpha2.AcceleratorTypeNVIDIA,
			model:       servingv1alpha2.AcceleratorModelSM100,
			wantVRAMGiB: 192,
			wantArch:    "Blackwell",
			supportsTP:  true,
		},
		{
			name:        "NVIDIA SM103 Blackwell Ultra",
			vendor:      servingv1alpha2.AcceleratorTypeNVIDIA,
			model:       servingv1alpha2.AcceleratorModelSM103,
			wantVRAMGiB: 288,
			wantArch:    "Blackwell Ultra",
			supportsTP:  true,
		},
		{
			name:        "AMD ROCm MI300X",
			vendor:      servingv1alpha2.AcceleratorTypeROCm,
			model:       "mi300x",
			wantVRAMGiB: 192,
			wantArch:    "CDNA3",
			supportsTP:  true,
		},
		{
			name:        "Apple Silicon MLX",
			vendor:      servingv1alpha2.AcceleratorTypeAppleMLX,
			model:       "m-series-ultra",
			wantVRAMGiB: 128,
			wantArch:    "Apple Silicon",
			supportsTP:  false,
		},
		{
			name:        "Host CPU",
			vendor:      servingv1alpha2.AcceleratorTypeCPU,
			model:       "host-cpu",
			wantVRAMGiB: 64,
			wantArch:    "x86_64",
			supportsTP:  false,
		},
		{
			name:        "Generic NPU",
			vendor:      servingv1alpha2.AcceleratorTypeNPU,
			model:       "generic-npu",
			wantVRAMGiB: 32,
			wantArch:    "NPU",
			supportsTP:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile, err := LookupProfile(tc.vendor, tc.model)
			if err != nil {
				t.Fatalf("lookup %s failed: %v", tc.name, err)
			}
			if got := float64(profile.VRAMPerDeviceBytes) / float64(gibibyte); got != tc.wantVRAMGiB {
				t.Errorf("VRAM = %f GiB, want %f GiB", got, tc.wantVRAMGiB)
			}
			if profile.Architecture != tc.wantArch {
				t.Errorf("Architecture = %s, want %s", profile.Architecture, tc.wantArch)
			}
			if profile.SupportsTensorParallel != tc.supportsTP {
				t.Errorf("SupportsTensorParallel = %v, want %v", profile.SupportsTensorParallel, tc.supportsTP)
			}
		})
	}
}

func TestUnsupportedHardwareParallelism(t *testing.T) {
	// Apple MLX lacks multi-GPU tensor parallel NCCL/NVLink support
	topo, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeAppleMLX, "m-series-ultra", 2)
	if err != nil {
		t.Fatalf("create Apple MLX topology: %v", err)
	}

	workload := GLM53EXL3Workload()
	result := ValidateTopology(topo, workload)

	if result.Vector.Parallelism != ParallelismStateUnsupported {
		t.Errorf("ParallelismState = %v, want %v", result.Vector.Parallelism, ParallelismStateUnsupported)
	}
	if result.Vector.Vendor != VendorCompatibilityIncompatible {
		t.Errorf("VendorState = %v, want %v (exl3 not supported natively on MLX)", result.Vector.Vendor, VendorCompatibilityIncompatible)
	}
	if result.IsCompliant() {
		t.Errorf("expected non-compliant for Apple MLX with TP=4 and EXL3")
	}
}

func TestVRAMHeadroomDegraded(t *testing.T) {
	// Headroom < 5% triggers DEGRADED state
	topo, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelRTXPRO6000, 4)
	if err != nil {
		t.Fatalf("create topology: %v", err)
	}

	workload := ModelWorkload{
		Name:                 "Custom-Tight-Fit",
		RequiredVRAMBytes:    188 * gibibyte, // 188 GiB on 192 GiB is ~2% headroom (< 5%)
		TensorParallelDegree: 4,
		Quantization:         "exl3",
	}

	result := ValidateTopology(topo, workload)
	if result.Vector.VRAM != VRAMStateDegraded {
		t.Errorf("VRAMState = %v, want %v", result.Vector.VRAM, VRAMStateDegraded)
	}
	if result.Phase != TopologyPhaseDegraded {
		t.Errorf("Phase = %v, want %v", result.Phase, TopologyPhaseDegraded)
	}
	if !result.IsCompliant() {
		t.Errorf("Degraded topology should still be compliant/executable")
	}
}

func TestCompoundAcceleratorTypeSpec(t *testing.T) {
	spec := &servingv1alpha2.AcceleratorSpec{
		Type: servingv1alpha2.AcceleratorType("nvidia/rtx-pro-6000"),
	}

	if got, want := spec.Vendor(), servingv1alpha2.AcceleratorTypeNVIDIA; got != want {
		t.Errorf("Vendor() = %s, want %s", got, want)
	}
	if got, want := spec.ResolvedModel(), servingv1alpha2.AcceleratorModelRTXPRO6000; got != want {
		t.Errorf("ResolvedModel() = %s, want %s", got, want)
	}

	topo, err := NewHardwareTopologyFromSpec(spec)
	if err != nil {
		t.Fatalf("unexpected error creating topology from compound spec: %v", err)
	}
	if topo.DeviceCount != 1 {
		t.Errorf("DeviceCount default = %d, want 1", topo.DeviceCount)
	}
	if topo.Profile.Model != servingv1alpha2.AcceleratorModelRTXPRO6000 {
		t.Errorf("Profile Model = %s, want %s", topo.Profile.Model, servingv1alpha2.AcceleratorModelRTXPRO6000)
	}
}

func TestGLM53OnBlackwellProfiles(t *testing.T) {
	workload := GLM53EXL3Workload()

	// 4x SM100 (4x 192 GiB = 768 GiB, TP=4)
	topoSM100, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelSM100, 4)
	if err != nil {
		t.Fatalf("create SM100 topology: %v", err)
	}
	resSM100 := ValidateTopology(topoSM100, workload)
	if !resSM100.IsCompliant() || resSM100.Phase != TopologyPhaseCompliant {
		t.Errorf("expected 4x SM100 to be compliant, got %v", resSM100.Phase)
	}
	if resSM100.Vector.VRAM != VRAMStateSufficient || resSM100.Vector.Parallelism != ParallelismStateOptimal {
		t.Errorf("4x SM100 states: VRAM=%v, Parallelism=%v", resSM100.Vector.VRAM, resSM100.Vector.Parallelism)
	}

	// 1x SM100 (192 GiB >= 176 GiB, but count=1 < TP=4 => MISALIGNED)
	topoSM100Single, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelSM100, 1)
	if err != nil {
		t.Fatalf("create 1x SM100 topology: %v", err)
	}
	resSingle := ValidateTopology(topoSM100Single, workload)
	if resSingle.IsCompliant() || resSingle.Phase != TopologyPhaseNonCompliant {
		t.Errorf("expected 1x SM100 to be non-compliant due to TP misalignment, got %v", resSingle.Phase)
	}
	if resSingle.Vector.Parallelism != ParallelismStateMisaligned {
		t.Errorf("1x SM100 ParallelismState = %v, want %v", resSingle.Vector.Parallelism, ParallelismStateMisaligned)
	}

	// 4x DGX Spark (4x 144 GiB = 576 GiB, TP=4)
	topoSpark, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelDGXSpark, 4)
	if err != nil {
		t.Fatalf("create DGX Spark topology: %v", err)
	}
	resSpark := ValidateTopology(topoSpark, workload)
	if !resSpark.IsCompliant() || resSpark.Phase != TopologyPhaseCompliant {
		t.Errorf("expected 4x DGX Spark to be compliant, got %v", resSpark.Phase)
	}

	// 4x SM103 (4x 288 GiB = 1152 GiB, TP=4)
	topoSM103, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelSM103, 4)
	if err != nil {
		t.Fatalf("create SM103 topology: %v", err)
	}
	resSM103 := ValidateTopology(topoSM103, workload)
	if !resSM103.IsCompliant() || resSM103.Phase != TopologyPhaseCompliant {
		t.Errorf("expected 4x SM103 to be compliant, got %v", resSM103.Phase)
	}
}

func TestBlackwellModernQuantization(t *testing.T) {
	blackwellProfiles := []servingv1alpha2.AcceleratorModel{
		servingv1alpha2.AcceleratorModelSM100,
		servingv1alpha2.AcceleratorModelSM103,
		servingv1alpha2.AcceleratorModelDGXSpark,
	}

	quantMethods := []string{"nvfp4", "mxfp8", "fp8", "exl3", "bitsandbytes"}

	for _, model := range blackwellProfiles {
		topo, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeNVIDIA, model, 4)
		if err != nil {
			t.Fatalf("create %s topology: %v", model, err)
		}
		for _, q := range quantMethods {
			workload := ModelWorkload{
				Name:                 "Quant-Test",
				Quantization:         q,
				RequiredVRAMBytes:    100 * gibibyte,
				TensorParallelDegree: 4,
			}
			res := ValidateTopology(topo, workload)
			if res.Vector.Vendor != VendorCompatibilityNative {
				t.Errorf("%s with quantization %q vendor state = %v, want %v", model, q, res.Vector.Vendor, VendorCompatibilityNative)
			}
		}
	}

	// Ada (RTX PRO 6000) does not support NVFP4 or MXFP8
	adaTopo, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelRTXPRO6000, 4)
	if err != nil {
		t.Fatalf("create Ada topology: %v", err)
	}
	for _, unsupported := range []string{"nvfp4", "mxfp8"} {
		workload := ModelWorkload{
			Name:                 "Unsupported-Ada-Quant",
			Quantization:         unsupported,
			RequiredVRAMBytes:    100 * gibibyte,
			TensorParallelDegree: 4,
		}
		res := ValidateTopology(adaTopo, workload)
		if res.Vector.Vendor != VendorCompatibilityIncompatible {
			t.Errorf("Ada RTX PRO 6000 should be INCOMPATIBLE with %q, got %v", unsupported, res.Vector.Vendor)
		}
		if res.IsCompliant() {
			t.Errorf("Ada RTX PRO 6000 with %q should not be compliant", unsupported)
		}
	}
}

func TestCrossVendorModelLookupMismatch(t *testing.T) {
	// Requesting an NVIDIA model with ROCm vendor must fail
	_, err := LookupProfile(servingv1alpha2.AcceleratorTypeROCm, servingv1alpha2.AcceleratorModelSM100)
	if err == nil {
		t.Error("expected error looking up SM100 with ROCm vendor, got nil")
	}

	// Requesting a ROCm model with NVIDIA vendor must fail
	_, err = LookupProfile(servingv1alpha2.AcceleratorTypeNVIDIA, "mi300x")
	if err == nil {
		t.Error("expected error looking up mi300x with NVIDIA vendor, got nil")
	}
}

func TestHeterogeneousCompoundSpecs(t *testing.T) {
	cases := []struct {
		typeStr    string
		wantVendor servingv1alpha2.AcceleratorType
		wantModel  servingv1alpha2.AcceleratorModel
	}{
		{"nvidia/sm100", servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelSM100},
		{"nvidia-dgx-spark", servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelDGXSpark},
		{"nvidia/sm103", servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelSM103},
		{"rocm/mi300x", servingv1alpha2.AcceleratorTypeROCm, "mi300x"},
		{"apple-mlx", servingv1alpha2.AcceleratorTypeAppleMLX, ""},
	}

	for _, tc := range cases {
		spec := &servingv1alpha2.AcceleratorSpec{Type: servingv1alpha2.AcceleratorType(tc.typeStr)}
		if got := spec.Vendor(); got != tc.wantVendor {
			t.Errorf("Vendor(%q) = %s, want %s", tc.typeStr, got, tc.wantVendor)
		}
		if got := spec.ResolvedModel(); got != tc.wantModel {
			t.Errorf("ResolvedModel(%q) = %s, want %s", tc.typeStr, got, tc.wantModel)
		}
	}
}
