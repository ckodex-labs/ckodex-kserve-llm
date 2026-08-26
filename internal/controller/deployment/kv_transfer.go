package deployment

import (
	"encoding/json"
	"strconv"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
)

func (b *Builder) applyKVTransfer(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec, role string) {
	if len(podSpec.Containers) == 0 || llmSvc.Spec.KVCache == nil || llmSvc.Spec.KVCache.Transfer == nil {
		return
	}
	t := llmSvc.Spec.KVCache.Transfer
	role = transferRole(t.Role, role)
	connector := map[string]string{"nixl": "NixlConnector", "lmcache": "LMCacheConnectorV1", "mooncake": "MooncakeConnector"}[t.Connector]
	if connector == "" {
		return
	}
	extra := parseKVExtraConfig(t.ExtraConfig)
	addLMCacheExtra(extra, t.LMCache)
	c := &podSpec.Containers[0]
	if t.LMCache != nil && t.LMCache.Mode == servingv1alpha2.LMCacheModeMultiprocess && t.LMCache.EngineRef != nil {
		b.configureMultiprocessCache(c, podSpec, t.LMCache.EngineRef.Name)
		return
	}
	b.configureInlineTransfer(c, t, connector, role, extra)
}

func transferRole(configured, requested string) string {
	if requested != "" {
		return requested
	}
	if configured != "" {
		return configured
	}
	return "kv_both"
}

func addLMCacheExtra(extra map[string]interface{}, cache *servingv1alpha2.LMCacheSpec) {
	if cache == nil || cache.Mode == servingv1alpha2.LMCacheModeMultiprocess {
		return
	}
	if cache.ChunkSize != nil {
		if _, ok := extra["chunk_size"]; !ok {
			extra["chunk_size"] = *cache.ChunkSize
		}
	}
	if cache.LocalCPU != nil {
		if _, ok := extra["local_cpu"]; !ok {
			extra["local_cpu"] = *cache.LocalCPU
		}
	}
	if cache.LocalCPUSizeGiB != nil {
		if _, ok := extra["max_local_cpu_size"]; !ok {
			extra["max_local_cpu_size"] = *cache.LocalCPUSizeGiB
		}
	}
}

func (b *Builder) configureMultiprocessCache(c *corev1.Container, podSpec *corev1.PodSpec, engine string) {
	configEnv := "CKODEX_LMCACHE_KV_TRANSFER_CONFIG"
	if !hasEnv(c.Env, configEnv) {
		c.Env = append(c.Env, corev1.EnvVar{Name: configEnv, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: engine + "-connection"}, Key: "kv-transfer-config"}}})
	}
	if !hasArg(c.Args, "--kv-transfer-config") {
		c.Args = append(c.Args, "--kv-transfer-config", "$("+configEnv+")")
	}
	podSpec.HostIPC = true
	setEnvDefault(c, "PYTHONHASHSEED", "0")
}

func (b *Builder) configureInlineTransfer(c *corev1.Container, transfer *servingv1alpha2.KVTransferSpec, connector, role string, extra map[string]interface{}) {
	cfg, err := json.Marshal(map[string]interface{}{"kv_connector": connector, "kv_role": role, "kv_connector_extra_config": extra})
	if err != nil {
		return
	}
	if !hasArg(c.Args, "--kv-transfer-config") {
		c.Args = append(c.Args, "--kv-transfer-config", string(cfg))
	}
	for _, env := range transfer.Env {
		if !hasEnv(c.Env, env.Name) {
			c.Env = append(c.Env, *env.DeepCopy())
		}
	}
	if strings.EqualFold(transfer.Connector, "lmcache") {
		b.configureLMCacheEnv(c, transfer.LMCache)
	}
}

func (b *Builder) configureLMCacheEnv(c *corev1.Container, cache *servingv1alpha2.LMCacheSpec) {
	setEnvDefault(c, "LMCACHE_USE_EXPERIMENTAL", "True")
	if cache == nil {
		return
	}
	setEnvDefault(c, "PYTHONHASHSEED", "0")
	if cache.ChunkSize != nil {
		setEnvDefault(c, "LMCACHE_CHUNK_SIZE", strconv.FormatInt(int64(*cache.ChunkSize), 10))
	}
	if cache.LocalCPU != nil {
		setEnvDefault(c, "LMCACHE_LOCAL_CPU", strconv.FormatBool(*cache.LocalCPU))
	}
	if cache.LocalCPUSizeGiB != nil {
		setEnvDefault(c, "LMCACHE_MAX_LOCAL_CPU_SIZE", strconv.FormatInt(int64(*cache.LocalCPUSizeGiB), 10))
	}
}
