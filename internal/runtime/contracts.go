/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package runtime defines the engine-neutral inference runtime seam.
package runtime

import (
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
	Capabilities() CapabilityMatrix
	Render(RenderRequest) RenderedRuntime
	Validate(*servingv1alpha2.LLMInferenceService) field.ErrorList
}
