/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package conformance_test

import (
	"os"
	"strings"
	"testing"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	runtimeregistry "github.com/ckodex-labs/kserve-llm-operator/internal/runtime/registry"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	kubeyaml "sigs.k8s.io/yaml"
)

type inferenceSample struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Spec       struct {
		Model struct {
			URI string `yaml:"uri"`
		} `yaml:"model"`
	} `yaml:"spec"`
}

type tinyGLMNextSample struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Namespace string            `yaml:"namespace"`
		Labels    map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		Model struct {
			Name     string `yaml:"name"`
			URI      string `yaml:"uri"`
			Revision string `yaml:"revision"`
		} `yaml:"model"`
		Template struct {
			Spec struct {
				Containers []struct {
					Args []string `yaml:"args"`
					Env  []struct {
						Name  string `yaml:"name"`
						Value string `yaml:"value"`
					} `yaml:"env"`
					Resources struct {
						Requests map[string]string `yaml:"requests"`
						Limits   map[string]string `yaml:"limits"`
					} `yaml:"resources"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

type crdContract struct {
	Spec struct {
		Versions []struct {
			Name    string `yaml:"name"`
			Served  bool   `yaml:"served"`
			Storage bool   `yaml:"storage"`
		} `yaml:"versions"`
	} `yaml:"spec"`
}

type conversionPatchContract struct {
	Metadata struct {
		Name        string            `yaml:"name"`
		Annotations map[string]string `yaml:"annotations"`
	} `yaml:"metadata"`
	Spec struct {
		Conversion struct {
			Strategy string `yaml:"strategy"`
			Webhook  struct {
				ConversionReviewVersions []string `yaml:"conversionReviewVersions"`
				ClientConfig             struct {
					Service struct {
						Name      string `yaml:"name"`
						Namespace string `yaml:"namespace"`
						Path      string `yaml:"path"`
					} `yaml:"service"`
				} `yaml:"clientConfig"`
			} `yaml:"webhook"`
		} `yaml:"conversion"`
	} `yaml:"spec"`
}

func TestLocalInferenceSampleUsesStableV1ModelSourceContract(t *testing.T) {
	data, err := os.ReadFile("../../local/04-llm-inference-service.yaml")
	require.NoError(t, err)

	var sample inferenceSample
	require.NoError(t, yaml.Unmarshal(data, &sample))
	require.Equal(t, "serving.ckodex.com/v1", sample.APIVersion)
	require.Equal(t, "LLMInferenceService", sample.Kind)
	require.True(t, strings.HasPrefix(sample.Spec.Model.URI, "hf://"))
}

func TestTinyGLMNextSamplePinsLocalArchitectureFixture(t *testing.T) {
	data, err := os.ReadFile("../../config/samples/llminferenceservice_glm5_next_tiny.yaml")
	require.NoError(t, err)

	var sample tinyGLMNextSample
	require.NoError(t, yaml.Unmarshal(data, &sample))
	require.Equal(t, "serving.ckodex.com/v1", sample.APIVersion)
	require.Equal(t, "LLMInferenceService", sample.Kind)
	require.Equal(t, "default", sample.Metadata.Namespace)
	require.Equal(t, "local-architecture-fixture", sample.Metadata.Labels["serving.ckodex.com/profile"])
	require.Equal(t, "inference-optimization/GLM-5.3-Flash-0.1B-A0.1B", sample.Spec.Model.Name)
	require.Equal(t, "hf://inference-optimization/GLM-5.3-Flash-0.1B-A0.1B", sample.Spec.Model.URI)
	require.Equal(t, "8311399447eba9c9b215e3209ab6f25e59c7d21e", sample.Spec.Model.Revision)
	require.Len(t, sample.Spec.Template.Spec.Containers, 1)
	container := sample.Spec.Template.Spec.Containers[0]
	require.Equal(t, []string{"--max-model-len", "2048", "--max-num-seqs", "1"}, container.Args)
	env := make(map[string]string, len(container.Env))
	for _, variable := range container.Env {
		env[variable.Name] = variable.Value
	}
	require.Equal(t, "1", env["VLLM_CPU_KVCACHE_SPACE"])
	require.Equal(t, "0-2", env["VLLM_CPU_OMP_THREADS_BIND"])
	require.Equal(t, "3", env["OMP_NUM_THREADS"])
	require.Equal(t, map[string]string{"cpu": "2", "memory": "2Gi"}, container.Resources.Requests)
	require.Equal(t, map[string]string{"cpu": "4", "memory": "4Gi"}, container.Resources.Limits)
}

func TestTinyGLMNextSampleConvertsAndRendersThroughDefaultRuntime(t *testing.T) {
	data, err := os.ReadFile("../../config/samples/llminferenceservice_glm5_next_tiny.yaml")
	require.NoError(t, err)

	var stable servingv1.LLMInferenceService
	require.NoError(t, kubeyaml.Unmarshal(data, &stable))
	alpha := &servingv1alpha2.LLMInferenceService{}
	require.NoError(t, alpha.ConvertFrom(&stable))
	require.Equal(t, stable.Spec.Model, servingv1.ModelSpec{
		Name:     "inference-optimization/GLM-5.3-Flash-0.1B-A0.1B",
		URI:      "hf://inference-optimization/GLM-5.3-Flash-0.1B-A0.1B",
		Revision: "8311399447eba9c9b215e3209ab6f25e59c7d21e",
	})

	adapter, ok := runtimeregistry.Resolve(alpha.Spec.Engine)
	require.True(t, ok)
	require.Equal(t, "vllm", adapter.Name())
	rendered := adapter.Render(inferenceruntime.RenderRequest{
		Service:      alpha,
		ModelPath:    "/mnt/models",
		ExistingArgs: stable.Spec.Template.Spec.Containers[0].Args,
	})
	require.Contains(t, rendered.Args, "--model")
	require.Contains(t, rendered.Args, "/mnt/models")
	require.Contains(t, rendered.Args, "--max-model-len")
	require.Contains(t, rendered.Args, "2048")
	require.Contains(t, rendered.Args, "--max-num-seqs")
	require.Contains(t, rendered.Args, "1")
	require.NotContains(t, rendered.Args, "--quantization")
}

func TestConsoleSourceIsPresentInCheckout(t *testing.T) {
	for _, path := range []string{
		"../../console/package.json",
		"../../console/package-lock.json",
		"../../console/Dockerfile",
	} {
		info, err := os.Lstat(path)
		require.NoError(t, err, "console source must be present: %s", path)
		require.False(t, info.Mode()&os.ModeSymlink != 0, "console source must not be a symlink: %s", path)
	}
}

func TestStableCRDDocumentsHfMountModelSources(t *testing.T) {
	data, err := os.ReadFile("../../config/crd/serving.ckodex.com_llminferenceservices.yaml")
	require.NoError(t, err)

	document := string(data)
	require.Contains(t, document, "name: v1")
	require.Contains(t, document, "pattern: ^(hf|hf-mount|hf-mirror|s3|swfs|seaweedfs|gs|pvc|oci|ocis|modelpack|https?)://.*$")
}

func TestBetaCRDProfileBindsStableConversionWebhook(t *testing.T) {
	crdData, err := os.ReadFile("../../config/crd/serving.ckodex.com_llminferenceservices.yaml")
	require.NoError(t, err)
	var crd crdContract
	require.NoError(t, yaml.Unmarshal(crdData, &crd))

	versions := make(map[string]struct{ Served, Storage bool }, len(crd.Spec.Versions))
	for _, version := range crd.Spec.Versions {
		versions[version.Name] = struct{ Served, Storage bool }{Served: version.Served, Storage: version.Storage}
	}
	require.Equal(t, struct{ Served, Storage bool }{Served: true, Storage: true}, versions["v1"])
	require.Equal(t, struct{ Served, Storage bool }{Served: true, Storage: false}, versions["v1alpha2"])

	patchData, err := os.ReadFile("../../config/crd/patches/llminferenceservice-conversion.yaml")
	require.NoError(t, err)
	var patch conversionPatchContract
	require.NoError(t, yaml.Unmarshal(patchData, &patch))
	require.Equal(t, "llminferenceservices.serving.ckodex.com", patch.Metadata.Name)
	require.Equal(t, "Webhook", patch.Spec.Conversion.Strategy)
	require.Equal(t, []string{"v1"}, patch.Spec.Conversion.Webhook.ConversionReviewVersions)
	require.Equal(t, "ckodex-kserve-llm-operator-webhook-service", patch.Spec.Conversion.Webhook.ClientConfig.Service.Name)
	require.Equal(t, "ckodex-system", patch.Spec.Conversion.Webhook.ClientConfig.Service.Namespace)
	require.Equal(t, "/convert", patch.Spec.Conversion.Webhook.ClientConfig.Service.Path)
	require.Equal(t, "ckodex-system/ckodex-kserve-llm-operator-webhook-cert", patch.Metadata.Annotations["cert-manager.io/inject-ca-from"])

	kustomization, err := os.ReadFile("../../config/crd/kustomization.yaml")
	require.NoError(t, err)
	require.Contains(t, string(kustomization), "patches/llminferenceservice-conversion.yaml")
}
