package vllm

import (
	"encoding/json"
	"reflect"
	"strings"
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

func TestAdapterImageContractV0330(t *testing.T) {
	adapter := Adapter{}
	image := adapter.Image()
	require.Equal(t, "vllm/vllm-openai", image.Repository)
	require.Equal(t, "v0.33.0", image.Tag)
	require.Equal(t, "sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e", image.Digest)
	require.True(t, image.Valid())
	require.Equal(t, "vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e", image.Reference())
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

func TestAdapterAppliesSecureFrontendDefaults(t *testing.T) {
	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{ModelPath: "/models/model"})
	assertArgumentPair(t, rendered.Args, "--allowed-origins", "[]")
	require.Equal(t, []string{"GET", "POST"}, jsonListArgumentForTest(t, rendered.Args, "--allowed-methods"))
	require.Equal(t, []string{"Authorization", "Content-Type", "OpenAI-Organization", "OpenAI-Project", "X-CKodex-Adapter", "X-Request-ID"}, jsonListArgumentForTest(t, rendered.Args, "--allowed-headers"))
	require.Contains(t, rendered.Args, "--disable-fastapi-docs")
	require.Contains(t, rendered.Args, "--no-allow-credentials")
	require.Contains(t, rendered.Args, "--no-trust-remote-code")
}

func TestAdapterPreservesExplicitCORSOrigin(t *testing.T) {
	rendered := (Adapter{}).Render(inferenceruntime.RenderRequest{
		ModelPath:    "/models/model",
		ExistingArgs: []string{"--allowed-origins", `["https://console.example"]`, "--no-disable-fastapi-docs"},
	})
	require.Equal(t, []string{"https://console.example"}, jsonListArgumentForTest(t, rendered.Args, "--allowed-origins"))
	require.NotContains(t, rendered.Args, "--disable-fastapi-docs")
	require.Contains(t, rendered.Args, "--no-disable-fastapi-docs")
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

func TestAdapterRejectsRemovedMedusaAndMLPSpeculators(t *testing.T) {
	for _, method := range []string{"medusa", "mlp", "MEDUSA", "MLP"} {
		service := &servingv1alpha2.LLMInferenceService{
			Spec: servingv1alpha2.LLMInferenceServiceSpec{
				SpeculativeDecoding: &servingv1alpha2.SpeculativeDecodingSpec{
					Method: method,
				},
			},
		}
		errs := (Adapter{}).Validate(service)
		require.Len(t, errs, 1)
		require.Equal(t, "spec.speculativeDecoding.method", errs[0].Field)
	}
}

func TestAdapterRejectsDeprecatedMambaCacheMode(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Template.Spec.Containers = []corev1.Container{
		{Args: []string{"--mamba-cache-mode", "cache"}},
	}
	errs := (Adapter{}).Validate(service)
	require.Len(t, errs, 1)
	require.Equal(t, "spec.template.spec.containers[0].args[0]", errs[0].Field)
}

func TestAdapterRejectsWildcardAndMalformedAllowedOrigins(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "wildcard", args: []string{"--allowed-origins", `["*"]`}},
		{name: "wildcard origin host", args: []string{"--allowed-origins", `["https://*.example"]`}},
		{name: "wildcard origin underscore alias", args: []string{"--allowed_origins", `["*"]`}},
		{name: "wildcard origin abbreviation", args: []string{"--allowed-or", `["*"]`}},
		{name: "valid origin abbreviation", args: []string{"--allowed-o", `["https://console.example"]`}},
		{name: "missing value", args: []string{"--allowed-origins"}},
		{name: "malformed JSON", args: []string{"--allowed-origins", "https://console.example"}},
		{name: "wildcard method", args: []string{"--allowed-methods", `["*"]`}},
		{name: "invalid origin path", args: []string{"--allowed-origins", `["https://console.example/app"]`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &servingv1alpha2.LLMInferenceService{}
			service.Spec.Template.Spec.Containers = []corev1.Container{{Args: test.args}}
			require.NotEmpty(t, (Adapter{}).Validate(service))
		})
	}
}

func TestAdapterAcceptsJSONCORSLists(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Template.Spec.Containers = []corev1.Container{{Args: []string{
		"--allowed-origins", `[]`,
		"--allowed-methods", `["GET","POST"]`,
		"--allowed-headers", `["Authorization","Content-Type"]`,
	}}}
	require.Empty(t, (Adapter{}).Validate(service))
}

func jsonListArgumentForTest(t *testing.T, args []string, flag string) []string {
	t.Helper()
	for index, argument := range args {
		if argument == flag || strings.HasPrefix(argument, flag+"=") {
			value, ok := argumentValue(args, index, flag)
			require.True(t, ok)
			var values []string
			require.NoError(t, json.Unmarshal([]byte(value), &values))
			return values
		}
	}
	t.Fatalf("argument %s not found in %v", flag, args)
	return nil
}

func TestAdapterRejectsServerDevelopmentMode(t *testing.T) {
	for _, value := range []string{"1", "2", "-1"} {
		service := &servingv1alpha2.LLMInferenceService{}
		service.Spec.Template.Spec.Containers = []corev1.Container{{Env: []corev1.EnvVar{{Name: "VLLM_SERVER_DEV_MODE", Value: value}}}}
		require.NotEmpty(t, (Adapter{}).Validate(service), "value %q must be rejected", value)
	}

	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Template.Spec.Containers = []corev1.Container{{Env: []corev1.EnvVar{{
		Name:      "VLLM_SERVER_DEV_MODE",
		ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "dev-mode"}, Key: "enabled"}},
	}}}}
	require.NotEmpty(t, (Adapter{}).Validate(service), "the validator cannot establish that a Secret does not enable dev mode")

	service.Spec.Template.Spec.Containers[0].Env[0] = corev1.EnvVar{Name: "VLLM_SERVER_DEV_MODE", Value: "0"}
	require.Empty(t, (Adapter{}).Validate(service), "explicitly disabled dev mode is allowed")
}

func TestAdapterBoundsConfiguredSequenceFanOut(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "minimum", value: "1", valid: true},
		{name: "default maximum", value: "128", valid: true},
		{name: "zero", value: "0"},
		{name: "above limit", value: "129"},
		{name: "invalid", value: "unbounded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &servingv1alpha2.LLMInferenceService{}
			service.Spec.Template.Spec.Containers = []corev1.Container{{Env: []corev1.EnvVar{{Name: "VLLM_MAX_N_SEQUENCES", Value: test.value}}}}
			errs := (Adapter{}).Validate(service)
			if test.valid {
				require.Empty(t, errs)
			} else {
				require.NotEmpty(t, errs)
			}
		})
	}

	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Template.Spec.Containers = []corev1.Container{{Env: []corev1.EnvVar{{
		Name: "VLLM_MAX_N_SEQUENCES",
		ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "sequence-limit"}, Key: "value",
		}},
	}}}}
	require.NotEmpty(t, (Adapter{}).Validate(service), "secret-backed limits cannot be validated before execution")
}

func TestAdapterRejectsInsecureRuntimeOverrides(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		env  []corev1.EnvVar
		cmd  []string
	}{
		{name: "command override", cmd: []string{"/bin/sh", "-c", "vllm serve --trust-remote-code"}},
		{name: "enable docs", args: []string{"--no-disable-fastapi-docs"}},
		{name: "enable docs alternate spelling", args: []string{"--enable-fastapi-docs"}},
		{name: "local media path", args: []string{"--allowed-local-media-path", "/"}},
		{name: "local media path underscore alias", args: []string{"--allowed_local_media_path", "/"}},
		{name: "local media path abbreviation", args: []string{"--allowed-l", "/"}},
		{name: "media redirects enabled", env: []corev1.EnvVar{{Name: "VLLM_MEDIA_URL_ALLOW_REDIRECTS", Value: "1"}}},
		{name: "media redirects from secret", env: []corev1.EnvVar{{
			Name: "VLLM_MEDIA_URL_ALLOW_REDIRECTS",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "redirects"}, Key: "enabled",
			}},
		}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &servingv1alpha2.LLMInferenceService{}
			service.Spec.Template.Spec.Containers = []corev1.Container{{Args: test.args, Env: test.env, Command: test.cmd}}
			require.NotEmpty(t, (Adapter{}).Validate(service))
		})
	}
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
