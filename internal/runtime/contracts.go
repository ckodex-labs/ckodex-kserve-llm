/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package runtime defines the engine-neutral inference runtime seam.
package runtime

import (
	"encoding/hex"
	"fmt"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// CapabilitySupport describes how an adapter provides one runtime capability.
type CapabilitySupport string

const (
	CapabilitySupported   CapabilitySupport = "supported"
	CapabilityUnsupported CapabilitySupport = "unsupported"
	CapabilityEmulated    CapabilitySupport = "emulated"
)

// ConformanceTier identifies the adapter-contract tier implemented by an engine.
type ConformanceTier int

const (
	ConformanceTierServed ConformanceTier = 1
)

// CapabilityMatrix is total over the currently governed runtime capabilities.
type CapabilityMatrix struct {
	TensorParallel      CapabilitySupport
	DataParallel        CapabilitySupport
	LocalDataParallel   CapabilitySupport
	PipelineParallel    CapabilitySupport
	ExpertParallel      CapabilitySupport
	ExpertLoadBalancing CapabilitySupport
	KVCacheDtype        CapabilitySupport
	CPUOffload          CapabilitySupport
	KVTransfer          CapabilitySupport
	SpeculativeDecoding CapabilitySupport
	Quantization        CapabilitySupport
	LoRAHotSwap         CapabilitySupport
}

// ImageContract binds an adapter to an immutable upstream image manifest.
type ImageContract struct {
	Repository string
	Tag        string
	Digest     string
}

// Reference returns the tag-qualified, digest-pinned image reference.
func (contract ImageContract) Reference() string {
	return fmt.Sprintf("%s:%s@%s", contract.Repository, contract.Tag, contract.Digest)
}

// Valid reports whether the contract is complete and digest-pinned.
func (contract ImageContract) Valid() bool {
	if contract.Repository == "" || contract.Tag == "" || !strings.HasPrefix(contract.Digest, "sha256:") {
		return false
	}
	digestBytes, err := hex.DecodeString(strings.TrimPrefix(contract.Digest, "sha256:"))
	return err == nil && len(digestBytes) == 32
}

// HealthContract declares the engine-native readiness endpoint.
type HealthContract struct {
	Path string
}

// MetricsContract declares the engine-native Prometheus endpoint and the
// arguments that must be rendered to enable it.
type MetricsContract struct {
	Path       string
	EnableArgs []string
}

// RenderRequest is the pure input to engine argument rendering.
type RenderRequest struct {
	Service      *servingv1alpha2.LLMInferenceService
	ExistingArgs []string
	ModelPath    string
	Host         string
	Port         int32
}

// RenderedRuntime is the deterministic engine-owned container configuration.
type RenderedRuntime struct {
	Args []string
}

// Adapter maps the stable service contract onto one inference engine.
type Adapter interface {
	Name() string
	Tier() ConformanceTier
	Image() ImageContract
	HealthContract() HealthContract
	MetricsContract() MetricsContract
	Capabilities() CapabilityMatrix
	Render(RenderRequest) RenderedRuntime
	Validate(*servingv1alpha2.LLMInferenceService) field.ErrorList
}
