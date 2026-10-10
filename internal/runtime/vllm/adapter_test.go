package vllm

import (
	"reflect"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func TestAdapterRendersGovernedArguments(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Model.Name = "served-model"
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Tensor: ptr.To(int32(2)), Data: ptr.To(int32(3)), DataLocal: ptr.To(int32(1)),
		Pipeline: ptr.To(int32(4)), Expert: true, EPLBEnabled: true,
	}
	service.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Dtype: "fp8", SwapSpaceGB: ptr.To(int32(8))}
	service.Spec.SpeculativeDecoding = &servingv1alpha2.SpeculativeDecodingSpec{
		Method: "mtp", NumTokens: ptr.To(int32(5)), DraftModel: "draft/model",
	}
	service.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "awq"}

	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{Service: service, ModelPath: "/models/model"})
	assertArgumentPair(t, rendered.Args, "--tensor-parallel-size", "2")
	assertArgumentPair(t, rendered.Args, "--data-parallel-size", "3")
	assertArgumentPair(t, rendered.Args, "--data-parallel-size-local", "1")
	assertArgumentPair(t, rendered.Args, "--pipeline-parallel-size", "4")
	assertArgumentPair(t, rendered.Args, "--kv-cache-dtype", "fp8")
	assertArgumentPair(t, rendered.Args, "--cpu-offload-gb", "8")
	assertArgumentPair(t, rendered.Args, "--spec-method", "mtp")
	assertArgumentPair(t, rendered.Args, "--spec-tokens", "5")
	assertArgumentPair(t, rendered.Args, "--spec-model", "draft/model")
	assertArgumentPair(t, rendered.Args, "--quantization", "awq")
	assertArgumentPair(t, rendered.Args, "--served-model-name", "served-model")
	require.Contains(t, rendered.Args, "--enable-expert-parallel")
	require.Contains(t, rendered.Args, "--enable-eplb")
}

func TestAdapterPreservesExplicitServedModelName(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service:      service,
		ModelPath:    "/models/model",
		ExistingArgs: []string{"--served-model-name", "custom-name"},
	})
	assertArgumentPair(t, rendered.Args, "--served-model-name", "custom-name")
}

func TestAdapterPreservesExplicitArguments(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{Tensor: ptr.To(int32(4))}
	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service: service, ModelPath: "/models/model",
		ExistingArgs: []string{"--tensor-parallel-size", "8", "--port=9000"},
	})
	assertArgumentPair(t, rendered.Args, "--tensor-parallel-size", "8")
	require.NotContains(t, rendered.Args, "8000")
}

func TestAdapterDeclaresLegacyCapabilitiesHonestly(t *testing.T) {
	adapter := Adapter{}
	require.Equal(t, "vllm", adapter.Name())
	require.Equal(t, inferenceruntime.ConformanceTierServed, adapter.Tier())
	require.True(t, adapter.Image().Valid())
	require.Equal(t, "/health", adapter.HealthContract().Path)
	require.Equal(t, "/metrics", adapter.MetricsContract().Path)
	capabilities := adapter.Capabilities()
	require.Equal(t, inferenceruntime.CapabilityEmulated, capabilities.KVTransfer)
	require.Equal(t, inferenceruntime.CapabilityEmulated, capabilities.LoRAHotSwap)
	value := reflect.ValueOf(capabilities)
	for index := 0; index < value.NumField(); index++ {
		require.NotEmpty(t, value.Field(index).String(), value.Type().Field(index).Name)
	}
}

func TestAdapterRejectsWrongEngine(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = "other"
	require.NotEmpty(t, (Adapter{}).Validate(service))
	require.NotEmpty(t, (Adapter{}).Validate(nil))
}

func TestAdapterRejectsIgnoredCheckpointPath(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Quantization = &servingv1alpha2.QuantizationSpec{CheckpointPath: "/models/checkpoint"}
	require.Equal(t, "spec.quantization.checkpointPath", (Adapter{}).Validate(service)[0].Field)
}

func TestAdapterRejectsGGUFWithoutVerifiedRuntimePlugin(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "gguf"}
	require.Equal(t, "spec.quantization.method", (Adapter{}).Validate(service)[0].Field)
}

func TestAdapterImageContractV0310(t *testing.T) {
	adapter := Adapter{}
	image := adapter.Image()
	require.Equal(t, "vllm/vllm-openai", image.Repository)
	require.Equal(t, "v0.31.0", image.Tag)
	require.Equal(t, "sha256:c1c9f6fd5c109ba7f0546a59f5b2f15fb87f64c77782e90a27b648b42a8e67c3", image.Digest)
	require.True(t, image.Valid())
	require.Equal(t, "vllm/vllm-openai:v0.31.0@sha256:c1c9f6fd5c109ba7f0546a59f5b2f15fb87f64c77782e90a27b648b42a8e67c3", image.Reference())
}

func TestAdapterRendersNVFP4KVCache(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Dtype: "nvfp4"}

	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{Service: service, ModelPath: "/models/model"})
	assertArgumentPair(t, rendered.Args, "--kv-cache-dtype", "nvfp4")
}

func TestAdapterRendersModernFeaturesViaAnnotations(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Annotations = map[string]string{
		"serving.ckodex.com/fast-restart":            "true",
		"serving.ckodex.com/flashmla":                "true",
		"serving.ckodex.com/mega-gate":               "true",
		"serving.ckodex.com/trust-request-mm-kwargs": "true",
	}

	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{Service: service, ModelPath: "/models/model"})
	assertArgumentPair(t, rendered.Args, "--load-format", "ipc_cache")
	require.Contains(t, rendered.Args, "--enable-fast-restart")
	require.Contains(t, rendered.Args, "--enable-flashmla")
	require.Contains(t, rendered.Args, "--enable-mega-gate")
	require.Contains(t, rendered.Args, "--trust-request-mm-kwargs")
}

func TestAdapterPreservesExplicitModernFlags(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Annotations = map[string]string{
		"serving.ckodex.com/fast-restart":            "true",
		"serving.ckodex.com/flashmla":                "true",
		"serving.ckodex.com/mega-gate":               "true",
		"serving.ckodex.com/trust-request-mm-kwargs": "true",
	}
	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service:   service,
		ModelPath: "/models/model",
		ExistingArgs: []string{
			"--load-format=custom",
			"--enable-flashmla",
			"--enable-mega-gate",
			"--trust-request-mm-kwargs",
		},
	})
	require.Contains(t, rendered.Args, "--load-format=custom")
	require.NotContains(t, rendered.Args, "ipc_cache")
	require.Contains(t, rendered.Args, "--enable-flashmla")
	require.Contains(t, rendered.Args, "--enable-mega-gate")
	require.Contains(t, rendered.Args, "--trust-request-mm-kwargs")
}

func TestAdapterStripsRemovedTokenizerModeSlow(t *testing.T) {
	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		ModelPath: "/models/model",
		ExistingArgs: []string{
			"--tokenizer-mode", "slow",
			"--max-model-len", "2048",
			"--tokenizer-mode=slow",
		},
	})
	require.NotContains(t, rendered.Args, "slow")
	require.NotContains(t, rendered.Args, "--tokenizer-mode")
	require.NotContains(t, rendered.Args, "--tokenizer-mode=slow")
	assertArgumentPair(t, rendered.Args, "--max-model-len", "2048")
}

func TestAdapterRejectsRemovedTokenizerModeSlowInValidation(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Template.Spec.Containers = []corev1.Container{
		{Args: []string{"--tokenizer-mode", "slow"}},
	}
	errs := (Adapter{}).Validate(service)
	require.Len(t, errs, 1)
	require.Equal(t, "spec.template.spec.containers[0].args", errs[0].Field)

	serviceEquals := &servingv1alpha2.LLMInferenceService{}
	serviceEquals.Spec.Template.Spec.Containers = []corev1.Container{
		{Args: []string{"--tokenizer-mode=slow"}},
	}
	errsEquals := (Adapter{}).Validate(serviceEquals)
	require.Len(t, errsEquals, 1)
	require.Equal(t, "spec.template.spec.containers[0].args", errsEquals[0].Field)
}

func TestAdapterValidatesKVCacheDtype(t *testing.T) {
	adapter := Adapter{}
	for _, valid := range []string{"auto", "fp8", "fp16", "bf16", "nvfp4"} {
		service := &servingv1alpha2.LLMInferenceService{}
		service.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Dtype: valid}
		require.Empty(t, adapter.Validate(service), "expected %s to be valid", valid)
	}

	serviceInvalid := &servingv1alpha2.LLMInferenceService{}
	serviceInvalid.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Dtype: "int4"}
	errs := adapter.Validate(serviceInvalid)
	require.Len(t, errs, 1)
	require.Equal(t, "spec.kvCache.dtype", errs[0].Field)
}

func assertArgumentPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for index := 0; index < len(args)-1; index++ {
		if args[index] == flag {
			require.Equal(t, value, args[index+1])
			return
		}
	}
	t.Fatalf("argument %s not found in %v", flag, args)
}
