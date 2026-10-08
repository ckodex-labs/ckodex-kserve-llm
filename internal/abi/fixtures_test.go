/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package abi_test

import (
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/abi"
)

var fixedTime = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func baseModelSpec() abi.ModelSpec {
	return abi.ModelSpec{
		Descriptor: abi.ModelDescriptor{
			ModelID:         "meta-llama/Llama-3-8B-Instruct",
			Family:          "llama3",
			Architecture:    "LlamaForCausalLM",
			ContextLength:   8192,
			ParametersCount: 8000000000,
			Precision:       "bfloat16",
			ContentDigest:   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		SourceURI: "s3://models/llama-3-8b-instruct",
		Revision:  "main",
	}
}

func baseRuntimeSpec() abi.RuntimeSpec {
	return abi.RuntimeSpec{
		Descriptor: abi.RuntimeDescriptor{
			Engine:          abi.EngineVLLM,
			Version:         "v0.31.0",
			ImageDigest:     "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			ConformanceTier: abi.ConformanceTier3,
		},
		Accelerator:      abi.AcceleratorCUDA,
		AcceleratorCount: 2,
		MemoryBytes:      32 * 1024 * 1024 * 1024,
		Parallelism: abi.ParallelismSpec{
			TensorParallel:   2,
			PipelineParallel: 1,
			DataParallel:     1,
			ExpertParallel:   1,
		},
		ConcurrencyLimit: 128,
	}
}

func baseAdapters() []abi.AdapterSpec {
	return []abi.AdapterSpec{
		{
			Descriptor: abi.AdapterDescriptor{
				AdapterID:     "customer-support-lora",
				Type:          abi.AdapterLoRA,
				BaseDigest:    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				Rank:          16,
				Alpha:         32.0,
				TargetModules: []string{"q_proj", "v_proj"},
				Digest:        "sha256:112233445566778899aabbccddeeff00112233445566778899aabbccddeeff00",
			},
			HotSwapPolicy: abi.HotSwapImmediate,
			WeightsURI:    "s3://adapters/lora-1",
		},
	}
}

func baseTokenizerSpec() abi.TokenizerSpec {
	return abi.TokenizerSpec{
		Descriptor: abi.TokenizerDescriptor{
			Kind:             abi.TokenizerBPE,
			VocabularySize:   128256,
			TruncationLength: 8192,
			SpecialTokens: abi.SpecialTokens{
				BOS: "<|begin_of_text|>",
				EOS: "<|end_of_text|>",
			},
		},
		ConfigURI: "s3://models/llama-3-8b-instruct/tokenizer.json",
	}
}

func baseKVTransferSpec() *abi.KVTransferSpec {
	return &abi.KVTransferSpec{
		Descriptor: abi.KVTransferDescriptor{
			Protocol:            abi.KVProtocolNixl,
			CacheDtype:          abi.KVDtypeFP8,
			BlockSize:           16,
			PageSize:            4096,
			ChannelCapacityByte: 1024 * 1024 * 1024,
		},
		RemoteEndpoint: "paged-kv.decode-cluster:9090",
		SessionID:      "session-001",
	}
}

func baseRoutingSpec() abi.RoutingSpec {
	return abi.RoutingSpec{
		Policy: abi.RoutingPolicyKVAware,
		Targets: []abi.RouteTarget{
			{
				Endpoint: "10.0.0.1:8000",
				Weight:   100,
				Priority: 1,
				State: abi.StateVector{
					Lifecycle:   abi.LifecycleActive,
					Sync:        abi.SyncSynchronized,
					Health:      abi.HealthHealthy,
					Attestation: abi.AttestationVerified,
					ObservedAt:  fixedTime,
				},
			},
		},
		MaxQueueTimeout: 5 * time.Second,
	}
}

func baseEvidenceRecord() abi.EvidenceRecord {
	return abi.EvidenceRecord{
		Claims: []abi.EvidenceClaim{
			{
				ClaimID:    "claim-1",
				Kind:       abi.EvidenceDigest,
				Issuer:     "ckodex-notary",
				SubjectURN: "urn:ois:model:ckodex:llama3-8b",
				Algorithm:  "sha256",
				Digest:     "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				AssertedAt: fixedTime,
			},
		},
		State: abi.AttestationVerified,
	}
}

func baseValidSpec() abi.OpenInferenceSpec {
	return abi.OpenInferenceSpec{
		Version:    "v1alpha1",
		Model:      baseModelSpec(),
		Runtime:    baseRuntimeSpec(),
		Adapters:   baseAdapters(),
		Tokenizer:  baseTokenizerSpec(),
		KVTransfer: baseKVTransferSpec(),
		Routing:    baseRoutingSpec(),
		Evidence:   baseEvidenceRecord(),
		State: abi.StateVector{
			Lifecycle:   abi.LifecycleReady,
			Sync:        abi.SyncSynchronized,
			Health:      abi.HealthHealthy,
			Attestation: abi.AttestationVerified,
			ObservedAt:  fixedTime,
		},
	}
}

func fullTier3Capabilities() abi.CapabilityMatrix {
	return abi.CapabilityMatrix{
		TensorParallel:      abi.CapabilitySupported,
		DataParallel:        abi.CapabilitySupported,
		PipelineParallel:    abi.CapabilitySupported,
		ExpertParallel:      abi.CapabilitySupported,
		KVCacheDtype:        abi.CapabilitySupported,
		KVTransfer:          abi.CapabilitySupported,
		SpeculativeDecoding: abi.CapabilitySupported,
		Quantization:        abi.CapabilitySupported,
		LoRAHotSwap:         abi.CapabilitySupported,
		StructuredOutput:    abi.CapabilitySupported,
		ChunkedPrefill:      abi.CapabilitySupported,
	}
}
