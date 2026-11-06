package validation

import (
	"strings"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
)

func TestMultiprocessLMCacheContract(t *testing.T) {
	newService := func() *servingv1alpha2.LLMInferenceService {
		return &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
			KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
				Connector: "lmcache", LMCache: &servingv1alpha2.LMCacheSpec{Mode: servingv1alpha2.LMCacheModeMultiprocess, EngineRef: &corev1.LocalObjectReference{Name: "shared-kv"}},
			}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "model"}, {Name: "sidecar"}}}},
		}}
	}
	tests := []struct {
		name   string
		change func(*servingv1alpha2.LLMInferenceService)
		want   string
	}{
		{"default role", func(*servingv1alpha2.LLMInferenceService) {}, ""},
		{"both role", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.KVCache.Transfer.Role = "kv_both" }, ""},
		{"wrong connector", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.KVCache.Transfer.Connector = "nixl" }, "spec.kvCache.transfer.connector"},
		{"missing engine", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.KVCache.Transfer.LMCache.EngineRef = nil }, "spec.kvCache.transfer.lmcache.engineRef.name"},
		{"empty engine", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.KVCache.Transfer.LMCache.EngineRef.Name = "" }, "spec.kvCache.transfer.lmcache.engineRef.name"},
		{"discarded extra config", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.KVCache.Transfer.ExtraConfig = map[string]string{"x": "y"}
		}, "spec.kvCache.transfer.extraConfig"},
		{"producer", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.KVCache.Transfer.Role = "kv_producer" }, "spec.kvCache.transfer.role"},
		{"consumer", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.KVCache.Transfer.Role = "kv_consumer" }, "spec.kvCache.transfer.role"},
		{"prefill", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.Prefill = &servingv1alpha2.PrefillSpec{} }, "spec.prefill"},
		{"worker", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.Worker = &servingv1alpha2.WorkerSpec{} }, "spec.worker"},
		{"pipeline multiple", func(s *servingv1alpha2.LLMInferenceService) {
			n := int32(2)
			s.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{Pipeline: &n}
		}, "spec.parallelism.pipeline"},
		{"pipeline one", func(s *servingv1alpha2.LLMInferenceService) {
			n := int32(1)
			s.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{Pipeline: &n}
		}, ""},
		{"tensor multiple", func(s *servingv1alpha2.LLMInferenceService) {
			n := int32(2)
			s.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{Tensor: &n}
		}, ""},
		{"transfer env", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.KVCache.Transfer.Env = []corev1.EnvVar{{Name: "CKODEX_LMCACHE_KV_TRANSFER_CONFIG"}}
		}, "spec.kvCache.transfer.env[0].name"},
		{"template env", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "CKODEX_LMCACHE_KV_TRANSFER_CONFIG"}}
		}, "spec.template.spec.containers[0].env[0].name"},
		{"args separate", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.Template.Spec.Containers[0].Args = []string{"--kv-transfer-config", "{}"}
		}, "spec.template.spec.containers[0].args[0]"},
		{"args equals", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.Template.Spec.Containers[0].Args = []string{"--kv-transfer-config={}"}
		}, "spec.template.spec.containers[0].args[0]"},
		{"command argument", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.Template.Spec.Containers[0].Command = []string{"vllm", "--kv-transfer-config={}"}
		}, "spec.template.spec.containers[0].command[1]"},
		{"unrelated argument", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.Template.Spec.Containers[0].Args = []string{"--kv-transfer-config-extra=1"}
		}, ""},
		{"sidecar argument", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.Template.Spec.Containers[1].Args = []string{"--kv-transfer-config={}"}
		}, ""},
		{"in-process unaffected", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.KVCache.Transfer.LMCache.Mode = servingv1alpha2.LMCacheModeInProcess
			s.Spec.KVCache.Transfer.Role = "kv_producer"
		}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newService()
			tc.change(s)
			errs := ValidateLLMInferenceServiceSurface(s)
			if tc.want == "" && len(errs) != 0 {
				t.Fatalf("unexpected rejection: %v", errs)
			}
			if tc.want != "" && (len(errs) == 0 || !strings.Contains(errs.ToAggregate().Error(), tc.want)) {
				t.Fatalf("expected %s rejection, got %v", tc.want, errs)
			}
		})
	}
}
