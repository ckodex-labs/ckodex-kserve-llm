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

func TestLookupHeterogeneousProfiles(t *testing.T) {
	cases := []struct {
		name        string
		vendor      servingv1alpha2.AcceleratorType
		model       servingv1alpha2.AcceleratorModel
		wantVRAMGiB float64
		wantArch    string
		wantCap     string
		wantConn    string
		supportsTP  bool
		supportsEP  bool
		expectedQ   string
	}{
		{
			name:        "GB300 Superchip",
			vendor:      servingv1alpha2.AcceleratorTypeNVIDIA,
			model:       servingv1alpha2.AcceleratorModelGB300,
			wantVRAMGiB: 252,
			wantArch:    "Blackwell Ultra Superchip",
			wantCap:     "sm_103",
			wantConn:    "NVLink-C2C",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "nvfp4",
		},
		{
			name:        "Apple M4 Ultra",
			vendor:      servingv1alpha2.AcceleratorTypeAppleMetal,
			model:       servingv1alpha2.AcceleratorModelM4Ultra,
			wantVRAMGiB: 192,
			wantArch:    "Apple Silicon UMA",
			wantCap:     "metal-3",
			wantConn:    "UltraFusion-UMA",
			supportsTP:  false,
			supportsEP:  false,
			expectedQ:   "mlx",
		},
		{
			name:        "Apple M3 Ultra",
			vendor:      servingv1alpha2.AcceleratorTypeAppleMetal,
			model:       servingv1alpha2.AcceleratorModelM3Ultra,
			wantVRAMGiB: 128,
			wantArch:    "Apple Silicon UMA",
			wantCap:     "metal-3",
			wantConn:    "UltraFusion-UMA",
			supportsTP:  false,
			supportsEP:  false,
			expectedQ:   "gguf",
		},
		{
			name:        "ROCm MI325X",
			vendor:      servingv1alpha2.AcceleratorTypeROCm,
			model:       servingv1alpha2.AcceleratorModelMI325X,
			wantVRAMGiB: 256,
			wantArch:    "CDNA3",
			wantCap:     "gfx942",
			wantConn:    "Infinity-Fabric",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "fp8",
		},
		{
			name:        "Google TPU v6e Trillium",
			vendor:      servingv1alpha2.AcceleratorTypeGoogleTPU,
			model:       servingv1alpha2.AcceleratorModelTPUv6e,
			wantVRAMGiB: 32,
			wantArch:    "Trillium",
			wantCap:     "tpu-v6e",
			wantConn:    "ICI-Torus",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "int8",
		},
		{
			name:        "Google TPU v5p",
			vendor:      servingv1alpha2.AcceleratorTypeGoogleTPU,
			model:       servingv1alpha2.AcceleratorModelTPUv5p,
			wantVRAMGiB: 96,
			wantArch:    "TPU-v5p",
			wantCap:     "tpu-v5p",
			wantConn:    "ICI-Torus",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "fp8",
		},
		{
			name:        "Google TPU v5e Lite",
			vendor:      servingv1alpha2.AcceleratorTypeGoogleTPU,
			model:       servingv1alpha2.AcceleratorModelTPUv5e,
			wantVRAMGiB: 16,
			wantArch:    "TPU-v5e",
			wantCap:     "tpu-v5e",
			wantConn:    "ICI-2D-Torus",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "bf16",
		},
		{
			name:        "Google TPU v4",
			vendor:      servingv1alpha2.AcceleratorTypeGoogleTPU,
			model:       servingv1alpha2.AcceleratorModelTPUv4,
			wantVRAMGiB: 32,
			wantArch:    "TPU-v4",
			wantCap:     "tpu-v4",
			wantConn:    "ICI-3D-Torus",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "int8",
		},
		{
			name:        "AWS Trainium2",
			vendor:      servingv1alpha2.AcceleratorTypeAWSNeuron,
			model:       servingv1alpha2.AcceleratorModelTrn2,
			wantVRAMGiB: 96,
			wantArch:    "NeuronCore-v2",
			wantCap:     "trn2",
			wantConn:    "NeuronLink-v2",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "bf16",
		},
		{
			name:        "AWS Trainium1",
			vendor:      servingv1alpha2.AcceleratorTypeAWSNeuron,
			model:       servingv1alpha2.AcceleratorModelTrn1,
			wantVRAMGiB: 32,
			wantArch:    "NeuronCore-v2",
			wantCap:     "trn1",
			wantConn:    "NeuronLink-v2",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "bf16",
		},
		{
			name:        "AWS Inferentia2",
			vendor:      servingv1alpha2.AcceleratorTypeAWSNeuron,
			model:       servingv1alpha2.AcceleratorModelInf2,
			wantVRAMGiB: 32,
			wantArch:    "NeuronCore-v2",
			wantCap:     "inf2",
			wantConn:    "NeuronLink-v2",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "fp16",
		},
		{
			name:        "Intel Gaudi3",
			vendor:      servingv1alpha2.AcceleratorTypeIntelGaudi,
			model:       servingv1alpha2.AcceleratorModelGaudi3,
			wantVRAMGiB: 128,
			wantArch:    "G3-DualDie",
			wantCap:     "gaudi3",
			wantConn:    "Integrated-200GbE-RoCE",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "fp8",
		},
		{
			name:        "Intel Gaudi2",
			vendor:      servingv1alpha2.AcceleratorTypeIntelGaudi,
			model:       servingv1alpha2.AcceleratorModelGaudi2,
			wantVRAMGiB: 96,
			wantArch:    "Gaudi2",
			wantCap:     "gaudi2",
			wantConn:    "Integrated-100GbE-RoCE",
			supportsTP:  true,
			supportsEP:  true,
			expectedQ:   "bf16",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile, err := LookupProfile(tc.vendor, tc.model)
			if err != nil {
				t.Fatalf("LookupProfile(%s, %s) failed: %v", tc.vendor, tc.model, err)
			}
			if got := float64(profile.VRAMPerDeviceBytes) / float64(gibibyte); got != tc.wantVRAMGiB {
				t.Errorf("VRAM = %f GiB, want %f GiB", got, tc.wantVRAMGiB)
			}
			if profile.Architecture != tc.wantArch {
				t.Errorf("Architecture = %s, want %s", profile.Architecture, tc.wantArch)
			}
			if profile.ComputeCapability != tc.wantCap {
				t.Errorf("ComputeCapability = %s, want %s", profile.ComputeCapability, tc.wantCap)
			}
			if profile.Interconnect != tc.wantConn {
				t.Errorf("Interconnect = %s, want %s", profile.Interconnect, tc.wantConn)
			}
			if profile.SupportsTensorParallel != tc.supportsTP {
				t.Errorf("SupportsTensorParallel = %v, want %v", profile.SupportsTensorParallel, tc.supportsTP)
			}
			if profile.SupportsExpertParallel != tc.supportsEP {
				t.Errorf("SupportsExpertParallel = %v, want %v", profile.SupportsExpertParallel, tc.supportsEP)
			}
			foundQ := false
			for _, q := range profile.SupportedQuantizations {
				if q == tc.expectedQ {
					foundQ = true
					break
				}
			}
			if !foundQ {
				t.Errorf("expected quantization %s not found in %v", tc.expectedQ, profile.SupportedQuantizations)
			}
		})
	}
}

func TestHeterogeneousVRAMCalculations(t *testing.T) {
	tests := []struct {
		vendor       servingv1alpha2.AcceleratorType
		model        servingv1alpha2.AcceleratorModel
		count        int32
		wantTotalGiB float64
	}{
		{servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelGB300, 1, 252.0},
		{servingv1alpha2.AcceleratorTypeNVIDIA, servingv1alpha2.AcceleratorModelGB300, 4, 1008.0},
		{servingv1alpha2.AcceleratorTypeROCm, servingv1alpha2.AcceleratorModelMI325X, 8, 2048.0},
		{servingv1alpha2.AcceleratorTypeAppleMetal, servingv1alpha2.AcceleratorModelM4Ultra, 1, 192.0},
		{servingv1alpha2.AcceleratorTypeAppleMetal, servingv1alpha2.AcceleratorModelM3Ultra, 1, 128.0},
		{servingv1alpha2.AcceleratorTypeGoogleTPU, servingv1alpha2.AcceleratorModelTPUv6e, 4, 128.0},
		{servingv1alpha2.AcceleratorTypeGoogleTPU, servingv1alpha2.AcceleratorModelTPUv6e, 8, 256.0},
		{servingv1alpha2.AcceleratorTypeGoogleTPU, servingv1alpha2.AcceleratorModelTPUv5p, 4, 384.0},
		{servingv1alpha2.AcceleratorTypeGoogleTPU, servingv1alpha2.AcceleratorModelTPUv5e, 8, 128.0},
		{servingv1alpha2.AcceleratorTypeGoogleTPU, servingv1alpha2.AcceleratorModelTPUv4, 4, 128.0},
		{servingv1alpha2.AcceleratorTypeAWSNeuron, servingv1alpha2.AcceleratorModelTrn2, 4, 384.0},
		{servingv1alpha2.AcceleratorTypeAWSNeuron, servingv1alpha2.AcceleratorModelTrn1, 4, 128.0},
		{servingv1alpha2.AcceleratorTypeAWSNeuron, servingv1alpha2.AcceleratorModelInf2, 4, 128.0},
		{servingv1alpha2.AcceleratorTypeIntelGaudi, servingv1alpha2.AcceleratorModelGaudi3, 4, 512.0},
		{servingv1alpha2.AcceleratorTypeIntelGaudi, servingv1alpha2.AcceleratorModelGaudi3, 8, 1024.0},
		{servingv1alpha2.AcceleratorTypeIntelGaudi, servingv1alpha2.AcceleratorModelGaudi2, 4, 384.0},
	}

	for _, tc := range tests {
		topo, err := NewHardwareTopology(tc.vendor, tc.model, tc.count)
		if err != nil {
			t.Fatalf("NewHardwareTopology(%s, %s, %d) failed: %v", tc.vendor, tc.model, tc.count, err)
		}
		if got := topo.TotalVRAMGiB(); got != tc.wantTotalGiB {
			t.Errorf("TotalVRAMGiB(%s, %d) = %f, want %f", tc.model, tc.count, got, tc.wantTotalGiB)
		}
	}
}

func TestHeterogeneousWorkloadValidation(t *testing.T) {
	// Google TPU v6e Trillium 8-chip pod with BF16
	tpuTopo, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeGoogleTPU, servingv1alpha2.AcceleratorModelTPUv6e, 8)
	if err != nil {
		t.Fatalf("create TPU v6e topology: %v", err)
	}
	tpuWorkload := ModelWorkload{
		Name:                 "Llama-3-70B-BF16",
		Quantization:         "bf16",
		RequiredVRAMBytes:    140 * gibibyte,
		TensorParallelDegree: 8,
		RequiresExpert:       false,
	}
	resTPU := ValidateTopology(tpuTopo, tpuWorkload)
	if !resTPU.IsCompliant() || resTPU.Phase != TopologyPhaseCompliant {
		t.Errorf("TPU v6e validation expected compliant, got %v: %v", resTPU.Phase, resTPU.Reasons)
	}

	// TPU with unsupported quantization (e.g. exl3) must fail
	incompatWorkload := ModelWorkload{
		Name:                 "TPU-Incompatible-Quant",
		Quantization:         "exl3",
		RequiredVRAMBytes:    100 * gibibyte,
		TensorParallelDegree: 4,
	}
	tpuTopo4, _ := NewHardwareTopology(servingv1alpha2.AcceleratorTypeGoogleTPU, servingv1alpha2.AcceleratorModelTPUv6e, 4)
	resIncompat := ValidateTopology(tpuTopo4, incompatWorkload)
	if resIncompat.IsCompliant() || resIncompat.Vector.Vendor != VendorCompatibilityIncompatible {
		t.Errorf("TPU v6e with exl3 should be incompatible, got %v", resIncompat.Vector.Vendor)
	}

	// Apple M4 Ultra with GGUF single device execution
	m4Topo, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeAppleMetal, servingv1alpha2.AcceleratorModelM4Ultra, 1)
	if err != nil {
		t.Fatalf("create M4 Ultra topology: %v", err)
	}
	m4Workload := ModelWorkload{
		Name:                 "DeepSeek-R1-Q4-GGUF",
		Quantization:         "gguf",
		RequiredVRAMBytes:    160 * gibibyte,
		TensorParallelDegree: 1,
	}
	resM4 := ValidateTopology(m4Topo, m4Workload)
	if !resM4.IsCompliant() || resM4.Phase != TopologyPhaseCompliant {
		t.Errorf("M4 Ultra GGUF single device should be compliant, got %v", resM4.Phase)
	}

	// Apple M4 Ultra requesting TP=4 must fail because M4 Ultra lacks TP interconnect
	m4TPWorkload := ModelWorkload{
		Name:                 "M4-Invalid-TP",
		Quantization:         "mlx",
		RequiredVRAMBytes:    100 * gibibyte,
		TensorParallelDegree: 4,
	}
	resM4TP := ValidateTopology(m4Topo, m4TPWorkload)
	if resM4TP.IsCompliant() || resM4TP.Vector.Parallelism != ParallelismStateUnsupported {
		t.Errorf("M4 Ultra with TP=4 should report unsupported parallelism, got %v", resM4TP.Vector.Parallelism)
	}

	// AMD ROCm MI325X 8-GPU with FP8 and TP=8
	rocmTopo, err := NewHardwareTopology(servingv1alpha2.AcceleratorTypeROCm, servingv1alpha2.AcceleratorModelMI325X, 8)
	if err != nil {
		t.Fatalf("create MI325X topology: %v", err)
	}
	rocmWorkload := ModelWorkload{
		Name:                 "Qwen-2.5-72B-FP8",
		Quantization:         "fp8",
		RequiredVRAMBytes:    80 * gibibyte,
		TensorParallelDegree: 8,
	}
	resROCm := ValidateTopology(rocmTopo, rocmWorkload)
	if !resROCm.IsCompliant() {
		t.Errorf("MI325X FP8 TP=8 expected compliant, got %v", resROCm.Phase)
	}
}

func TestHeterogeneousSpecParsingAndResolution(t *testing.T) {
	cases := []struct {
		spec        servingv1alpha2.AcceleratorSpec
		wantVendor  servingv1alpha2.AcceleratorType
		wantModel   servingv1alpha2.AcceleratorModel
		wantVRAMGiB float64
	}{
		{
			spec: servingv1alpha2.AcceleratorSpec{
				Type:  servingv1alpha2.AcceleratorTypeGoogleTPU,
				Model: servingv1alpha2.AcceleratorModelTPUv6e,
				Count: ptr.To(int32(4)),
			},
			wantVendor:  servingv1alpha2.AcceleratorTypeGoogleTPU,
			wantModel:   servingv1alpha2.AcceleratorModelTPUv6e,
			wantVRAMGiB: 128.0,
		},
		{
			spec: servingv1alpha2.AcceleratorSpec{
				Type:  "google-tpu-v6e",
				Count: ptr.To(int32(8)),
			},
			wantVendor:  servingv1alpha2.AcceleratorTypeGoogleTPU,
			wantModel:   servingv1alpha2.AcceleratorModelTPUv6e,
			wantVRAMGiB: 256.0,
		},
		{
			spec: servingv1alpha2.AcceleratorSpec{
				Type:  servingv1alpha2.AcceleratorTypeAWSNeuron,
				Model: servingv1alpha2.AcceleratorModelTrn2,
				Count: ptr.To(int32(2)),
			},
			wantVendor:  servingv1alpha2.AcceleratorTypeAWSNeuron,
			wantModel:   servingv1alpha2.AcceleratorModelTrn2,
			wantVRAMGiB: 192.0,
		},
		{
			spec: servingv1alpha2.AcceleratorSpec{
				Type:  servingv1alpha2.AcceleratorTypeIntelGaudi,
				Model: servingv1alpha2.AcceleratorModelGaudi3,
				Count: ptr.To(int32(4)),
			},
			wantVendor:  servingv1alpha2.AcceleratorTypeIntelGaudi,
			wantModel:   servingv1alpha2.AcceleratorModelGaudi3,
			wantVRAMGiB: 512.0,
		},
		{
			spec: servingv1alpha2.AcceleratorSpec{
				Type:  servingv1alpha2.AcceleratorTypeAppleMetal,
				Model: servingv1alpha2.AcceleratorModelM4Ultra,
				Count: ptr.To(int32(1)),
			},
			wantVendor:  servingv1alpha2.AcceleratorTypeAppleMetal,
			wantModel:   servingv1alpha2.AcceleratorModelM4Ultra,
			wantVRAMGiB: 192.0,
		},
		{
			spec: servingv1alpha2.AcceleratorSpec{
				Type:  "apple-m4-ultra",
				Count: ptr.To(int32(1)),
			},
			wantVendor:  servingv1alpha2.AcceleratorTypeAppleMetal,
			wantModel:   servingv1alpha2.AcceleratorModelM4Ultra,
			wantVRAMGiB: 192.0,
		},
		{
			spec: servingv1alpha2.AcceleratorSpec{
				Type:  servingv1alpha2.AcceleratorTypeNVIDIA,
				Model: servingv1alpha2.AcceleratorModelGB300,
				Count: ptr.To(int32(2)),
			},
			wantVendor:  servingv1alpha2.AcceleratorTypeNVIDIA,
			wantModel:   servingv1alpha2.AcceleratorModelGB300,
			wantVRAMGiB: 504.0,
		},
		{
			spec: servingv1alpha2.AcceleratorSpec{
				Type:  "rocm/mi325x",
				Count: ptr.To(int32(4)),
			},
			wantVendor:  servingv1alpha2.AcceleratorTypeROCm,
			wantModel:   servingv1alpha2.AcceleratorModelMI325X,
			wantVRAMGiB: 1024.0,
		},
	}

	for _, tc := range cases {
		if got := tc.spec.Vendor(); got != tc.wantVendor {
			t.Errorf("Vendor(%v) = %s, want %s", tc.spec.Type, got, tc.wantVendor)
		}
		if got := tc.spec.ResolvedModel(); got != tc.wantModel {
			t.Errorf("ResolvedModel(%v) = %s, want %s", tc.spec.Type, got, tc.wantModel)
		}
		topo, err := NewHardwareTopologyFromSpec(&tc.spec)
		if err != nil {
			t.Fatalf("NewHardwareTopologyFromSpec failed: %v", err)
		}
		if got := topo.TotalVRAMGiB(); got != tc.wantVRAMGiB {
			t.Errorf("TotalVRAMGiB() = %f, want %f", got, tc.wantVRAMGiB)
		}
	}
}
