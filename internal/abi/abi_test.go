/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package abi_test

import (
	"errors"
	"testing"

	"github.com/ckodex-labs/kserve-llm-operator/internal/abi"
)

// mockBackend implements abi.BackendEngine for multi-backend tests.
type mockBackend struct {
	kind       abi.EngineKind
	descriptor abi.RuntimeDescriptor
	caps       abi.CapabilityMatrix
}

func (m *mockBackend) Kind() abi.EngineKind {
	return m.kind
}

func (m *mockBackend) Descriptor() abi.RuntimeDescriptor {
	return m.descriptor
}

func (m *mockBackend) Capabilities() abi.CapabilityMatrix {
	return m.caps
}

func (m *mockBackend) ValidateSpec(spec abi.OpenInferenceSpec) error {
	return spec.Validate()
}

func (m *mockBackend) EvaluateConformance(tier abi.ConformanceTier) abi.ConformanceEvaluation {
	return abi.EvaluateStandardConformance(m.caps, tier)
}

type backendTestCase struct {
	kind    abi.EngineKind
	version string
	tier    abi.ConformanceTier
	caps    abi.CapabilityMatrix
}

func getPrimaryBackends() []backendTestCase {
	return []backendTestCase{
		{
			kind:    abi.EngineKServe,
			version: "v0.19.0",
			tier:    abi.ConformanceTier1,
			caps: abi.CapabilityMatrix{
				TensorParallel:      abi.CapabilitySupported,
				DataParallel:        abi.CapabilitySupported,
				PipelineParallel:    abi.CapabilityUnsupported,
				ExpertParallel:      abi.CapabilityUnsupported,
				KVCacheDtype:        abi.CapabilityUnsupported,
				KVTransfer:          abi.CapabilityUnsupported,
				SpeculativeDecoding: abi.CapabilityUnsupported,
				Quantization:        abi.CapabilitySupported,
				LoRAHotSwap:         abi.CapabilityUnsupported,
				StructuredOutput:    abi.CapabilitySupported,
				ChunkedPrefill:      abi.CapabilityUnsupported,
			},
		},
		{
			kind:    abi.EngineLLMD,
			version: "v0.9.0",
			tier:    abi.ConformanceTier2,
			caps: func() abi.CapabilityMatrix {
				c := fullTier3Capabilities()
				c.KVTransfer = abi.CapabilityUnsupported
				return c
			}(),
		},
		{
			kind:    abi.EngineVLLM,
			version: "v0.31.0",
			tier:    abi.ConformanceTier3,
			caps:    fullTier3Capabilities(),
		},
	}
}

func getExtendedBackends() []backendTestCase {
	tier2Caps := fullTier3Capabilities()
	tier2Caps.KVTransfer = abi.CapabilityUnsupported

	return []backendTestCase{
		{
			kind:    abi.EngineTensorFold,
			version: "v1.2.0",
			tier:    abi.ConformanceTier2,
			caps: abi.CapabilityMatrix{
				TensorParallel:      abi.CapabilitySupported,
				DataParallel:        abi.CapabilitySupported,
				PipelineParallel:    abi.CapabilitySupported,
				ExpertParallel:      abi.CapabilitySupported,
				KVCacheDtype:        abi.CapabilitySupported,
				KVTransfer:          abi.CapabilityEmulated,
				SpeculativeDecoding: abi.CapabilitySupported,
				Quantization:        abi.CapabilitySupported,
				LoRAHotSwap:         abi.CapabilitySupported,
				StructuredOutput:    abi.CapabilitySupported,
				ChunkedPrefill:      abi.CapabilitySupported,
			},
		},
		{
			kind:    abi.EngineSGLang,
			version: "v0.4.0",
			tier:    abi.ConformanceTier2,
			caps:    tier2Caps,
		},
	}
}

func getTestBackends() []backendTestCase {
	return append(getPrimaryBackends(), getExtendedBackends()...)
}

func verifyBackend(t *testing.T, b backendTestCase, spec abi.OpenInferenceSpec) {
	t.Helper()
	engine := &mockBackend{
		kind: b.kind,
		descriptor: abi.RuntimeDescriptor{
			Engine:          b.kind,
			Version:         b.version,
			ImageDigest:     "sha256:1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff",
			ConformanceTier: b.tier,
		},
		caps: b.caps,
	}

	if engine.Kind() != b.kind {
		t.Fatalf("expected kind %s, got %s", b.kind, engine.Kind())
	}
	if err := engine.ValidateSpec(spec); err != nil {
		t.Fatalf("backend %s failed valid spec: %v", b.kind, err)
	}
	eval := engine.EvaluateConformance(b.tier)
	if eval.Status != abi.ConformancePass {
		t.Fatalf("backend %s failed declared tier %d: %v", b.kind, b.tier, eval.Violations)
	}
}

func TestMultiBackendAbstraction(t *testing.T) {
	spec := baseValidSpec()
	backends := getTestBackends()
	for _, b := range backends {
		verifyBackend(t, b, spec)
	}
}

func TestSpecValidation(t *testing.T) {
	spec := baseValidSpec()
	if err := spec.Validate(); err != nil {
		t.Fatalf("base spec should be valid, got: %v", err)
	}

	invalidSpec := spec
	invalidSpec.Version = ""
	if err := invalidSpec.Validate(); !errors.Is(err, abi.ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec for missing version, got: %v", err)
	}
}

func TestModelValidation(t *testing.T) {
	spec := baseValidSpec()

	badID := spec.Model
	badID.Descriptor.ModelID = "   "
	if err := badID.Validate(); !errors.Is(err, abi.ErrInvalidModel) {
		t.Fatalf("expected ErrInvalidModel for empty model id, got: %v", err)
	}

	badContext := spec.Model
	badContext.Descriptor.ContextLength = 0
	if err := badContext.Validate(); !errors.Is(err, abi.ErrInvalidModel) {
		t.Fatalf("expected ErrInvalidModel for zero context length, got: %v", err)
	}

	badDigest := spec.Model
	badDigest.Descriptor.ContentDigest = ""
	if err := badDigest.Validate(); !errors.Is(err, abi.ErrInvalidModel) {
		t.Fatalf("expected ErrInvalidModel for empty digest, got: %v", err)
	}

	badURI := spec.Model
	badURI.SourceURI = ""
	if err := badURI.Validate(); !errors.Is(err, abi.ErrInvalidModel) {
		t.Fatalf("expected ErrInvalidModel for empty source URI, got: %v", err)
	}
}

func TestRuntimeValidation(t *testing.T) {
	spec := baseValidSpec()

	badEngine := spec.Runtime
	badEngine.Descriptor.Engine = ""
	if err := badEngine.Validate(); !errors.Is(err, abi.ErrInvalidRuntime) {
		t.Fatalf("expected ErrInvalidRuntime for empty engine, got: %v", err)
	}

	badTier := spec.Runtime
	badTier.Descriptor.ConformanceTier = 99
	if err := badTier.Validate(); !errors.Is(err, abi.ErrInvalidRuntime) {
		t.Fatalf("expected ErrInvalidRuntime for out-of-range tier, got: %v", err)
	}

	badAccel := spec.Runtime
	badAccel.AcceleratorCount = -1
	if err := badAccel.Validate(); !errors.Is(err, abi.ErrInvalidRuntime) {
		t.Fatalf("expected ErrInvalidRuntime for negative accelerator count, got: %v", err)
	}

	badParallel := spec.Runtime
	badParallel.Parallelism.TensorParallel = -1
	if err := badParallel.Validate(); !errors.Is(err, abi.ErrInvalidRuntime) {
		t.Fatalf("expected ErrInvalidRuntime for negative tensor parallel, got: %v", err)
	}
}

func TestAdapterValidation(t *testing.T) {
	spec := baseValidSpec()
	adapter := spec.Adapters[0]

	badID := adapter
	badID.Descriptor.AdapterID = ""
	if err := badID.Validate(); !errors.Is(err, abi.ErrInvalidAdapter) {
		t.Fatalf("expected ErrInvalidAdapter for empty adapter id, got: %v", err)
	}

	badRank := adapter
	badRank.Descriptor.Rank = 0
	if err := badRank.Validate(); !errors.Is(err, abi.ErrInvalidAdapter) {
		t.Fatalf("expected ErrInvalidAdapter for zero rank, got: %v", err)
	}

	badURI := adapter
	badURI.WeightsURI = ""
	if err := badURI.Validate(); !errors.Is(err, abi.ErrInvalidAdapter) {
		t.Fatalf("expected ErrInvalidAdapter for empty weights URI, got: %v", err)
	}
}

func TestTokenizerValidation(t *testing.T) {
	spec := baseValidSpec()

	badKind := spec.Tokenizer
	badKind.Descriptor.Kind = ""
	if err := badKind.Validate(); !errors.Is(err, abi.ErrInvalidTokenizer) {
		t.Fatalf("expected ErrInvalidTokenizer for empty kind, got: %v", err)
	}

	badVocab := spec.Tokenizer
	badVocab.Descriptor.VocabularySize = 0
	if err := badVocab.Validate(); !errors.Is(err, abi.ErrInvalidTokenizer) {
		t.Fatalf("expected ErrInvalidTokenizer for zero vocab size, got: %v", err)
	}
}

func TestKVTransferValidation(t *testing.T) {
	spec := baseValidSpec()
	kv := *spec.KVTransfer

	badProto := kv
	badProto.Descriptor.Protocol = ""
	if err := badProto.Validate(); !errors.Is(err, abi.ErrInvalidKVTransfer) {
		t.Fatalf("expected ErrInvalidKVTransfer for empty protocol, got: %v", err)
	}

	badEndpoint := kv
	badEndpoint.RemoteEndpoint = ""
	if err := badEndpoint.Validate(); !errors.Is(err, abi.ErrInvalidKVTransfer) {
		t.Fatalf("expected ErrInvalidKVTransfer for empty endpoint, got: %v", err)
	}
}

func TestRoutingValidation(t *testing.T) {
	spec := baseValidSpec()

	badPolicy := spec.Routing
	badPolicy.Policy = ""
	if err := badPolicy.Validate(); !errors.Is(err, abi.ErrInvalidRouting) {
		t.Fatalf("expected ErrInvalidRouting for empty policy, got: %v", err)
	}

	noTargets := spec.Routing
	noTargets.Targets = nil
	if err := noTargets.Validate(); !errors.Is(err, abi.ErrInvalidRouting) {
		t.Fatalf("expected ErrInvalidRouting for empty targets, got: %v", err)
	}

	emptyTarget := spec.Routing
	emptyTarget.Targets = []abi.RouteTarget{{Endpoint: ""}}
	if err := emptyTarget.Validate(); !errors.Is(err, abi.ErrInvalidRouting) {
		t.Fatalf("expected ErrInvalidRouting for target with empty endpoint, got: %v", err)
	}
}

func TestCapabilityTotalityAndConformance(t *testing.T) {
	caps := fullTier3Capabilities()
	if err := caps.Validate(); err != nil {
		t.Fatalf("valid capability matrix failed: %v", err)
	}

	brokenCaps := caps
	brokenCaps.ChunkedPrefill = ""
	if err := brokenCaps.Validate(); !errors.Is(err, abi.ErrInvalidCapability) {
		t.Fatalf("expected ErrInvalidCapability for undeclared capability, got: %v", err)
	}

	evalPass := abi.EvaluateStandardConformance(caps, abi.ConformanceTier3)
	if evalPass.Status != abi.ConformancePass || len(evalPass.Violations) > 0 {
		t.Fatalf("expected Tier 3 pass, got: %v, violations: %v", evalPass.Status, evalPass.Violations)
	}

	unsupportedKV := caps
	unsupportedKV.KVTransfer = abi.CapabilityUnsupported
	evalFail := abi.EvaluateStandardConformance(unsupportedKV, abi.ConformanceTier3)
	if evalFail.Status != abi.ConformanceFail || len(evalFail.Violations) == 0 {
		t.Fatalf("expected Tier 3 fail when KVTransfer unsupported, got: %v", evalFail.Status)
	}

	unsupportedDtype := caps
	unsupportedDtype.KVCacheDtype = abi.CapabilityUnsupported
	evalTier2Fail := abi.EvaluateStandardConformance(unsupportedDtype, abi.ConformanceTier2)
	if evalTier2Fail.Status != abi.ConformanceFail || len(evalTier2Fail.Violations) == 0 {
		t.Fatalf("expected Tier 2 fail when KVCacheDtype unsupported, got: %v", evalTier2Fail.Status)
	}
}

func TestEvidenceValidation(t *testing.T) {
	spec := baseValidSpec()

	badState := spec.Evidence
	badState.State = abi.AttestationUnspecified
	if err := badState.Validate(); !errors.Is(err, abi.ErrInvalidEvidence) {
		t.Fatalf("expected ErrInvalidEvidence for unspecified attestation state, got: %v", err)
	}

	badDigest := spec.Evidence
	badDigest.Claims = []abi.EvidenceClaim{{Digest: ""}}
	if err := badDigest.Validate(); !errors.Is(err, abi.ErrInvalidEvidence) {
		t.Fatalf("expected ErrInvalidEvidence for empty claim digest, got: %v", err)
	}
}
