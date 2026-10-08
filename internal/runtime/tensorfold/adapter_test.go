package tensorfold

import (
	"reflect"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestAdapterDeclaresImageEndpointsAndCapabilities(t *testing.T) {
	adapter := Adapter{}
	require.Equal(t, engineName, adapter.Name())
	require.Equal(t, inferenceruntime.ConformanceTierServed, adapter.Tier())
	require.True(t, adapter.Image().Valid())
	require.Equal(t, "tensorfold/tensorfold:v1.0.0@sha256:d8e5f2a1b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0", adapter.Image().Reference())
	require.Equal(t, "/health", adapter.HealthContract().Path)
	require.Equal(t, "/metrics", adapter.MetricsContract().Path)
	require.Equal(t, []string{"--metrics-open"}, adapter.MetricsContract().EnableArgs)

	capabilities := adapter.Capabilities()
	require.Equal(t, inferenceruntime.CapabilitySupported, capabilities.SpeculativeDecoding)
	require.Equal(t, inferenceruntime.CapabilitySupported, capabilities.DataParallel)
	require.Equal(t, inferenceruntime.CapabilityUnsupported, capabilities.TensorParallel)

	value := reflect.ValueOf(capabilities)
	for index := 0; index < value.NumField(); index++ {
		require.NotEmpty(t, value.Field(index).String(), value.Type().Field(index).Name)
	}
}

func TestAdapterRendersGovernedArguments(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	service.Spec.Model.Name = "served-model"
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Data: ptr.To(int32(4)),
	}
	service.Spec.SpeculativeDecoding = &servingv1alpha2.SpeculativeDecodingSpec{
		Method:     "mtp",
		NumTokens:  ptr.To(int32(3)),
		DraftModel: "draft/model",
	}

	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service:      service,
		ModelPath:    "/models/model",
		Host:         "127.0.0.1",
		Port:         9000,
		ExistingArgs: []string{"--device", "mps", "--max-model-len=4096", "--device=cuda", "--max-model-len", "2048"},
	})
	require.Equal(t, []string{"tensorfold", "serve", "/models/model"}, rendered.Args[:3])
	assertArgumentPair(t, rendered.Args, "--name", "served-model")
	assertArgumentPair(t, rendered.Args, "--host", "127.0.0.1")
	assertArgumentPair(t, rendered.Args, "--port", "9000")
	assertArgumentPair(t, rendered.Args, "--parallel", "4")
	assertArgumentPair(t, rendered.Args, "--spec-method", "mtp")
	assertArgumentPair(t, rendered.Args, "--spec-tokens", "3")
	assertArgumentPair(t, rendered.Args, "--draft-model", "draft/model")
	require.Contains(t, rendered.Args, "--metrics-open")
	require.NotContains(t, rendered.Args, "--device")
	require.NotContains(t, rendered.Args, "mps")
	require.NotContains(t, rendered.Args, "--max-model-len=4096")
	require.NotContains(t, rendered.Args, "--device=cuda")
	require.NotContains(t, rendered.Args, "--max-model-len")
}

func TestAdapterRenderWithDefaultsAndNilService(t *testing.T) {
	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		ModelPath: "/models/default",
	})
	require.Equal(t, []string{"tensorfold", "serve", "/models/default"}, rendered.Args[:3])
	assertArgumentPair(t, rendered.Args, "--host", "0.0.0.0")
	assertArgumentPair(t, rendered.Args, "--port", "8000")
	require.Contains(t, rendered.Args, "--metrics-open")

	// Negative port fallback
	renderedNegPort := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Port: -1,
	})
	assertArgumentPair(t, renderedNegPort.Args, "--port", "8000")
}

func TestAdapterRenderWithEmptyParallelismAndSpeculative(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	// Spec with non-positive values to exercise appendPositiveSize guard branches
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Data: ptr.To(int32(0)),
	}
	service.Spec.SpeculativeDecoding = &servingv1alpha2.SpeculativeDecodingSpec{
		NumTokens: ptr.To(int32(-1)),
	}

	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service:   service,
		ModelPath: "/models/test",
	})
	require.NotContains(t, rendered.Args, "--parallel")
	require.NotContains(t, rendered.Args, "--spec-tokens")
	require.NotContains(t, rendered.Args, "--spec-method")
	require.NotContains(t, rendered.Args, "--draft-model")

	// Also test with nil Parallelism and nil SpeculativeDecoding
	serviceNil := &servingv1alpha2.LLMInferenceService{}
	renderedNil := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service:   serviceNil,
		ModelPath: "/models/test",
	})
	require.NotContains(t, renderedNil.Args, "--parallel")
	require.NotContains(t, renderedNil.Args, "--spec-tokens")
}

func TestAdapterPreservesExplicitArguments(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	service.Spec.Model.Name = "service-model"
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Data: ptr.To(int32(2)),
	}
	service.Spec.SpeculativeDecoding = &servingv1alpha2.SpeculativeDecodingSpec{
		Method:     "draft",
		NumTokens:  ptr.To(int32(5)),
		DraftModel: "custom-draft",
	}

	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		Service:   service,
		ModelPath: "/models/model",
		ExistingArgs: []string{
			"--name", "explicit-model",
			"--host", "custom-host",
			"--port=31000",
			"--parallel", "8",
			"--spec-method", "draft-v2",
			"--spec-tokens", "10",
			"--draft-model", "explicit-draft",
			"--metrics-open",
		},
	})

	assertArgumentPair(t, rendered.Args, "--name", "explicit-model")
	assertArgumentPair(t, rendered.Args, "--host", "custom-host")
	require.Contains(t, rendered.Args, "--port=31000")
	assertArgumentPair(t, rendered.Args, "--parallel", "8")
	assertArgumentPair(t, rendered.Args, "--spec-method", "draft-v2")
	assertArgumentPair(t, rendered.Args, "--spec-tokens", "10")
	assertArgumentPair(t, rendered.Args, "--draft-model", "explicit-draft")
	require.Equal(t, 1, countArgument(rendered.Args, "--metrics-open"))
}

func TestAdapterLaunchPrefixVariations(t *testing.T) {
	// Already has tensorfold serve and model
	r1 := (Adapter{}).Render(inferenceruntime.RenderRequest{
		ExistingArgs: []string{"tensorfold", "serve", "/explicit/model", "--context", "8192"},
		ModelPath:    "/models/ignored",
	})
	require.Equal(t, []string{"tensorfold", "serve", "/explicit/model"}, r1.Args[:3])
	require.Equal(t, 1, countArgument(r1.Args, "serve"))

	// Has tensorfold serve without model
	r2 := (Adapter{}).Render(inferenceruntime.RenderRequest{
		ExistingArgs: []string{"tensorfold", "serve", "--context", "8192"},
		ModelPath:    "/models/inserted",
	})
	require.Equal(t, []string{"tensorfold", "serve", "/models/inserted", "--context", "8192"}, r2.Args[:5])

	// Has tensorfold-native serve with model
	r3 := (Adapter{}).Render(inferenceruntime.RenderRequest{
		ExistingArgs: []string{"tensorfold-native", "serve", "/explicit/model"},
		ModelPath:    "/models/ignored",
	})
	require.Equal(t, []string{"tensorfold-native", "serve", "/explicit/model"}, r3.Args[:3])

	// Empty modelPath
	r4 := (Adapter{}).Render(inferenceruntime.RenderRequest{
		ExistingArgs: []string{"--context", "8192"},
		ModelPath:    "",
	})
	require.Equal(t, []string{"tensorfold", "serve", "--context", "8192"}, r4.Args[:4])

	// Has tensorfold serve with empty modelPath
	r5 := (Adapter{}).Render(inferenceruntime.RenderRequest{
		ExistingArgs: []string{"tensorfold", "serve", "--context", "8192"},
		ModelPath:    "",
	})
	require.Equal(t, []string{"tensorfold", "serve", "--context", "8192"}, r5.Args[:4])
}

func TestAdapterValidatesSuccessfully(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Data: ptr.To(int32(2)),
	}
	service.Spec.SpeculativeDecoding = &servingv1alpha2.SpeculativeDecodingSpec{
		Method: "mtp",
	}
	require.Empty(t, (Adapter{}).Validate(service))

	service.Spec.SpeculativeDecoding.Method = "draft"
	require.Empty(t, (Adapter{}).Validate(service))

	service.Spec.SpeculativeDecoding.Method = ""
	require.Empty(t, (Adapter{}).Validate(service))
}

func TestAdapterRejectsNilServiceAndWrongEngine(t *testing.T) {
	require.NotEmpty(t, (Adapter{}).Validate(nil))

	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = "other"
	errs := (Adapter{}).Validate(service)
	require.Len(t, errs, 1)
	require.Equal(t, "spec.engine", errs[0].Field)
}

func TestAdapterRejectsUnmappedFields(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Tensor:      ptr.To(int32(2)),
		DataLocal:   ptr.To(int32(1)),
		Pipeline:    ptr.To(int32(2)),
		Expert:      true,
		EPLBEnabled: true,
	}
	service.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Dtype: "fp8"}
	service.Spec.Quantization = &servingv1alpha2.QuantizationSpec{Method: "awq"}
	service.Spec.Prefill = &servingv1alpha2.PrefillSpec{}
	service.Spec.Worker = &servingv1alpha2.WorkerSpec{}
	service.Spec.SpeculativeDecoding = &servingv1alpha2.SpeculativeDecodingSpec{
		Method: "ngram",
	}

	errs := (Adapter{}).Validate(service)
	require.Len(t, errs, 10)
	require.Equal(t, "spec.parallelism.tensor", errs[0].Field)
	require.Equal(t, "spec.parallelism.dataLocal", errs[1].Field)
	require.Equal(t, "spec.parallelism.pipeline", errs[2].Field)
	require.Equal(t, "spec.parallelism.expert", errs[3].Field)
	require.Equal(t, "spec.parallelism.eplbEnabled", errs[4].Field)
	require.Equal(t, "spec.kvCache", errs[5].Field)
	require.Equal(t, "spec.quantization", errs[6].Field)
	require.Equal(t, "spec.prefill", errs[7].Field)
	require.Equal(t, "spec.worker", errs[8].Field)
	require.Equal(t, "spec.speculativeDecoding.method", errs[9].Field)
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
