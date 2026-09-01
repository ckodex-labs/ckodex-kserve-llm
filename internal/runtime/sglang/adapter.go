/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package sglang implements the SGLang runtime adapter.
package sglang

import (
	"strconv"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const engineName = "sglang"

// Adapter renders the SGLang v0.5.18 HTTP server contract.
type Adapter struct{}

func (Adapter) Name() string { return engineName }

func (Adapter) Tier() inferenceruntime.ConformanceTier {
	return inferenceruntime.ConformanceTierServed
}

func (Adapter) Image() inferenceruntime.ImageContract {
	return inferenceruntime.ImageContract{
		Repository: "lmsysorg/sglang",
		Tag:        "v0.5.18",
		Digest:     "sha256:9e148f5ac788e856a06166bd6347a831831eb9fcfab4d1770874823a7c29a1a1",
	}
}

func (Adapter) HealthContract() inferenceruntime.HealthContract {
	return inferenceruntime.HealthContract{Path: "/health"}
}

func (Adapter) MetricsContract() inferenceruntime.MetricsContract {
	return inferenceruntime.MetricsContract{Path: "/metrics", EnableArgs: []string{"--enable-metrics"}}
}

func (Adapter) Capabilities() inferenceruntime.CapabilityMatrix {
	return inferenceruntime.CapabilityMatrix{
		TensorParallel:      inferenceruntime.CapabilitySupported,
		DataParallel:        inferenceruntime.CapabilitySupported,
		LocalDataParallel:   inferenceruntime.CapabilityUnsupported,
		PipelineParallel:    inferenceruntime.CapabilitySupported,
		ExpertParallel:      inferenceruntime.CapabilityUnsupported,
		ExpertLoadBalancing: inferenceruntime.CapabilityUnsupported,
		KVCacheDtype:        inferenceruntime.CapabilityUnsupported,
		CPUOffload:          inferenceruntime.CapabilityUnsupported,
		KVTransfer:          inferenceruntime.CapabilityUnsupported,
		SpeculativeDecoding: inferenceruntime.CapabilityUnsupported,
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
		if parallelism.DataLocal != nil {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "parallelism", "dataLocal"), "SGLang has no local data-parallel argument in this adapter contract"))
		}
		if parallelism.Expert {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "parallelism", "expert"), "SGLang expert parallelism is not mapped by this adapter contract"))
		}
		if parallelism.EPLBEnabled {
			errs = append(errs, field.Forbidden(field.NewPath("spec", "parallelism", "eplbEnabled"), "SGLang EPLB is not mapped by this adapter contract"))
		}
	}
	if service.Spec.KVCache != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "kvCache"), "SGLang KV-cache fields are not mapped by this adapter contract"))
	}
	if service.Spec.SpeculativeDecoding != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "speculativeDecoding"), "SGLang speculative decoding differs from the stable service schema"))
	}
	if service.Spec.Quantization != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "quantization"), "SGLang quantization values are not mapped by this adapter contract"))
	}
	if service.Spec.Prefill != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "prefill"), "SGLang disaggregated prefill is not mapped by this adapter contract"))
	}
	if service.Spec.Worker != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "worker"), "SGLang multi-node workers are not mapped by this adapter contract"))
	}
	return errs
}

func (Adapter) Render(request inferenceruntime.RenderRequest) inferenceruntime.RenderedRuntime {
	args := stripVLLMHardwareArgs(request.ExistingArgs)
	args = ensureLaunchPrefix(args)
	args = appendPair(args, "--model-path", request.ModelPath)
	if request.Service != nil && request.Service.Spec.Model.Name != "" {
		args = appendPair(args, "--served-model-name", request.Service.Spec.Model.Name)
	}
	args = appendPair(args, "--host", defaultString(request.Host, "0.0.0.0"))
	args = appendPair(args, "--port", strconv.FormatInt(int64(defaultPort(request.Port)), 10))
	args = appendSwitch(args, "--enable-metrics")
	if request.Service != nil {
		args = renderParallelism(args, request.Service.Spec.Parallelism)
	}
	return inferenceruntime.RenderedRuntime{Args: args}
}

func ensureLaunchPrefix(args []string) []string {
	if len(args) >= 3 && args[0] == "python3" && args[1] == "-m" && args[2] == "sglang.launch_server" {
		return args
	}
	return append([]string{"python3", "-m", "sglang.launch_server"}, args...)
}

func renderParallelism(args []string, spec *servingv1alpha2.ParallelismSpec) []string {
	if spec == nil {
		return args
	}
	args = appendPositiveSize(args, "--tensor-parallel-size", spec.Tensor)
	args = appendPositiveSize(args, "--data-parallel-size", spec.Data)
	return appendPositiveSize(args, "--pipeline-parallel-size", spec.Pipeline)
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
