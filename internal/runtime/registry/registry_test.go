package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveUsesDefaultAndRejectsUnregisteredEngines(t *testing.T) {
	defaultAdapter, ok := Resolve("")
	require.True(t, ok)
	require.Equal(t, DefaultEngine, defaultAdapter.Name())

	namedAdapter, ok := Resolve(DefaultEngine)
	require.True(t, ok)
	require.Equal(t, defaultAdapter.Name(), namedAdapter.Name())

	sglangAdapter, ok := Resolve(SGLangEngine)
	require.True(t, ok)
	require.Equal(t, SGLangEngine, sglangAdapter.Name())
	require.True(t, sglangAdapter.Image().Valid())

	_, ok = Resolve("quant-cpp")
	require.False(t, ok)
}

func TestNamesReturnsDefensiveCopy(t *testing.T) {
	names := Names()
	require.Equal(t, []string{SGLangEngine, DefaultEngine}, names)
	names[0] = "mutated"
	require.Equal(t, []string{SGLangEngine, DefaultEngine}, Names())
}
