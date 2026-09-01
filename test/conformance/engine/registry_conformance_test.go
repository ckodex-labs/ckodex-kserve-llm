/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package engine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	runtimeregistry "github.com/ckodex-labs/kserve-llm-operator/internal/runtime/registry"
	"github.com/ckodex-labs/kserve-llm-operator/internal/validation"
	"github.com/stretchr/testify/require"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// TestAdmittedSpecsResolveToRuntime proves the spec-to-runtime direction: each
// admission value resolves to one deterministic adapter implementation.
func TestAdmittedSpecsResolveToRuntime(t *testing.T) {
	for _, engineName := range validation.AdmittedInferenceEngines() {
		t.Run(engineName, func(t *testing.T) {
			require.NoError(t, validation.ValidateInferenceEngine(engineName))
			adapter, ok := runtimeregistry.Resolve(engineName)
			require.True(t, ok)
			require.Equal(t, engineName, adapter.Name())

			service := baselineService(engineName)
			require.Empty(t, adapter.Validate(service))
			request := inferenceruntime.RenderRequest{
				Service: service, ModelPath: "/mnt/models", Host: "127.0.0.1", Port: 9000,
			}
			require.Equal(t, adapter.Render(request), adapter.Render(request))
			require.True(t, adapter.Image().Valid())
			require.NotEmpty(t, adapter.HealthContract().Path)
			require.NotEmpty(t, adapter.MetricsContract().Path)
			for _, enableArg := range adapter.MetricsContract().EnableArgs {
				require.Contains(t, adapter.Render(request).Args, enableArg)
			}
			requireCapabilityTotality(t, adapter.Capabilities())
		})
	}
}

// TestRegisteredRuntimesAreAdmittedBySpec proves the runtime-to-spec direction:
// no adapter can be registered without admission and generated-schema values.
func TestRegisteredRuntimesAreAdmittedBySpec(t *testing.T) {
	registered := runtimeregistry.Names()
	require.Equal(t, registered, validation.AdmittedInferenceEngines())
	require.Equal(t, registered, engineEnum(t, "v1alpha2"))
	require.Equal(t, registered, engineEnum(t, "v1"))
	for _, engineName := range registered {
		require.NoError(t, validation.ValidateInferenceEngine(engineName))
	}
}

func TestUnregisteredRuntimeClaimsAreRejected(t *testing.T) {
	for _, engineName := range []string{"quant-cpp", "llama-cpp", "tensorrt-llm"} {
		t.Run(engineName, func(t *testing.T) {
			require.Error(t, validation.ValidateInferenceEngine(engineName))
			_, ok := runtimeregistry.Resolve(engineName)
			require.False(t, ok)
		})
	}
}

func baselineService(engineName string) *servingv1alpha2.LLMInferenceService {
	service := &servingv1alpha2.LLMInferenceService{}
	service.Spec.Engine = engineName
	service.Spec.Model.Name = "conformance-model"
	return service
}

func requireCapabilityTotality(t *testing.T, matrix inferenceruntime.CapabilityMatrix) {
	t.Helper()
	value := reflect.ValueOf(matrix)
	for index := 0; index < value.NumField(); index++ {
		support := inferenceruntime.CapabilitySupport(value.Field(index).String())
		require.Contains(t, []inferenceruntime.CapabilitySupport{
			inferenceruntime.CapabilitySupported,
			inferenceruntime.CapabilityUnsupported,
			inferenceruntime.CapabilityEmulated,
		}, support, value.Type().Field(index).Name)
	}
}

func engineEnum(t *testing.T, versionName string) []string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "config", "crd", "serving.ckodex.com_llminferenceservices.yaml")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	crd := &extensionsv1.CustomResourceDefinition{}
	require.NoError(t, yaml.Unmarshal(contents, crd))
	for _, version := range crd.Spec.Versions {
		if version.Name != versionName {
			continue
		}
		specSchema := version.Schema.OpenAPIV3Schema.Properties["spec"]
		if versionName == "v1" {
			specSchema = specSchema.Properties["experimental"]
		}
		engineSchema := specSchema.Properties["engine"]
		values := make([]string, 0, len(engineSchema.Enum))
		for _, value := range engineSchema.Enum {
			var engineName string
			require.NoError(t, json.Unmarshal(value.Raw, &engineName))
			values = append(values, engineName)
		}
		return values
	}
	t.Fatal("v1alpha2 schema not found")
	return nil
}
