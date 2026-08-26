package v1alpha2

import (
	"encoding/json"
	"testing"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func conversionFixture() *LLMInferenceService {
	producer := "kv_producer"
	src := &LLMInferenceService{}
	src.Spec.Model = ModelSpec{URI: "hf://org/model", Name: "model", Revision: "v2"}
	src.Spec.Model.HardwareAware = true
	src.Spec.Model.Storage = &StorageSpec{
		ExternalSecret: &ExternalSecretSpec{
			SecretStoreRef:  SecretStoreRef{Name: "cluster-store", Kind: "ClusterSecretStore"},
			RefreshInterval: "15m",
			Data: []ExternalSecretData{{
				SecretKey: "token",
				RemoteRef: ExternalSecretRemoteRef{Key: "models/llama", Property: "token"},
			}},
		},
	}
	src.Spec.Router.Scheduler = &SchedulerSpec{Replicas: ptr.To(int32(2))}
	src.Spec.Router.Route.HTTPRoute = &HTTPRouteSpec{
		Hostnames:  []string{"model.example.test"},
		Resilience: &ResilienceSpec{Timeout: "45s", MaxRetries: 2, RetryOn: "5xx"},
	}
	src.Spec.Parallelism = &ParallelismSpec{Pipeline: ptr.To(int32(2)), EPLBEnabled: true}
	src.Spec.SpeculativeDecoding = &SpeculativeDecodingSpec{Method: "mtp", NumTokens: ptr.To(int32(5))}
	src.Spec.Quantization = &QuantizationSpec{Method: "fp8", CheckpointPath: "/models/checkpoint"}
	src.Spec.Engine = "vllm"
	src.Spec.ToolSurface = &ToolSurface{AllowedAPIs: []string{"api.example.test"}, AllowedCIDRs: []string{"10.0.0.0/8"}}
	src.Spec.Observability = &ObservabilitySpec{Sink: &TelemetrySink{Type: "otlp", Endpoint: "http://otel:4317"}}
	src.Spec.KVCache = &KVCacheSpec{Transfer: &KVTransferSpec{
		Connector: "lmcache", Role: producer, ExtraConfig: map[string]string{"chunk_size": "256"},
		Env:     []corev1.EnvVar{{Name: "LMCACHE_CONFIG_FILE", Value: "/etc/lmcache/config.yaml"}},
		LMCache: &LMCacheSpec{Mode: LMCacheModeMultiprocess, EngineRef: &corev1.LocalObjectReference{Name: "shared-kv"}},
	}}
	src.Status = LLMInferenceServiceStatus{
		Conditions:       []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
		StatePlanes:      StatePlanes{Lifecycle: "active", Trust: "verified", Risk: "normal"},
		Optimized:        true,
		DetectedHardware: "nvidia-h100",
		AdaptiveMetrics:  &AdaptiveMetrics{P99Latency: "120ms", QueueDepth: 3, LoadLevel: "Light"},
	}
	return src
}

func TestKVTransferConversionPreservesConnectorAndRole(t *testing.T) {
	src := conversionFixture()

	hub := &servingv1.LLMInferenceService{}
	require.NoError(t, src.ConvertTo(hub))
	require.NotNil(t, hub.Spec.Experimental)
	require.Equal(t, "lmcache", hub.Spec.Experimental.KVCache.Transfer.Connector)
	require.Equal(t, "kv_producer", hub.Spec.Experimental.KVCache.Transfer.Role)
	require.Equal(t, "v2", hub.Spec.Model.Revision)
	require.Equal(t, "shared-kv", hub.Spec.Experimental.KVCache.Transfer.LMCache.EngineRef.Name)
	require.Equal(t, int32(2), *hub.Spec.Router.Scheduler.Replicas)

	roundTrip := &LLMInferenceService{}
	require.NoError(t, roundTrip.ConvertFrom(hub))
	require.Equal(t, src.Spec.KVCache.Transfer.ExtraConfig, roundTrip.Spec.KVCache.Transfer.ExtraConfig)
	require.Equal(t, src.Spec.KVCache.Transfer.Env, roundTrip.Spec.KVCache.Transfer.Env)
	require.Equal(t, src.Spec.Model.Revision, roundTrip.Spec.Model.Revision)
	require.Equal(t, "shared-kv", roundTrip.Spec.KVCache.Transfer.LMCache.EngineRef.Name)
	require.True(t, roundTrip.Spec.Model.HardwareAware)
	require.Equal(t, src.Spec.Model.Storage.ExternalSecret, roundTrip.Spec.Model.Storage.ExternalSecret)
	require.Equal(t, src.Spec.Parallelism, roundTrip.Spec.Parallelism)
	require.Equal(t, src.Spec.Router.Route.HTTPRoute, roundTrip.Spec.Router.Route.HTTPRoute)
	require.Equal(t, src.Spec.SpeculativeDecoding, roundTrip.Spec.SpeculativeDecoding)
	require.Equal(t, src.Spec.Quantization, roundTrip.Spec.Quantization)
	require.Equal(t, src.Spec.Engine, roundTrip.Spec.Engine)
	require.Equal(t, src.Spec.ToolSurface, roundTrip.Spec.ToolSurface)
	require.Equal(t, src.Spec.Observability, roundTrip.Spec.Observability)
	require.Equal(t, src.Status.Conditions, roundTrip.Status.Conditions)
	require.Equal(t, src.Status.StatePlanes, roundTrip.Status.StatePlanes)
	require.Equal(t, src.Status.Optimized, roundTrip.Status.Optimized)
	require.Equal(t, src.Status.DetectedHardware, roundTrip.Status.DetectedHardware)
	require.Equal(t, src.Status.AdaptiveMetrics, roundTrip.Status.AdaptiveMetrics)
}

func FuzzLLMInferenceServiceConversionRoundTrip(f *testing.F) {
	f.Add([]byte(`{"metadata":{"name":"model"},"spec":{"model":{"uri":"hf://org/model","name":"model"},"engine":"vllm"}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var src LLMInferenceService
		if err := json.Unmarshal(data, &src); err != nil {
			t.Skip()
		}

		hub := &servingv1.LLMInferenceService{}
		require.NoError(t, src.ConvertTo(hub))

		roundTrip := &LLMInferenceService{}
		require.NoError(t, roundTrip.ConvertFrom(hub))
		require.Equal(t, src.ObjectMeta, roundTrip.ObjectMeta)
		require.Equal(t, src.Spec, roundTrip.Spec)
		require.Equal(t, src.Status, roundTrip.Status)
	})
}
