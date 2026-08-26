package vllm

import (
	"reflect"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestAdapterRendersGovernedArguments(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
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
	require.Contains(t, rendered.Args, "--enable-expert-parallel")
	require.Contains(t, rendered.Args, "--enable-eplb")
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
