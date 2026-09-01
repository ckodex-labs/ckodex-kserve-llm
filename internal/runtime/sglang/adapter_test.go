package sglang

import (
	"reflect"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestAdapterRendersVerifiedServerContract(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	service.Spec.Model.Name = "served-model"
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Tensor: ptr.To(int32(2)), Data: ptr.To(int32(3)), Pipeline: ptr.To(int32(4)),
	}

	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service: service, ModelPath: "/models/model", Host: "127.0.0.1", Port: 9000,
		ExistingArgs: []string{"--device", "mps", "--max-model-len=4096"},
	})
	require.Equal(t, []string{"python3", "-m", "sglang.launch_server"}, rendered.Args[:3])
	assertArgumentPair(t, rendered.Args, "--model-path", "/models/model")
	assertArgumentPair(t, rendered.Args, "--served-model-name", "served-model")
	assertArgumentPair(t, rendered.Args, "--host", "127.0.0.1")
	assertArgumentPair(t, rendered.Args, "--port", "9000")
	assertArgumentPair(t, rendered.Args, "--tensor-parallel-size", "2")
	assertArgumentPair(t, rendered.Args, "--data-parallel-size", "3")
	assertArgumentPair(t, rendered.Args, "--pipeline-parallel-size", "4")
	require.Contains(t, rendered.Args, "--enable-metrics")
	require.NotContains(t, rendered.Args, "--device")
	require.NotContains(t, rendered.Args, "mps")
	require.NotContains(t, rendered.Args, "--max-model-len=4096")
}

func TestAdapterPreservesExplicitEndpointArguments(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service: service, ModelPath: "/models/model",
		ExistingArgs: []string{"--host", "custom-host", "--port=31000", "--enable-metrics"},
	})
	assertArgumentPair(t, rendered.Args, "--host", "custom-host")
	require.Contains(t, rendered.Args, "--port=31000")
	require.Equal(t, 1, countArgument(rendered.Args, "--enable-metrics"))
}

func TestAdapterDoesNotDuplicateExplicitLaunchPrefix(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service: service, ModelPath: "/models/model",
		ExistingArgs: []string{"python3", "-m", "sglang.launch_server", "--host", "0.0.0.0"},
	})
	require.Equal(t, []string{"python3", "-m", "sglang.launch_server"}, rendered.Args[:3])
	require.Equal(t, 1, countArgument(rendered.Args, "sglang.launch_server"))
}

func TestAdapterDeclaresImageEndpointsAndCapabilities(t *testing.T) {
	adapter := Adapter{}
	require.Equal(t, engineName, adapter.Name())
	require.Equal(t, inferenceruntime.ConformanceTierServed, adapter.Tier())
	require.True(t, adapter.Image().Valid())
	require.Equal(t, "lmsysorg/sglang:v0.5.18@sha256:9e148f5ac788e856a06166bd6347a831831eb9fcfab4d1770874823a7c29a1a1", adapter.Image().Reference())
	require.Equal(t, "/health", adapter.HealthContract().Path)
	require.Equal(t, "/metrics", adapter.MetricsContract().Path)
	require.Equal(t, []string{"--enable-metrics"}, adapter.MetricsContract().EnableArgs)

	value := reflect.ValueOf(adapter.Capabilities())
	for index := 0; index < value.NumField(); index++ {
		require.NotEmpty(t, value.Field(index).String(), value.Type().Field(index).Name)
	}
}

func TestAdapterRejectsUnmappedServiceFields(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{DataLocal: ptr.To(int32(1)), Expert: true, EPLBEnabled: true}
	service.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Dtype: "fp8"}
	service.Spec.SpeculativeDecoding = &servingv1alpha2.SpeculativeDecodingSpec{Method: "ngram"}
	service.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "fp8"}
	service.Spec.Prefill = &servingv1alpha2.PrefillSpec{}
	service.Spec.Worker = &servingv1alpha2.WorkerSpec{}

	errs := (Adapter{}).Validate(service)
	require.Len(t, errs, 8)
	require.Equal(t, "spec.parallelism.dataLocal", errs[0].Field)
	require.Equal(t, "spec.worker", errs[7].Field)
}

func TestAdapterRejectsWrongEngineAndNilService(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = "vllm"
	require.NotEmpty(t, (Adapter{}).Validate(service))
	require.NotEmpty(t, (Adapter{}).Validate(nil))
}

func assertArgumentPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for index := 0; index < len(args)-1; index++ {
		if args[index] == flag && args[index+1] == value {
			return
		}
	}
	t.Fatalf("argument pair %q %q absent from %v", flag, value, args)
}

func countArgument(args []string, target string) int {
	count := 0
	for _, argument := range args {
		if argument == target {
			count++
		}
	}
	return count
}
