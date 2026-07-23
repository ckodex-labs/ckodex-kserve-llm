package compatibility_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func TestGeneratedLLMInferenceServiceSchemasAcceptDocumentedModelURIs(t *testing.T) {
	manifest, err := os.ReadFile("../../config/crd/serving.ckodex.com_llminferenceservices.yaml")
	require.NoError(t, err)

	var crd apiextensionsv1.CustomResourceDefinition
	require.NoError(t, yaml.Unmarshal(manifest, &crd))

	for _, version := range crd.Spec.Versions {
		version := version
		t.Run(version.Name, func(t *testing.T) {
			schema := version.Schema.OpenAPIV3Schema
			require.NotNil(t, schema)
			uriSchema := schema.Properties["spec"].Properties["model"].Properties["uri"]
			require.NotEmpty(t, uriSchema.Pattern)

			pattern, err := regexp.Compile(uriSchema.Pattern)
			require.NoError(t, err)
			for _, uri := range []string{
				"hf://org/model",
				"hf-mount://org/model",
				"hf-mirror://org/model",
				"swfs://models/model",
				"seaweedfs://models/model",
				"pvc://model-weights/subpath",
			} {
				require.Truef(t, pattern.MatchString(uri), "%s schema rejects documented URI %q", version.Name, uri)
			}
		})
	}
}
