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

// Adapter renders the vLLM 0.31 command-line contract.
type Adapter struct{}

func (Adapter) Name() string { return "vllm" }

// Tier remains served-only until metrics and receipt contracts move behind this seam.
func (Adapter) Tier() inferenceruntime.ConformanceTier {
	return inferenceruntime.ConformanceTierServed
}

func (Adapter) Image() inferenceruntime.ImageContract {
	return inferenceruntime.ImageContract{
		Repository: "vllm/vllm-openai",
		Tag:        "v0.31.0",
		Digest:     "sha256:c1c9f6fd5c109ba7f0546a59f5b2f15fb87f64c77782e90a27b648b42a8e67c3",
	}
}

func (Adapter) HealthContract() inferenceruntime.HealthContract {
	return inferenceruntime.HealthContract{Path: "/health"}
}

func (Adapter) MetricsContract() inferenceruntime.MetricsContract {
	return inferenceruntime.MetricsContract{Path: "/metrics"}
}

// Capabilities declares the vLLM v0.31.0 capability matrix.
// vLLM v0.31.0 enhances KVCacheDtype with NVFP4, ExpertParallel with MoE Mega-Gate,
// and features fast restart weight preloading and FlashMLA attention.
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
	errs := field.ErrorList{}
	if service.Spec.Engine != "" && service.Spec.Engine != "vllm" {
		errs = append(errs, field.NotSupported(field.NewPath("spec", "engine"), service.Spec.Engine, []string{"vllm"}))
	}
	if service.Spec.Quantization != nil && service.Spec.Quantization.CheckpointPath != "" {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "quantization", "checkpointPath"), "checkpoint paths are not consumed by vLLM"))
	}
	if service.Spec.Quantization != nil && service.Spec.Quantization.Method == "gguf" {
		errs = append(errs, field.NotSupported(
			field.NewPath("spec", "quantization", "method"),
			service.Spec.Quantization.Method,
			[]string{"awq", "gptq", "bitsandbytes", "fp8"},
		))
	}
	if service.Spec.KVCache != nil && service.Spec.KVCache.Dtype != "" {
		switch service.Spec.KVCache.Dtype {
		case "auto", "fp8", "fp16", "bf16", "nvfp4":
		default:
			errs = append(errs, field.NotSupported(
				field.NewPath("spec", "kvCache", "dtype"),
				service.Spec.KVCache.Dtype,
				[]string{"auto", "fp8", "fp16", "bf16", "nvfp4"},
			))
		}
	}
	for i, container := range service.Spec.Template.Spec.Containers {
		for j, arg := range container.Args {
			if arg == "--tokenizer-mode=slow" || (arg == "--tokenizer-mode" && j+1 < len(container.Args) && container.Args[j+1] == "slow") {
				errs = append(errs, field.Forbidden(
					field.NewPath("spec", "template", "spec", "containers").Index(i).Child("args"),
					"tokenizer_mode 'slow' was removed in vLLM v0.31.0; use 'auto' or 'mistral'",
				))
			}
		}
	}
	return errs
}

func (Adapter) Render(request inferenceruntime.RenderRequest) inferenceruntime.RenderedRuntime {
	args := stripRemovedArgs(request.ExistingArgs)
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
	args = renderModernFeatures(args, request.Service)
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

func stripRemovedArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--tokenizer-mode=slow" {
			continue
		}
		if argument == "--tokenizer-mode" && index+1 < len(args) && args[index+1] == "slow" {
			index++
			continue
		}
		filtered = append(filtered, argument)
	}
	return filtered
}

func getAnnotation(service *servingv1alpha2.LLMInferenceService, key string) string {
	if service == nil {
		return ""
	}
	if service.Annotations != nil && service.Annotations[key] != "" {
		return service.Annotations[key]
	}
	if service.Spec.Template.Annotations != nil && service.Spec.Template.Annotations[key] != "" {
		return service.Spec.Template.Annotations[key]
	}
	return ""
}

func renderModernFeatures(args []string, service *servingv1alpha2.LLMInferenceService) []string {
	if service == nil {
		return args
	}
	if getAnnotation(service, "serving.ckodex.com/fast-restart") == "true" ||
		getAnnotation(service, "serving.ckodex.com/preload-weight-cache") == "true" {
		args = appendPair(args, "--load-format", "ipc_cache")
		args = appendSwitch(args, "--enable-fast-restart")
	}
	if getAnnotation(service, "serving.ckodex.com/flashmla") == "true" ||
		getAnnotation(service, "serving.ckodex.com/enable-flashmla") == "true" {
		args = appendSwitch(args, "--enable-flashmla")
	}
	if getAnnotation(service, "serving.ckodex.com/mega-gate") == "true" ||
		getAnnotation(service, "serving.ckodex.com/moe-mega-gate") == "true" {
		args = appendSwitch(args, "--enable-mega-gate")
	}
	if getAnnotation(service, "serving.ckodex.com/trust-request-mm-kwargs") == "true" {
		args = appendSwitch(args, "--trust-request-mm-kwargs")
	}
	return args
}
