package deployment

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
)

func TestMultiprocessLMCacheConnectionConfigMapReference(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
				Connector: "lmcache",
				LMCache: &servingv1alpha2.LMCacheSpec{
					Mode:      servingv1alpha2.LMCacheModeMultiprocess,
					EngineRef: &corev1.LocalObjectReference{Name: "shared-kv"},
				},
			}},
		},
	}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm"}}}
	(&Builder{}).applyKVTransfer(service, pod, "")
	if !pod.HostIPC {
		t.Fatal("multiprocess LMCache requires shared host IPC")
	}
	for _, env := range pod.Containers[0].Env {
		if env.Name != "CKODEX_LMCACHE_KV_TRANSFER_CONFIG" {
			continue
		}
		if env.ValueFrom == nil || env.ValueFrom.ConfigMapKeyRef == nil {
			t.Fatal("missing ConfigMap key reference")
		}
		ref := env.ValueFrom.ConfigMapKeyRef
		if ref.Name != "shared-kv-connection" || ref.Key != "kv-transfer-config.json" {
			t.Fatalf("unexpected LMCache ConfigMap reference %q/%q", ref.Name, ref.Key)
		}
		if ref.Optional != nil && *ref.Optional {
			t.Fatal("connection config must not be optional")
		}
		return
	}
	t.Fatal("missing LMCache connection configuration")
}
