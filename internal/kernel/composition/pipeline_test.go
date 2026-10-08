/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package composition

import (
	"context"
	"strings"
	"testing"
)

func validComponent(kind ComponentKind, id string) Component {
	return Component{
		Identity: Identity{ID: id, Name: id + "-name", Kind: kind},
		Version:  "1.0.0",
		Digest:   "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Compatibility: CompatibilitySpec{
			Architecture:      "llama",
			SupportedRuntimes: []string{"vllm"},
			Stage:             StagePreExecution,
		},
		Provenance: Provenance{
			SourceURI: "oci://registry.ckodex.com/" + strings.ToLower(string(kind)),
			Publisher: "ckodex-foundation",
		},
		Admission: AdmissionRecord{
			Decision: AdmissionAdmitted,
		},
		Lifecycle: LifecycleRecord{
			State: LifecycleActive,
		},
		Evidence: EvidenceRecord{
			AttestationURI:  "urn:ckodex:slsa:1",
			SignatureDigest: "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
			Integrity:       IntegrityVerified,
		},
		StateVector: StateVector{
			Lifecycle:     LifecycleActive,
			Trust:         TrustVerified,
			Risk:          RiskNormal,
			Admission:     AdmissionAdmitted,
			Compatibility: CompatibilityCompatible,
			Integrity:     IntegrityVerified,
		},
	}
}

func buildValidPipeline() *Pipeline {
	p := NewPipeline("pipe-1", "governed-pipeline", "1.0.0")
	_ = p.SetModel(validComponent(KindModel, "base-model"))
	_ = p.SetTokenizer(validComponent(KindTokenizer, "base-tokenizer"))
	_ = p.SetRuntime(validComponent(KindRuntime, "vllm-engine"))
	return p
}

func TestPipeline_SlotValidation(t *testing.T) {
	p := NewPipeline("pipe-1", "test", "1.0.0")
	tok := validComponent(KindTokenizer, "tok-1")
	if err := p.SetModel(tok); err == nil || !strings.Contains(err.Error(), "invalid component kind") {
		t.Fatalf("expected ErrInvalidKind when setting Tokenizer in Model slot, got %v", err)
	}
	model := validComponent(KindModel, "mod-1")
	if err := p.SetTokenizer(model); err == nil || !strings.Contains(err.Error(), "invalid component kind") {
		t.Fatalf("expected ErrInvalidKind when setting Model in Tokenizer slot, got %v", err)
	}
	if err := p.SetRuntime(model); err == nil || !strings.Contains(err.Error(), "invalid component kind") {
		t.Fatalf("expected ErrInvalidKind when setting Model in Runtime slot, got %v", err)
	}
}

func TestPipeline_RequiredComponents(t *testing.T) {
	p := NewPipeline("pipe-1", "test", "1.0.0")
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "Model slot is mandatory") {
		t.Fatalf("expected mandatory Model error, got %v", err)
	}
	_ = p.SetModel(validComponent(KindModel, "mod"))
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "Tokenizer slot is mandatory") {
		t.Fatalf("expected mandatory Tokenizer error, got %v", err)
	}
	_ = p.SetTokenizer(validComponent(KindTokenizer, "tok"))
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "Runtime slot is mandatory") {
		t.Fatalf("expected mandatory Runtime error, got %v", err)
	}
}

func TestPipeline_IdentityAndDigest(t *testing.T) {
	p := buildValidPipeline()
	invalidMod := validComponent(KindModel, "mod")
	invalidMod.Digest = "tag-only:v1"
	_ = p.SetModel(invalidMod)
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("expected digest error on unpinned ref, got %v", err)
	}
	noID := validComponent(KindModel, "")
	noID.Identity.ID = ""
	_ = p.SetModel(noID)
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "identity missing") {
		t.Fatalf("expected missing identity error, got %v", err)
	}
}

func TestPipeline_ArchitectureCompatibility(t *testing.T) {
	p := buildValidPipeline()
	tok := validComponent(KindTokenizer, "tok")
	tok.Compatibility.Architecture = "mistral"
	_ = p.SetTokenizer(tok)
	report, err := p.Validate()
	if err == nil || report.CompatibilityStatus != CompatibilityIncompatible {
		t.Fatalf("expected architecture mismatch error, got err=%v report=%+v", err, report)
	}
}

func TestPipeline_QuantizationCompatibility(t *testing.T) {
	p := buildValidPipeline()
	q := validComponent(KindQuantization, "quant-awq")
	q.Compatibility.BaseModelDigest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	_ = p.SetQuantization(q)
	report, err := p.Validate()
	if err == nil || report.CompatibilityStatus != CompatibilityIncompatible {
		t.Fatalf("expected baseModelDigest mismatch for quantization, got %v", err)
	}
}

func TestPipeline_AdapterCompatibility(t *testing.T) {
	p := buildValidPipeline()
	adapter := validComponent(KindAdapter, "lora-finance")
	adapter.Compatibility.Architecture = "gemma"
	_ = p.AddAdapter(adapter)
	report, err := p.Validate()
	if err == nil || report.CompatibilityStatus != CompatibilityIncompatible {
		t.Fatalf("expected adapter architecture mismatch, got %v", err)
	}
}

func TestPipeline_ProjectionCompatibility(t *testing.T) {
	p := buildValidPipeline()
	p.Model.Compatibility.OutputDimension = 4096
	proj := validComponent(KindProjection, "vision-proj")
	proj.Compatibility.InputDimension = 2048
	_ = p.AddProjection(proj)
	report, err := p.Validate()
	if err == nil || report.CompatibilityStatus != CompatibilityIncompatible {
		t.Fatalf("expected dimension mismatch for projection, got %v", err)
	}
}

func TestPipeline_FilterAndGuardStages(t *testing.T) {
	p := buildValidPipeline()
	badFilter := validComponent(KindFilter, "filter-bad")
	badFilter.Compatibility.Stage = StageTokenize
	_ = p.AddFilter(badFilter)
	report, err := p.Validate()
	if err == nil || report.CompatibilityStatus != CompatibilityIncompatible {
		t.Fatalf("expected invalid stage for filter, got %v", err)
	}
}

func TestPipeline_ForbiddenVectorTuples_AntiExecuteAndUntrusted(t *testing.T) {
	p := buildValidPipeline()
	p.Model.StateVector.Trust = TrustDenied
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "anti_execute") {
		t.Fatalf("expected anti_execute on denied trust, got %v", err)
	}
	p = buildValidPipeline()
	p.Model.StateVector.Admission = AdmissionDenied
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "anti_execute") {
		t.Fatalf("expected anti_execute on denied admission, got %v", err)
	}
	p = buildValidPipeline()
	p.Model.StateVector.Lifecycle = LifecycleActive
	p.Model.StateVector.Trust = TrustUnknown
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "active_untrusted") {
		t.Fatalf("expected active_untrusted, got %v", err)
	}
}

func TestPipeline_ForbiddenVectorTuples_EscalationAndDAL(t *testing.T) {
	p := buildValidPipeline()
	p.Model.StateVector.Lifecycle = LifecycleQuarantined
	p.Model.StateVector.Admission = AdmissionAdmitted
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "negative_escalation_skipped") {
		t.Fatalf("expected negative_escalation_skipped, got %v", err)
	}
	p = buildValidPipeline()
	p.Model.StateVector.Risk = RiskHigh
	p.Model.StateVector.Integrity = IntegrityUnsigned
	if _, err := p.Validate(); err == nil || !strings.Contains(err.Error(), "empty_high_dal") {
		t.Fatalf("expected empty_high_dal, got %v", err)
	}
}

func TestPipeline_FullCompositeEndToEnd(t *testing.T) {
	p := buildValidPipeline()
	baseDigest := p.Model.Digest

	quant := validComponent(KindQuantization, "quant-awq")
	quant.Compatibility.BaseModelDigest = baseDigest
	_ = p.SetQuantization(quant)

	adapter := validComponent(KindAdapter, "lora-code")
	adapter.Compatibility.BaseModelDigest = baseDigest
	_ = p.AddAdapter(adapter)

	proj := validComponent(KindProjection, "embed-proj")
	proj.Compatibility.BaseModelDigest = baseDigest
	_ = p.AddProjection(proj)

	filter := validComponent(KindFilter, "pre-filter")
	filter.Compatibility.Stage = StagePreExecution
	_ = p.AddFilter(filter)

	guard := validComponent(KindGuard, "input-guard")
	guard.Compatibility.Stage = StagePreExecution
	_ = p.AddGuard(guard)

	plugin := validComponent(KindPlugin, "telemetry-plugin")
	_ = p.AddPlugin(plugin)

	report, err := p.Validate()
	if err != nil {
		t.Fatalf("expected valid composite pipeline, got err: %v", err)
	}
	if report.CompatibilityStatus != CompatibilityCompatible {
		t.Fatalf("expected Compatible status, got %s", report.CompatibilityStatus)
	}
	if !strings.HasPrefix(report.CompositeDigest, "sha256:") {
		t.Fatalf("expected sha256 composite digest, got %s", report.CompositeDigest)
	}
	if report.StateVector.Lifecycle != LifecycleActive {
		t.Fatalf("expected Active lifecycle vector, got %s", report.StateVector.Lifecycle)
	}
}

func TestPipeline_ExecutionSuccess(t *testing.T) {
	p := buildValidPipeline()
	payload := &Payload{Data: "hello world"}
	out, receipt, err := p.Execute(context.Background(), payload)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if receipt.Status != StatusSuccess {
		t.Fatalf("expected Success status, got %s", receipt.Status)
	}
	if !strings.Contains(out.Data, "hello world") {
		t.Fatalf("expected output data to contain input, got %s", out.Data)
	}
	if receipt.CompositeDigest == "" {
		t.Fatal("expected receipt to carry composite digest")
	}
}

func TestPipeline_ExecutionGuardBlocked(t *testing.T) {
	p := buildValidPipeline()
	guard := validComponent(KindGuard, "safety-guard")
	guard.Compatibility.Stage = StagePreExecution
	_ = p.AddGuard(guard)

	payload := &Payload{Data: "MALICIOUS PROMPT INJECTION"}
	out, receipt, err := p.Execute(context.Background(), payload)
	if err == nil {
		t.Fatal("expected execution error on malicious payload, got nil")
	}
	if out != nil {
		t.Fatalf("expected nil payload on block, got %+v", out)
	}
	if receipt == nil || receipt.Status != StatusHalted {
		t.Fatalf("expected Halted receipt status, got %+v", receipt)
	}
}

func TestPipeline_ExecutionPreflightFailure(t *testing.T) {
	p := buildValidPipeline()
	p.Model.StateVector.Trust = TrustDenied
	payload := &Payload{Data: "hello"}
	_, _, err := p.Execute(context.Background(), payload)
	if err == nil || !strings.Contains(err.Error(), "pre-flight validation failed") {
		t.Fatalf("expected pre-flight validation halt, got %v", err)
	}
}
