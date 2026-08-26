/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package deployment

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func hardeningService() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "inference"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://org/model", Name: "org/model"},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"team": "ai", "app.kubernetes.io/name": "user-value"},
					Annotations: map[string]string{"owner": "platform", "prometheus.io/scrape": "false"},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm", Image: "vllm/vllm-openai:test"}}},
			},
		},
	}
}

func TestBuilderMetadataPrecedence(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build()}
	deployment := builder.Build(context.Background(), hardeningService(), 1, HardwareNVIDIA, nil)
	assert.Equal(t, "ai", deployment.Spec.Template.Labels["team"])
	assert.Equal(t, "llminferenceservice", deployment.Spec.Template.Labels["app.kubernetes.io/name"])
	assert.Equal(t, "platform", deployment.Spec.Template.Annotations["owner"])
	assert.Equal(t, "true", deployment.Spec.Template.Annotations["prometheus.io/scrape"])
	assert.NotContains(t, deployment.Spec.Selector.MatchLabels, "team")
}

func TestBuilderForwardsDeclaredHuggingFaceRevision(t *testing.T) {
	builder := &Builder{Client: fake.NewClientBuilder().Build()}
	svc := hardeningService()
	svc.Spec.Model.Revision = "refs/pr/42"
	initializer := builder.BuildStorageInitializer(context.Background(), svc, HardwareNVIDIA, nil)
	require.NotNil(t, initializer)
	assert.Equal(t, "hf://org/model@refs/pr/42", initializer.Args[0])
}

func TestBuilderTypedLMCacheModes(t *testing.T) {
	t.Run("in process", func(t *testing.T) {
		svc := hardeningService()
		svc.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
			Connector: "lmcache",
			LMCache: &servingv1alpha2.LMCacheSpec{Mode: servingv1alpha2.LMCacheModeInProcess,
				ChunkSize: ptr.To(int32(128)), LocalCPU: ptr.To(true), LocalCPUSizeGiB: ptr.To(int32(8))},
		}}
		deployment := (&Builder{Client: fake.NewClientBuilder().Build()}).Build(context.Background(), svc, 1, HardwareNVIDIA, nil)
		container := deployment.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "128", envValue(container.Env, "LMCACHE_CHUNK_SIZE"))
		assert.Equal(t, "8", envValue(container.Env, "LMCACHE_MAX_LOCAL_CPU_SIZE"))
		assert.Contains(t, strings.Join(container.Args, " "), "LMCacheConnectorV1")
	})

	t.Run("multiprocess", func(t *testing.T) {
		svc := hardeningService()
		svc.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
			Connector: "lmcache", LMCache: &servingv1alpha2.LMCacheSpec{
				Mode:      servingv1alpha2.LMCacheModeMultiprocess,
				EngineRef: &corev1.LocalObjectReference{Name: "shared-kv"},
			},
		}}
		deployment := (&Builder{Client: fake.NewClientBuilder().Build()}).Build(context.Background(), svc, 1, HardwareNVIDIA, nil)
		container := deployment.Spec.Template.Spec.Containers[0]
		assert.True(t, deployment.Spec.Template.Spec.HostIPC)
		assert.Equal(t, "$(CKODEX_LMCACHE_KV_TRANSFER_CONFIG)", argValue(container.Args, "--kv-transfer-config"))
		for _, env := range container.Env {
			if env.Name == "CKODEX_LMCACHE_KV_TRANSFER_CONFIG" {
				require.NotNil(t, env.ValueFrom)
				assert.Equal(t, "shared-kv-connection", env.ValueFrom.ConfigMapKeyRef.Name)
				return
			}
		}
		t.Fatal("multiprocess ConfigMap environment was not injected")
	})
}

func envValue(env []corev1.EnvVar, name string) string {
	for _, item := range env {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}

func argValue(args []string, name string) string {
	for i := range args {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
