/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package conformance_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
