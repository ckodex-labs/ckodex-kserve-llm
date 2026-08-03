package compatibility_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func TestGeneratedSchemasExposeIssueHardeningFields(t *testing.T) {
	manifest, err := os.ReadFile("../../config/crd/serving.ckodex.com_llminferenceservices.yaml")
	require.NoError(t, err)
	var crd apiextensionsv1.CustomResourceDefinition
	require.NoError(t, yaml.Unmarshal(manifest, &crd))
	for _, version := range crd.Spec.Versions {
		spec := version.Schema.OpenAPIV3Schema.Properties["spec"]
		model := spec.Properties["model"]
		require.Contains(t, model.Properties, "revision", version.Name)
		metadata := spec.Properties["template"].Properties["metadata"]
		require.Contains(t, metadata.Properties, "labels", version.Name)
		require.Contains(t, metadata.Properties, "annotations", version.Name)
		scheduler := spec.Properties["router"].Properties["scheduler"]
		require.NotEmpty(t, scheduler.Properties, version.Name)
	}
}

func TestLMCacheSetupDryRunAndPinnedChecksum(t *testing.T) {
	data, err := os.ReadFile("../../run/setup-lmcache.sh")
	require.NoError(t, err)
	require.Contains(t, string(data), "8e5eab17ccead2915fc54465e485a9f2c6c947b9fb1301c2149ba1a94dc7e609")
	cmd := exec.Command("bash", "../../run/setup-lmcache.sh", "--mode", "multiprocess", "--namespace", "test-ns")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	require.True(t, strings.Contains(string(out), "Dry run"))
	require.Contains(t, string(out), "engineRef")
}
