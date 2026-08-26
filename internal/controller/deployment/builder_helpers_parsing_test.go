package deployment

import (
	"reflect"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestParseKVValueSupportsScalarStructuredAndTextValues(t *testing.T) {
	assert.Equal(t, true, parseKVValue("true"))
	assert.Equal(t, int64(42), parseKVValue("42"))
	assert.Equal(t, 3.5, parseKVValue("3.5"))
	assert.Equal(t, map[string]interface{}{"key": "value"}, parseKVValue(`{"key":"value"}`))
	assert.Equal(t, []interface{}{"a", float64(2)}, parseKVValue(`["a",2]`))
	assert.Equal(t, "not-json", parseKVValue("not-json"))
	assert.Equal(t, map[string]interface{}{}, parseKVValue("{}"))
}

func TestCopyStringMapAndDeploymentHelpersPreserveBoundaries(t *testing.T) {
	source := map[string]string{"a": "b"}
	copy := copyStringMap(source)
	copy["a"] = "changed"
	assert.Equal(t, "b", source["a"])
	assert.Equal(t, map[string]interface{}{"enabled": true, "count": int64(2)}, parseKVExtraConfig(map[string]string{"enabled": "true", "count": "2"}))
	assert.True(t, hasArg([]string{"--foo=bar"}, "--foo"))
	assert.False(t, hasArg([]string{"--foobar"}, "--foo"))
	assert.True(t, isNilSPIREInjector((*typedNilSPIREInjector)(nil)))
	assert.False(t, isNilSPIREInjector(structuredSPIREInjector{}))
	assert.True(t, reflect.DeepEqual(deploymentStrategyForReplicas(1), deploymentStrategyForReplicas(0)))
}

type structuredSPIREInjector struct{}

func (structuredSPIREInjector) InjectSidecar(_ *corev1.PodSpec, _ *servingv1alpha2.LLMInferenceService) {
}
