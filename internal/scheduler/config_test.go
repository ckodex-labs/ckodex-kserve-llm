/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"

	corev1 "k8s.io/api/core/v1"
)

func schedulerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

func minimalLLMSvc(name, ns string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: name,
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
		},
	}
}

// ---- buildDefaultConfig ----------------------------------------------------

func TestBuildDefaultConfig_ContainsServiceName(t *testing.T) {
	r := &ConfigReconciler{}
	svc := minimalLLMSvc("my-model", "default")
	data := r.buildDefaultConfig(svc)
	yaml, ok := data["scheduler.yaml"]
	require.True(t, ok, "scheduler.yaml key must be present")
	assert.Contains(t, yaml, "my-model")
}

func TestBuildDefaultConfig_ContainsExpectedPlugins(t *testing.T) {
	r := &ConfigReconciler{}
	svc := minimalLLMSvc("phi3", "default")
	data := r.buildDefaultConfig(svc)
	yaml := data["scheduler.yaml"]

	assert.Contains(t, yaml, "prefix-cache-scorer")
	assert.Contains(t, yaml, "queue-scorer")
	assert.Contains(t, yaml, "kv-cache-utilization-scorer")
	assert.Contains(t, yaml, "metrics-data-source")
	assert.Contains(t, yaml, "core-metrics-extractor")
}

func TestBuildDefaultConfig_PluginWeights(t *testing.T) {
	r := &ConfigReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	data := r.buildDefaultConfig(svc)
	yaml := data["scheduler.yaml"]

	assert.Equal(t, 2, strings.Count(yaml, `weight: 2`))
	assert.Equal(t, 1, strings.Count(yaml, `weight: 3`))
}

func TestBuildDefaultConfig_ValidYAMLProfile(t *testing.T) {
	r := &ConfigReconciler{}
	svc := minimalLLMSvc("gemma", "staging")
	data := r.buildDefaultConfig(svc)
	yaml := data["scheduler.yaml"]

	assert.Contains(t, yaml, "schedulingProfiles:")
	assert.Contains(t, yaml, "apiVersion: inference.networking.x-k8s.io/v1alpha1")
	assert.Contains(t, yaml, "- name: default")
	assert.Contains(t, yaml, "plugins:")
}

// ---- Reconcile — ConfigMap create/update -----------------------------------

func TestConfigReconciler_CreatesConfigMap(t *testing.T) {
	scheme := schedulerScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	r := &ConfigReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.Reconcile(context.Background(), svc))

	var cm corev1.ConfigMap
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "llama3-scheduler-config", Namespace: "default"}, &cm))

	assert.Contains(t, cm.Data["scheduler.yaml"], "llama3")
	assert.Equal(t, "ckodex-kserve-llm-operator", cm.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "scheduler-config", cm.Labels["serving.ckodex.com/role"])
}

func TestConfigReconciler_UpdatesExistingConfigMap(t *testing.T) {
	scheme := schedulerScheme(t)
	svc := minimalLLMSvc("mistral", "prod")

	r := &ConfigReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	// First reconcile — creates ConfigMap
	require.NoError(t, r.Reconcile(context.Background(), svc))

	// Second reconcile — should update without error
	require.NoError(t, r.Reconcile(context.Background(), svc))

	var cm corev1.ConfigMap
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "mistral-scheduler-config", Namespace: "prod"}, &cm))
	assert.Contains(t, cm.Data["scheduler.yaml"], "mistral")
}

func TestConfigReconciler_ConfigMapName(t *testing.T) {
	scheme := schedulerScheme(t)
	svc := minimalLLMSvc("phi-3-mini", "kube-system")

	r := &ConfigReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.Reconcile(context.Background(), svc))

	var cm corev1.ConfigMap
	err := r.Get(context.Background(),
		types.NamespacedName{Name: "phi-3-mini-scheduler-config", Namespace: "kube-system"}, &cm)
	assert.NoError(t, err)
}
