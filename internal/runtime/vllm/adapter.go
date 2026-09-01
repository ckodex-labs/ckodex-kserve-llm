/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package vllm implements the vLLM runtime adapter.
package vllm

import (
	"strconv"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Adapter renders the vLLM 0.28 command-line contract.
type Adapter struct{}

func (Adapter) Name() string { return "vllm" }

// Tier remains served-only until metrics and receipt contracts move behind this seam.
func (Adapter) Tier() inferenceruntime.ConformanceTier {
	return inferenceruntime.ConformanceTierServed
}

func (Adapter) Image() inferenceruntime.ImageContract {
	return inferenceruntime.ImageContract{
		Repository: "vllm/vllm-openai",
		Tag:        "v0.28.0",
		Digest:     "sha256:61fc8a896b0a4fbbbdc063bc4b0dbc25ce98e02b5050c24aeb7830ac02039b14",
	}
}

func (Adapter) HealthContract() inferenceruntime.HealthContract {
	return inferenceruntime.HealthContract{Path: "/health"}
}

func (Adapter) MetricsContract() inferenceruntime.MetricsContract {
	return inferenceruntime.MetricsContract{Path: "/metrics"}
}

func (Adapter) Capabilities() inferenceruntime.CapabilityMatrix {
	return inferenceruntime.CapabilityMatrix{
		TensorParallel:      inferenceruntime.CapabilitySupported,
		DataParallel:        inferenceruntime.CapabilitySupported,
		LocalDataParallel:   inferenceruntime.CapabilitySupported,
		PipelineParallel:    inferenceruntime.CapabilitySupported,
		ExpertParallel:      inferenceruntime.CapabilitySupported,
		ExpertLoadBalancing: inferenceruntime.CapabilitySupported,
		KVCacheDtype:        inferenceruntime.CapabilitySupported,
		CPUOffload:          inferenceruntime.CapabilitySupported,
		KVTransfer:          inferenceruntime.CapabilityEmulated,
		SpeculativeDecoding: inferenceruntime.CapabilitySupported,
		Quantization:        inferenceruntime.CapabilitySupported,
		LoRAHotSwap:         inferenceruntime.CapabilityEmulated,
	}
}

func (Adapter) Validate(service *servingv1alpha2.LLMInferenceService) field.ErrorList {
	if service == nil {
		return field.ErrorList{field.Required(field.NewPath("spec"), "service is required")}
	}
	if service.Spec.Engine != "" && service.Spec.Engine != "vllm" {
		return field.ErrorList{field.NotSupported(field.NewPath("spec", "engine"), service.Spec.Engine, []string{"vllm"})}
	}
	if service.Spec.Quantization != nil && service.Spec.Quantization.CheckpointPath != "" {
		return field.ErrorList{field.Forbidden(field.NewPath("spec", "quantization", "checkpointPath"), "checkpoint paths are not consumed by vLLM")}
	}
	if service.Spec.Quantization != nil && service.Spec.Quantization.Method == "gguf" {
		return field.ErrorList{field.NotSupported(
			field.NewPath("spec", "quantization", "method"),
			service.Spec.Quantization.Method,
			[]string{"awq", "gptq", "bitsandbytes", "fp8"},
		)}
	}
	return nil
}

func (Adapter) Render(request inferenceruntime.RenderRequest) inferenceruntime.RenderedRuntime {
	args := append([]string(nil), request.ExistingArgs...)
	args = prependPair(args, "--model", request.ModelPath)
	if request.Service != nil && request.Service.Spec.Model.Name != "" {
		args = appendPair(args, "--served-model-name", request.Service.Spec.Model.Name)
	}
	args = appendPair(args, "--host", defaultString(request.Host, "0.0.0.0"))
	args = appendPair(args, "--port", strconv.FormatInt(int64(defaultPort(request.Port)), 10))
	if request.Service == nil {
		return inferenceruntime.RenderedRuntime{Args: args}
	}
	args = renderParallelism(args, request.Service.Spec.Parallelism)
	args = renderCache(args, request.Service.Spec.KVCache)
	args = renderSpeculative(args, request.Service.Spec.SpeculativeDecoding)
	args = renderQuantization(args, request.Service.Spec.Quantization)
	return inferenceruntime.RenderedRuntime{Args: args}
}

func renderParallelism(args []string, spec *servingv1alpha2.ParallelismSpec) []string {
	if spec == nil {
		return args
	}
	args = appendPositiveSize(args, "--tensor-parallel-size", spec.Tensor)
	args = appendPositiveSize(args, "--data-parallel-size", spec.Data)
	args = appendPositiveSize(args, "--data-parallel-size-local", spec.DataLocal)
	args = appendPositiveSize(args, "--pipeline-parallel-size", spec.Pipeline)
	if spec.Expert {
		args = appendSwitch(args, "--enable-expert-parallel")
	}
	if spec.EPLBEnabled {
		args = appendSwitch(args, "--enable-eplb")
	}
	return args
}

func renderCache(args []string, spec *servingv1alpha2.KVCacheSpec) []string {
	if spec == nil {
		return args
	}
	if spec.Dtype != "" && spec.Dtype != "auto" {
		args = appendPair(args, "--kv-cache-dtype", spec.Dtype)
	}
	return appendPositiveSize(args, "--cpu-offload-gb", spec.SwapSpaceGB)
}

func renderSpeculative(args []string, spec *servingv1alpha2.SpeculativeDecodingSpec) []string {
	if spec == nil {
		return args
	}
	if spec.Method != "" {
		args = appendPair(args, "--spec-method", spec.Method)
	}
	args = appendPositiveSize(args, "--spec-tokens", spec.NumTokens)
	if spec.DraftModel != "" {
		args = appendPair(args, "--spec-model", spec.DraftModel)
	}
	return args
}

func renderQuantization(args []string, spec *servingv1alpha2.QuantizationSpec) []string {
	if spec == nil || spec.Method == "" || spec.Method == "gguf" {
		return args
	}
	return appendPair(args, "--quantization", spec.Method)
}

func appendPositiveSize(args []string, flag string, value *int32) []string {
	if value == nil || *value <= 0 {
		return args
	}
	return appendPair(args, flag, strconv.FormatInt(int64(*value), 10))
}

func appendPair(args []string, flag, value string) []string {
	if hasArgument(args, flag) {
		return args
	}
	return append(args, flag, value)
}

func prependPair(args []string, flag, value string) []string {
	if hasArgument(args, flag) {
		return args
	}
	return append([]string{flag, value}, args...)
}

func appendSwitch(args []string, flag string) []string {
	if hasArgument(args, flag) {
		return args
	}
	return append(args, flag)
}

func hasArgument(args []string, target string) bool {
	for _, argument := range args {
		if argument == target || strings.HasPrefix(argument, target+"=") {
			return true
		}
	}
	return false
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultPort(port int32) int32 {
	if port <= 0 {
		return 8000
	}
	return port
}
