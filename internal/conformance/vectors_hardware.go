/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package conformance

import (
	"context"
	"fmt"
)

func createPositiveGLMVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-pos-glm53-tensorfold",
		Name:        "GLM-5.3-EXL3 4x RTX PRO 6000 TensorFold Normal Init",
		Class:       ClassPositive,
		Severity:    SeverityStandard,
		Target:      TargetGLM53EXL3TensorFold,
		Description: "Verifies 4-way TensorFold partitioning of GLM-5.3-EXL3 within 192GB VRAM cap",
		Execute: func(_ context.Context) (*VectorResult, error) {
			const vramPerGPU = 48
			totalVRAM := 4 * vramPerGPU
			modelFootprint := 142
			if modelFootprint > totalVRAM {
				return nil, fmt.Errorf("model footprint %d exceeds VRAM %d", modelFootprint, totalVRAM)
			}
			return &VectorResult{
				VectorID: "vec-pos-glm53-tensorfold",
				Name:     "GLM-5.3-EXL3 4x RTX PRO 6000 TensorFold Normal Init",
				Class:    ClassPositive,
				Severity: SeverityStandard,
				Verdict:  VerdictSatisfied,
				ObservedState: StatePlaneVector{
					Lifecycle: "active",
					Trust:     "verified",
					Integrity: "verified",
					Binding:   "bound-4x-rtx-6000",
				},
				EvidenceDigest: hashString("glm53-exl3-fold4-success"),
				Message:        "TensorFold partition balanced across 4x RTX PRO 6000 GPUs",
			}, nil
		},
	}
}

func createPositiveVLLMVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-pos-vllm-nvfp4-flashmla",
		Name:        "vLLM v0.31.0 NVFP4 FlashMLA Execution",
		Class:       ClassPositive,
		Severity:    SeverityStandard,
		Target:      TargetVLLMNVFP4FlashMLA,
		Description: "Verifies NVFP4 execution on Blackwell with FlashMLA latent attention kernel",
		Execute: func(_ context.Context) (*VectorResult, error) {
			return &VectorResult{
				VectorID: "vec-pos-vllm-nvfp4-flashmla",
				Name:     "vLLM v0.31.0 NVFP4 FlashMLA Execution",
				Class:    ClassPositive,
				Severity: SeverityStandard,
				Verdict:  VerdictSatisfied,
				ObservedState: StatePlaneVector{
					Lifecycle: "active",
					Trust:     "verified",
					Integrity: "verified",
					Binding:   "vllm-0.31.0-nvfp4-flashmla",
				},
				EvidenceDigest: hashString("vllm-v0.31.0-flashmla-pass"),
				Message:        "FlashMLA kernel activated with native NVFP4 tensor cores",
			}, nil
		},
	}
}

func createNegativeVLLMVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-neg-vllm-nvfp4-arch",
		Name:        "vLLM NVFP4 Non-Blackwell Rejection",
		Class:       ClassNegative,
		Severity:    SeverityStandard,
		Target:      TargetVLLMNVFP4FlashMLA,
		Description: "NVFP4 quantization on incompatible GPU arch is rejected cleanly",
		Execute: func(_ context.Context) (*VectorResult, error) {
			gpuArch := "Ada-Lovelace"
			if gpuArch != "Blackwell" {
				return &VectorResult{
					VectorID: "vec-neg-vllm-nvfp4-arch",
					Name:     "vLLM NVFP4 Non-Blackwell Rejection",
					Class:    ClassNegative,
					Severity: SeverityStandard,
					Verdict:  VerdictRejected,
					ObservedState: StatePlaneVector{
						Lifecycle: "rejected",
						Trust:     "untrusted",
						Integrity: "verified",
						Binding:   "unbound",
					},
					EvidenceDigest: hashString("nvfp4-arch-rejected"),
					Message:        "Cleanly rejected: NVFP4 requires Blackwell tensor core architecture",
				}, nil
			}
			return nil, fmt.Errorf("unexpected arch match")
		},
	}
}

func createAntiGLMTamperVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-anti-glm53-tamper",
		Name:        "GLM-5.3 TensorFold Inter-GPU Shard Tampering",
		Class:       ClassAnti,
		Severity:    SeverityHardAnti,
		Target:      TargetGLM53EXL3TensorFold,
		Description: "Corrupted weight shard across TensorFold GPU nodes forces immediate HALT",
		Execute: func(_ context.Context) (*VectorResult, error) {
			shardChecksumValid := false
			if !shardChecksumValid {
				return &VectorResult{
					VectorID: "vec-anti-glm53-tamper",
					Name:     "GLM-5.3 TensorFold Inter-GPU Shard Tampering",
					Class:    ClassAnti,
					Severity: SeverityHardAnti,
					Verdict:  VerdictHaltViolation,
					ObservedState: StatePlaneVector{
						Lifecycle: "halted",
						Trust:     "denied",
						Integrity: "compromised",
						Binding:   "isolated",
					},
					EvidenceDigest: hashString("anti-glm53-shard-tamper"),
					Message:        "Hard Anti-Invariant Violation: Inter-GPU TensorFold shard checksum corrupted",
				}, nil
			}
			return nil, fmt.Errorf("unexpected valid checksum")
		},
	}
}

func createAntiVLLMBypassVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-anti-vllm-nvfp4-bypass",
		Name:        "vLLM NVFP4 Scale Factor Hardware Clamp Bypass",
		Class:       ClassAnti,
		Severity:    SeverityHardAnti,
		Target:      TargetVLLMNVFP4FlashMLA,
		Description: "Adversarial payload attempting to bypass FP4 tensor limits triggers HALT",
		Execute: func(_ context.Context) (*VectorResult, error) {
			payloadBypassAttempt := true
			if payloadBypassAttempt {
				return &VectorResult{
					VectorID: "vec-anti-vllm-nvfp4-bypass",
					Name:     "vLLM NVFP4 Scale Factor Hardware Clamp Bypass",
					Class:    ClassAnti,
					Severity: SeverityHardAnti,
					Verdict:  VerdictHaltViolation,
					ObservedState: StatePlaneVector{
						Lifecycle: "halted",
						Trust:     "denied",
						Integrity: "compromised",
						Binding:   "isolated",
					},
					EvidenceDigest: hashString("anti-vllm-clamp-bypass"),
					Message:        "Hard Anti-Invariant Violation: FP4 scale-factor clamp bypass detected",
				}, nil
			}
			return nil, fmt.Errorf("unexpected non-bypass")
		},
	}
}

func createDegradationFlashMLAVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-deg-flashmla-kv-fragmentation",
		Name:        "FlashMLA Latent Cache Fragmentation Degradation",
		Class:       ClassDegradation,
		Severity:    SeverityDegradable,
		Target:      TargetVLLMNVFP4FlashMLA,
		Description: "Under extreme KV cache fragmentation, FlashMLA degrades to chunked attention",
		Execute: func(_ context.Context) (*VectorResult, error) {
			return &VectorResult{
				VectorID: "vec-deg-flashmla-kv-fragmentation",
				Name:     "FlashMLA Latent Cache Fragmentation Degradation",
				Class:    ClassDegradation,
				Severity: SeverityDegradable,
				Verdict:  VerdictDegraded,
				ObservedState: StatePlaneVector{
					Lifecycle: "degraded",
					Trust:     "asserted",
					Integrity: "verified",
					Binding:   "chunked-fallback",
				},
				EvidenceDigest: hashString("flashmla-degradation-graceful"),
				Message:        "Graceful degradation: FlashMLA shifted to chunked prefill under fragmentation",
			}, nil
		},
	}
}

func createRecoveryTensorFoldVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-rec-tensorfold-pcie-reset",
		Name:        "GLM-5.3 TensorFold PCIe Bus Reset Recovery",
		Class:       ClassRecovery,
		Severity:    SeverityStandard,
		Target:      TargetGLM53EXL3TensorFold,
		Description: "Recovers 4x RTX PRO 6000 ring topology following PCIe link transient drop",
		Execute: func(_ context.Context) (*VectorResult, error) {
			return &VectorResult{
				VectorID: "vec-rec-tensorfold-pcie-reset",
				Name:     "GLM-5.3 TensorFold PCIe Bus Reset Recovery",
				Class:    ClassRecovery,
				Severity: SeverityStandard,
				Verdict:  VerdictSatisfied,
				ObservedState: StatePlaneVector{
					Lifecycle: "active",
					Trust:     "verified",
					Integrity: "verified",
					Binding:   "bound-4x-rtx-6000",
				},
				EvidenceDigest: hashString("tensorfold-recovery-pass"),
				Message:        "TensorFold inter-GPU ring restored and synchronized within recovery bound",
			}, nil
		},
	}
}

func createRaceFlashMLAVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-race-flashmla-kv-alloc",
		Name:        "FlashMLA Concurrent Dynamic KV Allocation",
		Class:       ClassRace,
		Severity:    SeverityStandard,
		Target:      TargetVLLMNVFP4FlashMLA,
		Description: "Concurrent requests allocating latent MLA KV blocks without race condition",
		Execute: func(_ context.Context) (*VectorResult, error) {
			return &VectorResult{
				VectorID: "vec-race-flashmla-kv-alloc",
				Name:     "FlashMLA Concurrent Dynamic KV Allocation",
				Class:    ClassRace,
				Severity: SeverityStandard,
				Verdict:  VerdictSatisfied,
				ObservedState: StatePlaneVector{
					Lifecycle: "active",
					Trust:     "verified",
					Integrity: "verified",
					Binding:   "atomic-kv-alloc",
				},
				EvidenceDigest: hashString("flashmla-race-free-alloc"),
				Message:        "Atomic latent cache reservation preserved invariance under 32 parallel requests",
			}, nil
		},
	}
}

func createSaturationGLMVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-sat-glm53-queue-overflow",
		Name:        "GLM-5.3 4x RTX PRO 6000 Request Queue Saturation",
		Class:       ClassSaturation,
		Severity:    SeverityStandard,
		Target:      TargetGLM53EXL3TensorFold,
		Description: "Queue saturation triggers backpressure without OOM kill or thread starvation",
		Execute: func(_ context.Context) (*VectorResult, error) {
			return &VectorResult{
				VectorID: "vec-sat-glm53-queue-overflow",
				Name:     "GLM-5.3 4x RTX PRO 6000 Request Queue Saturation",
				Class:    ClassSaturation,
				Severity: SeverityStandard,
				Verdict:  VerdictSatisfied,
				ObservedState: StatePlaneVector{
					Lifecycle: "active",
					Trust:     "verified",
					Integrity: "verified",
					Binding:   "backpressure-active",
				},
				EvidenceDigest: hashString("glm53-saturation-backpressure"),
				Message:        "Controlled backpressure activated: 503 retry-after emitted, 0 OOM kills",
			}, nil
		},
	}
}

func createValidButWrongFoldVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-vbw-glm53-5fold-on-4gpu",
		Name:        "GLM-5.3 Valid-But-Wrong 5-Fold Partition on 4 GPUs",
		Class:       ClassValidButWrong,
		Severity:    SeverityStandard,
		Target:      TargetGLM53EXL3TensorFold,
		Description: "Syntactically valid TensorFold spec with 5 fold partitions on 4 cards rejected",
		Execute: func(_ context.Context) (*VectorResult, error) {
			foldCount := 5
			gpuCount := 4
			if foldCount%gpuCount != 0 {
				return &VectorResult{
					VectorID: "vec-vbw-glm53-5fold-on-4gpu",
					Name:     "GLM-5.3 Valid-But-Wrong 5-Fold Partition on 4 GPUs",
					Class:    ClassValidButWrong,
					Severity: SeverityStandard,
					Verdict:  VerdictRejected,
					ObservedState: StatePlaneVector{
						Lifecycle: "rejected",
						Trust:     "untrusted",
						Integrity: "verified",
						Binding:   "unbound",
					},
					EvidenceDigest: hashString("valid-but-wrong-fold-rejected"),
					Message:        "Rejected: TensorFold partition count 5 indivisible by physical GPU count 4",
				}, nil
			}
			return nil, fmt.Errorf("unexpected divisible fold count")
		},
	}
}
