/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package abi defines the canonical OpenInferenceSpec Application Binary Interface (ABI).
// It establishes a stable, pure domain seam across heterogeneous execution backends
// (KServe, llm-d, vLLM v0.31.0, TensorFold, SGLang) without binding to transport,
// container runtime flags, or Kubernetes-specific schemas.
package abi

import (
	"fmt"
	"strings"
)

// Model is the pure canonical contract for model identity and bounds.
type Model interface {
	Descriptor() ModelDescriptor
	Validate() error
}

// Runtime is the pure execution contract for runtime engines.
type Runtime interface {
	Descriptor() RuntimeDescriptor
	Capabilities() CapabilityMatrix
	State() StateVector
	Validate(spec RuntimeSpec) error
}

// Adapter is the contract for parameter-efficient fine-tuning layers.
type Adapter interface {
	Descriptor() AdapterDescriptor
	Validate() error
}

// Tokenizer is the contract for text encodings and vocabulary boundaries.
type Tokenizer interface {
	Descriptor() TokenizerDescriptor
	Validate() error
}

// KVTransfer is the contract for cross-worker KV cache mobility.
type KVTransfer interface {
	Descriptor() KVTransferDescriptor
	State() KVTransferState
	Validate() error
}

// Router is the contract for traffic distribution and KV-aware scheduling.
type Router interface {
	SelectTarget(spec RoutingSpec, metrics []EndpointPickerMetrics) (RouteTarget, error)
	Validate() error
}

// EvidenceValidator is the contract for cryptographic verification.
type EvidenceValidator interface {
	Verify(record EvidenceRecord) (AttestationState, error)
}

// ConformanceEvaluator evaluates engine compliance against a declared tier.
type ConformanceEvaluator interface {
	Evaluate(caps CapabilityMatrix, tier ConformanceTier) ConformanceEvaluation
}

// TelemetryProvider collects standardized performance and signal records.
type TelemetryProvider interface {
	Collect() (TelemetryRecord, error)
	Validate() error
}

// OpenInferenceSpec is the root canonical declarative specification for model serving.
type OpenInferenceSpec struct {
	Version    string          `json:"version"`
	Model      ModelSpec       `json:"model"`
	Runtime    RuntimeSpec     `json:"runtime"`
	Adapters   []AdapterSpec   `json:"adapters,omitempty"`
	Tokenizer  TokenizerSpec   `json:"tokenizer"`
	KVTransfer *KVTransferSpec `json:"kv_transfer,omitempty"`
	Routing    RoutingSpec     `json:"routing"`
	Evidence   EvidenceRecord  `json:"evidence"`
	State      StateVector     `json:"state"`
}

// BackendEngine is the multi-backend interface implemented by runtime providers.
type BackendEngine interface {
	Kind() EngineKind
	Descriptor() RuntimeDescriptor
	Capabilities() CapabilityMatrix
	ValidateSpec(spec OpenInferenceSpec) error
	EvaluateConformance(tier ConformanceTier) ConformanceEvaluation
}

// Validate checks completeness of ModelSpec.
func (m ModelSpec) Validate() error {
	if strings.TrimSpace(m.Descriptor.ModelID) == "" {
		return fmt.Errorf("%w: missing model id", ErrInvalidModel)
	}
	if m.Descriptor.ContextLength <= 0 {
		return fmt.Errorf("%w: non-positive context length", ErrInvalidModel)
	}
	if strings.TrimSpace(m.Descriptor.ContentDigest) == "" {
		return fmt.Errorf("%w: missing content digest", ErrInvalidModel)
	}
	if strings.TrimSpace(m.SourceURI) == "" {
		return fmt.Errorf("%w: missing source uri", ErrInvalidModel)
	}
	return nil
}

// Validate checks completeness of RuntimeSpec.
func (r RuntimeSpec) Validate() error {
	if strings.TrimSpace(string(r.Descriptor.Engine)) == "" {
		return fmt.Errorf("%w: engine kind is required", ErrInvalidRuntime)
	}
	if r.Descriptor.ConformanceTier < ConformanceTier0 || r.Descriptor.ConformanceTier > ConformanceTier3 {
		return fmt.Errorf("%w: invalid conformance tier %d", ErrInvalidRuntime, r.Descriptor.ConformanceTier)
	}
	if r.AcceleratorCount < 0 {
		return fmt.Errorf("%w: negative accelerator count", ErrInvalidRuntime)
	}
	if r.Parallelism.TensorParallel < 0 || r.Parallelism.PipelineParallel < 0 {
		return fmt.Errorf("%w: negative parallelism count", ErrInvalidRuntime)
	}
	return nil
}

// Validate checks completeness of AdapterSpec.
func (a AdapterSpec) Validate() error {
	if strings.TrimSpace(a.Descriptor.AdapterID) == "" {
		return fmt.Errorf("%w: missing adapter id", ErrInvalidAdapter)
	}
	if a.Descriptor.Rank <= 0 {
		return fmt.Errorf("%w: invalid adapter rank %d", ErrInvalidAdapter, a.Descriptor.Rank)
	}
	if strings.TrimSpace(a.WeightsURI) == "" {
		return fmt.Errorf("%w: missing weights uri", ErrInvalidAdapter)
	}
	return nil
}

// Validate checks completeness of TokenizerSpec.
func (t TokenizerSpec) Validate() error {
	if strings.TrimSpace(string(t.Descriptor.Kind)) == "" {
		return fmt.Errorf("%w: missing tokenizer kind", ErrInvalidTokenizer)
	}
	if t.Descriptor.VocabularySize <= 0 {
		return fmt.Errorf("%w: non-positive vocabulary size", ErrInvalidTokenizer)
	}
	return nil
}

// Validate checks completeness of KVTransferSpec.
func (k KVTransferSpec) Validate() error {
	if strings.TrimSpace(string(k.Descriptor.Protocol)) == "" {
		return fmt.Errorf("%w: missing transfer protocol", ErrInvalidKVTransfer)
	}
	if strings.TrimSpace(k.RemoteEndpoint) == "" {
		return fmt.Errorf("%w: missing remote endpoint", ErrInvalidKVTransfer)
	}
	return nil
}

// Validate checks completeness of RoutingSpec.
func (r RoutingSpec) Validate() error {
	if strings.TrimSpace(string(r.Policy)) == "" {
		return fmt.Errorf("%w: missing routing policy", ErrInvalidRouting)
	}
	if len(r.Targets) == 0 {
		return fmt.Errorf("%w: at least one routing target required", ErrInvalidRouting)
	}
	for i, target := range r.Targets {
		if strings.TrimSpace(target.Endpoint) == "" {
			return fmt.Errorf("%w: target %d has empty endpoint", ErrInvalidRouting, i)
		}
	}
	return nil
}

// Validate checks totality of CapabilityMatrix. Every entry must be explicitly declared.
func (c CapabilityMatrix) Validate() error {
	caps := []CapabilitySupport{
		c.TensorParallel, c.DataParallel, c.PipelineParallel, c.ExpertParallel,
		c.KVCacheDtype, c.KVTransfer, c.SpeculativeDecoding, c.Quantization,
		c.LoRAHotSwap, c.StructuredOutput, c.ChunkedPrefill,
	}
	for _, capSupport := range caps {
		switch capSupport {
		case CapabilitySupported, CapabilityUnsupported, CapabilityEmulated:
		default:
			return fmt.Errorf("%w: invalid or undeclared capability state %q", ErrInvalidCapability, capSupport)
		}
	}
	return nil
}

// Validate checks totality of EvidenceRecord.
func (e EvidenceRecord) Validate() error {
	if e.State == AttestationUnspecified {
		return fmt.Errorf("%w: attestation state must be declared", ErrInvalidEvidence)
	}
	for i, claim := range e.Claims {
		if strings.TrimSpace(claim.Digest) == "" {
			return fmt.Errorf("%w: claim %d missing digest", ErrInvalidEvidence, i)
		}
	}
	return nil
}

// Validate checks the overall OpenInferenceSpec.
func (spec OpenInferenceSpec) Validate() error {
	if strings.TrimSpace(spec.Version) == "" {
		return fmt.Errorf("%w: missing version", ErrInvalidSpec)
	}
	if err := spec.Model.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	if err := spec.Runtime.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	for i, adapter := range spec.Adapters {
		if err := adapter.Validate(); err != nil {
			return fmt.Errorf("%w: adapter %d invalid: %w", ErrInvalidSpec, i, err)
		}
	}
	if err := spec.Tokenizer.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	if spec.KVTransfer != nil {
		if err := spec.KVTransfer.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidSpec, err)
		}
	}
	if err := spec.Routing.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	if err := spec.Evidence.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	return nil
}

// EvaluateStandardConformance evaluates a capability matrix against a tier.
func EvaluateStandardConformance(caps CapabilityMatrix, tier ConformanceTier) ConformanceEvaluation {
	if err := caps.Validate(); err != nil {
		return ConformanceEvaluation{
			Tier:       tier,
			Status:     ConformanceFail,
			Violations: []string{err.Error()},
		}
	}

	var violations []string
	if tier >= ConformanceTier2 && caps.KVCacheDtype == CapabilityUnsupported {
		violations = append(violations, "tier 2 requires kv cache dtype capability")
	}
	if tier >= ConformanceTier3 {
		if caps.KVTransfer != CapabilitySupported {
			violations = append(violations, "tier 3 requires native kv transfer capability")
		}
		if caps.LoRAHotSwap == CapabilityUnsupported {
			violations = append(violations, "tier 3 requires lora hot swap capability")
		}
	}

	status := ConformancePass
	if len(violations) > 0 {
		status = ConformanceFail
	}
	return ConformanceEvaluation{
		Tier:       tier,
		Status:     status,
		Violations: violations,
	}
}
