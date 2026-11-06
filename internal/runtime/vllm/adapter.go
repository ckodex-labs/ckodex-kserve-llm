/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package vllm implements the vLLM runtime adapter.
package vllm

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const RuntimeAcceptanceVersion = "vllm-v0.33.0"

// Adapter renders the vLLM 0.33 command-line contract.
type Adapter struct{}

func (Adapter) Name() string { return "vllm" }

// Tier remains served-only until metrics and receipt contracts move behind this seam.
func (Adapter) Tier() inferenceruntime.ConformanceTier {
	return inferenceruntime.ConformanceTierServed
}

func (Adapter) Image() inferenceruntime.ImageContract {
	return inferenceruntime.ImageContract{
		Repository: "vllm/vllm-openai",
		Tag:        "v0.33.0",
		Digest:     "sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e",
	}
}

func (Adapter) HealthContract() inferenceruntime.HealthContract {
	return inferenceruntime.HealthContract{Path: "/health"}
}

func (Adapter) MetricsContract() inferenceruntime.MetricsContract {
	return inferenceruntime.MetricsContract{Path: "/metrics"}
}

// Capabilities declares the vLLM v0.33.0 capability matrix.
// The upstream release retains NVFP4, expert-parallel, fast-restart, and
// FlashMLA support. Hardware-specific execution still requires local evidence.
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
	errs := validateSpecOptions(service)
	for i, container := range service.Spec.Template.Spec.Containers {
		if i != 0 {
			continue
		}
		errs = append(errs, validatePrimaryContainer(&container, i)...)
	}
	return errs
}

func validateSpecOptions(service *servingv1alpha2.LLMInferenceService) field.ErrorList {
	errs := field.ErrorList{}
	if service.Spec.Engine != "" && service.Spec.Engine != "vllm" {
		errs = append(errs, field.NotSupported(field.NewPath("spec", "engine"), service.Spec.Engine, []string{"vllm"}))
	}
	if service.Spec.Quantization != nil && service.Spec.Quantization.CheckpointPath != "" {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "quantization", "checkpointPath"), "checkpoint paths are not consumed by vLLM"))
	}
	if service.Spec.SpeculativeDecoding != nil && service.Spec.SpeculativeDecoding.Method != "" {
		switch strings.ToLower(service.Spec.SpeculativeDecoding.Method) {
		case "medusa", "mlp":
			errs = append(errs, field.NotSupported(
				field.NewPath("spec", "speculativeDecoding", "method"),
				service.Spec.SpeculativeDecoding.Method,
				[]string{"mtp", "eagle", "draft_model", "xpress"},
			))
		}
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
	return errs
}

func validatePrimaryContainer(container *corev1.Container, i int) field.ErrorList {
	errs := field.ErrorList{}
	if len(container.Command) > 0 {
		errs = append(errs, field.Forbidden(
			field.NewPath("spec", "template", "spec", "containers").Index(i).Child("command"),
			"the vLLM adapter owns the image entrypoint so rendered runtime and security settings cannot be bypassed",
		))
	}
	errs = append(errs, validateContainerEnv(container.Env, i)...)
	errs = append(errs, validateContainerArgs(container.Args, i)...)
	return errs
}

func validateContainerEnv(envs []corev1.EnvVar, i int) field.ErrorList {
	errs := field.ErrorList{}
	for _, env := range envs {
		if env.Name == "VLLM_SERVER_DEV_MODE" && (env.Value != "0" || env.ValueFrom != nil) {
			errs = append(errs, field.Forbidden(
				field.NewPath("spec", "template", "spec", "containers").Index(i).Child("env"),
				"VLLM_SERVER_DEV_MODE must remain unset or explicitly set to 0",
			))
		}
		if env.Name == "VLLM_MAX_N_SEQUENCES" {
			maxSequences, err := strconv.Atoi(env.Value)
			if env.ValueFrom != nil || err != nil || maxSequences < 1 || maxSequences > 128 {
				errs = append(errs, field.Forbidden(
					field.NewPath("spec", "template", "spec", "containers").Index(i).Child("env"),
					"VLLM_MAX_N_SEQUENCES must be an explicit integer from 1 through 128",
				))
			}
		}
		if env.Name == "VLLM_MEDIA_URL_ALLOW_REDIRECTS" && (env.Value != "0" || env.ValueFrom != nil) {
			errs = append(errs, field.Forbidden(
				field.NewPath("spec", "template", "spec", "containers").Index(i).Child("env"),
				"VLLM_MEDIA_URL_ALLOW_REDIRECTS must be explicitly disabled with value 0",
			))
		}
	}
	return errs
}

func validateContainerArgs(args []string, i int) field.ErrorList {
	errs := field.ErrorList{}
	for j, arg := range args {
		option := normalizeLongOption(arg)
		if option == "--tokenizer-mode=slow" || (option == "--tokenizer-mode" && j+1 < len(args) && args[j+1] == "slow") {
			errs = append(errs, field.Forbidden(
				field.NewPath("spec", "template", "spec", "containers").Index(i).Child("args"),
				"tokenizer_mode 'slow' is not supported by the vLLM v0.33.0 contract; use 'auto' or 'mistral'",
			))
		}
		if matchesLongOption(option, "--mamba-cache-mode") {
			errs = append(errs, field.Forbidden(
				field.NewPath("spec", "template", "spec", "containers").Index(i).Child("args").Index(j),
				"--mamba-cache-mode was deprecated and has no effect in vLLM v0.33.0",
			))
		}
		if option == "--no-disable-fastapi-docs" || option == "--enable-fastapi-docs" {
			errs = append(errs, field.Forbidden(
				field.NewPath("spec", "template", "spec", "containers").Index(i).Child("args").Index(j),
				"FastAPI documentation endpoints must remain disabled",
			))
		}
		if matchesLongOption(option, "--allowed-local-media-path") {
			pathValue, ok := argumentValue(args, j, "--allowed-local-media-path")
			if !ok || pathValue != "" {
				errs = append(errs, field.Forbidden(
					field.NewPath("spec", "template", "spec", "containers").Index(i).Child("args").Index(j),
					"local media access must remain disabled because it exposes server files to API clients",
				))
			}
		}
		errs = append(errs, validateCORSArgs(args, option, i, j)...)
	}
	return errs
}

func validateCORSArgs(args []string, option string, i, j int) field.ErrorList {
	errs := field.ErrorList{}
	flag := ""
	for _, candidate := range []string{"--allowed-origins", "--allowed-methods", "--allowed-headers"} {
		if matchesLongOption(option, candidate) {
			optionName, _, _ := strings.Cut(option, "=")
			if optionName != candidate {
				return field.ErrorList{field.Forbidden(
					field.NewPath("spec", "template", "spec", "containers").Index(i).Child("args").Index(j),
					"use the full CORS option name so the adapter can preserve the configured value",
				)}
			}
			flag = candidate
			break
		}
	}
	if flag == "" {
		return errs
	}
	valuesJSON, ok := argumentValue(args, j, flag)
	path := field.NewPath("spec", "template", "spec", "containers").Index(i).Child("args").Index(j)
	if !ok {
		return field.ErrorList{field.Required(path, "CORS settings must be JSON arrays; use [] to disable browser CORS")}
	}
	var values []string
	if err := json.Unmarshal([]byte(valuesJSON), &values); err != nil {
		return field.ErrorList{field.Invalid(path, valuesJSON, "CORS settings must be JSON string arrays")}
	}
	for _, value := range values {
		if strings.Contains(value, "*") {
			errs = append(errs, field.Forbidden(path, "wildcard CORS values are forbidden; specify exact trusted origins, methods, or headers"))
			continue
		}
		if flag == "--allowed-origins" && value == "" && len(values) == 1 {
			continue
		}
		if flag == "--allowed-origins" && !validCORSOrigin(value) {
			errs = append(errs, field.Invalid(path, value, "CORS origins must be explicit HTTP or HTTPS origins without credentials or paths"))
		}
		if flag == "--allowed-methods" && value != "GET" && value != "POST" {
			errs = append(errs, field.NotSupported(path, value, []string{"GET", "POST"}))
		}
		if flag == "--allowed-headers" && !validHeaderName(value) {
			errs = append(errs, field.Invalid(path, value, "CORS headers must be valid HTTP header names"))
		}
	}
	return errs
}

func argumentValue(args []string, index int, flag string) (value string, ok bool) {
	argument := normalizeLongOption(args[index])
	name, value, hasValue := strings.Cut(argument, "=")
	if hasValue && matchesLongOption(name, flag) {
		return value, true
	}
	if !matchesLongOption(argument, flag) || index+1 >= len(args) {
		return "", false
	}
	return args[index+1], true
}

func validCORSOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range []byte(name) {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		default:
			return false
		}
	}
	return true
}

func (Adapter) Render(request inferenceruntime.RenderRequest) inferenceruntime.RenderedRuntime {
	args := stripRemovedArgs(request.ExistingArgs)
	args = appendPair(args, "--allowed-origins", "[]")
	args = appendPair(args, "--allowed-methods", `["GET","POST"]`)
	args = appendPair(args, "--allowed-headers", `["Authorization","Content-Type","OpenAI-Organization","OpenAI-Project","X-CKodex-Adapter","X-Request-ID"]`)
	args = appendDefaultSwitchPair(args, "--disable-fastapi-docs", "--no-disable-fastapi-docs")
	args = appendDefaultSwitchPair(args, "--no-trust-remote-code", "--trust-remote-code")
	args = appendDefaultSwitchPair(args, "--no-allow-credentials", "--allow-credentials")
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

func appendDefaultSwitchPair(args []string, defaultFlag, oppositeFlag string) []string {
	if hasArgument(args, defaultFlag) || hasArgument(args, oppositeFlag) {
		return args
	}
	return append(args, defaultFlag)
}

func hasArgument(args []string, target string) bool {
	for _, argument := range args {
		argument = normalizeLongOption(argument)
		if argument == target || strings.HasPrefix(argument, target+"=") {
			return true
		}
	}
	return false
}

func normalizeLongOption(argument string) string {
	if !strings.HasPrefix(argument, "--") {
		return argument
	}
	name, value, hasValue := strings.Cut(argument, "=")
	name = strings.ReplaceAll(name, "_", "-")
	if !hasValue {
		return name
	}
	return name + "=" + value
}

func matchesLongOption(argument, flag string) bool {
	name, _, _ := strings.Cut(normalizeLongOption(argument), "=")
	return name == flag || (len(name) < len(flag) && strings.HasPrefix(flag, name))
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
		if matchesLongOption(argument, "--mamba-cache-mode") {
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
