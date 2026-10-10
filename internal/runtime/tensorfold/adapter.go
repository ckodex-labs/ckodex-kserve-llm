/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package tensorfold implements the TensorFold runtime adapter.
package tensorfold

import (
	"strconv"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const engineName = "tensorfold"

// Adapter renders the TensorFold native server contract.
type Adapter struct{}

func (Adapter) Name() string { return engineName }

func (Adapter) Tier() inferenceruntime.ConformanceTier {
	return inferenceruntime.ConformanceTierServed
}

func (Adapter) Image() inferenceruntime.ImageContract {
	return inferenceruntime.ImageContract{
		Repository: "tensorfold/tensorfold",
		Tag:        "v1.0.0",
		Digest:     "sha256:d8e5f2a1b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0",
	}
}

func (Adapter) HealthContract() inferenceruntime.HealthContract {
	return inferenceruntime.HealthContract{Path: "/health"}
}

func (Adapter) MetricsContract() inferenceruntime.MetricsContract {
	return inferenceruntime.MetricsContract{Path: "/metrics", EnableArgs: []string{"--metrics-open"}}
}

func (Adapter) Capabilities() inferenceruntime.CapabilityMatrix {
	return inferenceruntime.CapabilityMatrix{
		TensorParallel:      inferenceruntime.CapabilityUnsupported,
		DataParallel:        inferenceruntime.CapabilitySupported,
		LocalDataParallel:   inferenceruntime.CapabilityUnsupported,
		PipelineParallel:    inferenceruntime.CapabilityUnsupported,
		ExpertParallel:      inferenceruntime.CapabilityUnsupported,
		ExpertLoadBalancing: inferenceruntime.CapabilityUnsupported,
		KVCacheDtype:        inferenceruntime.CapabilityUnsupported,
		CPUOffload:          inferenceruntime.CapabilityUnsupported,
		KVTransfer:          inferenceruntime.CapabilityUnsupported,
		SpeculativeDecoding: inferenceruntime.CapabilitySupported,
		Quantization:        inferenceruntime.CapabilityUnsupported,
		LoRAHotSwap:         inferenceruntime.CapabilityUnsupported,
	}
}

func (Adapter) Validate(service *servingv1alpha2.LLMInferenceService) field.ErrorList {
	if service == nil {
		return field.ErrorList{field.Required(field.NewPath("spec"), "service is required")}
	}
	errs := field.ErrorList{}
	if service.Spec.Engine != engineName {
		errs = append(errs, field.NotSupported(field.NewPath("spec", "engine"), service.Spec.Engine, []string{engineName}))
	}
	if parallelism := service.Spec.Parallelism; parallelism != nil {
		if parallelism.Tensor != nil {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "parallelism", "tensor"), "TensorFold tensor parallelism is not mapped by this adapter contract"))
		}
		if parallelism.DataLocal != nil {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "parallelism", "dataLocal"), "TensorFold has no local data-parallel argument in this adapter contract"))
		}
		if parallelism.Pipeline != nil {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "parallelism", "pipeline"), "TensorFold pipeline parallelism is not mapped by this adapter contract"))
		}
		if parallelism.Expert {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "parallelism", "expert"), "TensorFold expert parallelism is not mapped by this adapter contract"))
		}
		if parallelism.EPLBEnabled {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "parallelism", "eplbEnabled"), "TensorFold EPLB is not mapped by this adapter contract"))
		}
	}
	if service.Spec.KVCache != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "kvCache"), "TensorFold KV-cache fields are not mapped by this adapter contract"))
	}
	if service.Spec.Quantization != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "quantization"), "TensorFold quantization is checkpoint-native and not mapped by this adapter contract"))
	}
	if service.Spec.Prefill != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "prefill"), "TensorFold disaggregated prefill is not mapped by this adapter contract"))
	}
	if service.Spec.Worker != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "worker"), "TensorFold multi-node workers are not mapped by this adapter contract"))
	}
	if spec := service.Spec.SpeculativeDecoding; spec != nil {
		if spec.Method != "" && spec.Method != "mtp" && spec.Method != "draft" {
			errs = append(errs, field.NotSupported(
				field.NewPath("spec", "speculativeDecoding", "method"),
				spec.Method,
				[]string{"mtp", "draft"},
			))
		}
	}
	return errs
}

func (Adapter) Render(request inferenceruntime.RenderRequest) inferenceruntime.RenderedRuntime {
	args := stripVLLMHardwareArgs(request.ExistingArgs)
	args = ensureLaunchPrefix(args, request.ModelPath)
	if request.Service != nil && request.Service.Spec.Model.Name != "" {
		args = appendPair(args, "--name", request.Service.Spec.Model.Name)
	}
	args = appendPair(args, "--host", defaultString(request.Host, "0.0.0.0"))
	args = appendPair(args, "--port", strconv.FormatInt(int64(defaultPort(request.Port)), 10))
	args = appendSwitch(args, "--metrics-open")
	if request.Service == nil {
		return inferenceruntime.RenderedRuntime{Args: args}
	}
	args = renderParallelism(args, request.Service.Spec.Parallelism)
	args = renderSpeculative(args, request.Service.Spec.SpeculativeDecoding)
	return inferenceruntime.RenderedRuntime{Args: args}
}

func ensureLaunchPrefix(args []string, modelPath string) []string {
	if len(args) >= 2 && (args[0] == "tensorfold" || args[0] == "tensorfold-native") && args[1] == "serve" {
		if len(args) >= 3 && !strings.HasPrefix(args[2], "-") {
			return args
		}
		if modelPath != "" {
			return append(args[:2], append([]string{modelPath}, args[2:]...)...)
		}
		return args
	}
	prefix := []string{"tensorfold", "serve"}
	if modelPath != "" {
		prefix = append(prefix, modelPath)
	}
	return append(prefix, args...)
}

func renderParallelism(args []string, spec *servingv1alpha2.ParallelismSpec) []string {
	if spec == nil {
		return args
	}
	return appendPositiveSize(args, "--parallel", spec.Data)
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
		args = appendPair(args, "--draft-model", spec.DraftModel)
	}
	return args
}

func stripVLLMHardwareArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--device" || argument == "--max-model-len" {
			index++
			continue
		}
		if strings.HasPrefix(argument, "--device=") || strings.HasPrefix(argument, "--max-model-len=") {
			continue
		}
		filtered = append(filtered, argument)
	}
	return filtered
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
