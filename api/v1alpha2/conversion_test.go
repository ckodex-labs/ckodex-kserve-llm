package v1alpha2

import (
	"testing"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func TestKVTransferConversionPreservesConnectorAndRole(t *testing.T) {
	producer := "kv_producer"
	src := &LLMInferenceService{}
	src.Spec.Model = ModelSpec{URI: "hf://org/model", Name: "model", Revision: "v2"}
	src.Spec.Router.Scheduler = &SchedulerSpec{Replicas: ptr.To(int32(2))}
	src.Spec.KVCache = &KVCacheSpec{Transfer: &KVTransferSpec{
		Connector: "lmcache", Role: producer, ExtraConfig: map[string]string{"chunk_size": "256"},
		Env:     []corev1.EnvVar{{Name: "LMCACHE_CONFIG_FILE", Value: "/etc/lmcache/config.yaml"}},
		LMCache: &LMCacheSpec{Mode: LMCacheModeMultiprocess, EngineRef: &corev1.LocalObjectReference{Name: "shared-kv"}},
	}}

	hub := &servingv1.LLMInferenceService{}
	require.NoError(t, src.ConvertTo(hub))
	require.NotNil(t, hub.Spec.Experimental)
	require.Equal(t, "lmcache", hub.Spec.Experimental.KVCache.Transfer.Connector)
	require.Equal(t, producer, hub.Spec.Experimental.KVCache.Transfer.Role)
	require.Equal(t, "v2", hub.Spec.Model.Revision)
	require.Equal(t, "shared-kv", hub.Spec.Experimental.KVCache.Transfer.LMCache.EngineRef.Name)
	require.Equal(t, int32(2), *hub.Spec.Router.Scheduler.Replicas)

	roundTrip := &LLMInferenceService{}
	require.NoError(t, roundTrip.ConvertFrom(hub))
	require.Equal(t, src.Spec.KVCache.Transfer.ExtraConfig, roundTrip.Spec.KVCache.Transfer.ExtraConfig)
	require.Equal(t, src.Spec.KVCache.Transfer.Env, roundTrip.Spec.KVCache.Transfer.Env)
	require.Equal(t, src.Spec.Model.Revision, roundTrip.Spec.Model.Revision)
	require.Equal(t, "shared-kv", roundTrip.Spec.KVCache.Transfer.LMCache.EngineRef.Name)
}
