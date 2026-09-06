/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// Regression: the 2026-09-01 live incident. The URI pvc://qwen38-27b-weights
// matched the Qwen3-7B preset by raw substring ("qwen3" inside "qwen38",
// "7b" inside "27b"); the preset merge then appended the preset's
// "--max-model-len 32768" value as a bare item (the flag was deduped) and
// duplicated "--tool-call-parser" in split form against the user's equals
// form. The rendered pod died with "unrecognized arguments: 32768".

func TestContainsToken(t *testing.T) {
	cases := []struct {
		name  string
		s     string
		token string
		want  bool
	}{
		{"prefix family is not a token", "pvc://qwen38-27b-weights", "qwen3", false},
		{"embedded size is not a token", "pvc://qwen38-27b-weights", "7b", false},
		{"standalone size is a token", "pvc://qwen38-27b-weights", "27b", true},
		{"plain family match", "hf://Qwen/Qwen3-7B", "qwen3", true},
		{"plain size match", "hf://Qwen/Qwen3-7B", "7b", true},
		{"72b size match", "hf://Qwen/Qwen3-72B", "72b", true},
		{"72b is not 7b", "hf://Qwen/Qwen3-72B", "7b", false},
		{"case insensitive", "OCI://Registry/QWEN38-27B", "qwen3", false},
		{"gemma variant", "google/gemma-4-E2B-it", "e2b", true},
		{"gemma wrong variant", "google/gemma-4-E4B-it", "e2b", false},
		{"trailing dash boundary", "swfs://filer/qwen3-7b/", "7b", true},
		{"no match", "pvc://llama-weights", "qwen3", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, containsToken(tc.s, tc.token))
		})
	}
}

func TestGetWellKnownConfig_NoPresetForEmbeddedFamily(t *testing.T) {
	// The incident URI must not match any Qwen3 preset.
	require.Nil(t, GetWellKnownConfig("pvc://qwen38-27b-weights"))
	// Real model URIs keep their presets.
	require.NotNil(t, GetWellKnownConfig("hf://Qwen/Qwen3-7B"))
	require.NotNil(t, GetWellKnownConfig("hf://Qwen/Qwen3-72B"))
}

func TestMergePresetArgs_SplitValueNeverOrphaned(t *testing.T) {
	// The exact incident shape: user args carry --max-model-len as a split
	// pair with a different value; the preset carries the same flag with
	// value 32768. The old item-wise merge appended the bare "32768".
	existing := []string{
		"--model=/mnt/models", "--max-model-len", "262144",
		"--gpu-memory-utilization=0.90",
		"--tool-call-parser=qwen3_xml", "--enable-auto-tool-choice",
	}
	preset := []string{
		"--max-model-len", "32768",
		"--trust-remote-code",
		"--gpu-memory-utilization", "0.90",
		"--enable-auto-tool-choice", "--tool-call-parser", "hermes",
	}
	got := mergePresetArgs(existing, preset)

	assert.NotContains(t, got, "32768", "preset value must not be appended bare")
	assert.NotContains(t, got, "hermes", "preset parser must not override the user's")
	assert.NotContains(t, got, "--gpu-memory-utilization", "equals-form user arg must suppress the split-form preset")
	assert.Contains(t, got, "--trust-remote-code", "absent preset flags are still merged")

	count := 0
	for _, a := range got {
		if strings.HasPrefix(a, "--tool-call-parser") {
			count++
		}
	}
	assert.Equal(t, 1, count, "exactly one tool-call-parser flag may remain")
}

func TestMergePresetArgs_UserValueWinsAcrossForms(t *testing.T) {
	existing := []string{"--gpu-memory-utilization=0.85"}
	preset := []string{"--gpu-memory-utilization", "0.90"}
	got := mergePresetArgs(existing, preset)
	assert.Equal(t, existing, got, "equals-form user value suppresses the split-form preset entirely")
}

func TestMergePresetArgs_AppendsAbsentPairsAtomically(t *testing.T) {
	got := mergePresetArgs([]string{"--model=/mnt/models"}, []string{"--max-model-len", "32768", "--enforce-eager"})
	assert.Equal(t, []string{"--model=/mnt/models", "--max-model-len", "32768", "--enforce-eager"}, got)
}

func TestApplyConfigToSpec_IncidentReproduction(t *testing.T) {
	r := &LLMInferenceServiceReconciler{}
	spec := &servingv1alpha2.LLMInferenceServiceSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm"}}},
		},
	}
	spec.Template.Spec.Containers[0].Args = []string{
		"--model=/mnt/models", "--served-model-name=qwen3-8-27b-nvfp4",
		"--tensor-parallel-size=1", "--max-model-len", "262144",
		"--gpu-memory-utilization=0.90", "--trust-remote-code",
		"--enable-auto-tool-choice", "--tool-call-parser=qwen3_xml",
		"--reasoning-parser=qwen3", "--quantization=compressed-tensors",
		"--max-num-seqs=64",
	}
	cfg := GetWellKnownConfig("pvc://qwen38-27b-weights")
	require.Nil(t, cfg, "the incident URI must not trigger a preset")

	// Even when a preset DOES fire (legit Qwen3 URI), the merged args must be
	// runnable: no bare values, no duplicate flags.
	cfg = GetWellKnownConfig("hf://Qwen/Qwen3-7B")
	require.NotNil(t, cfg)
	r.ApplyConfigToSpec(spec, cfg)
	args := spec.Template.Spec.Containers[0].Args
	for _, a := range args {
		if a == "32768" || a == "hermes" {
			t.Fatalf("orphaned preset item %q reached the rendered args: %v", a, args)
		}
	}
	seen := map[string]int{}
	for _, a := range args {
		name := a
		if eq := strings.Index(a, "="); eq >= 0 {
			name = a[:eq]
		}
		seen[name]++
	}
	for flag, n := range seen {
		assert.LessOrEqual(t, n, 1, "flag %s duplicated in rendered args", flag)
	}
}

func TestReconciliationPaused(t *testing.T) {
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen3-8-27b-nvfp4", Namespace: "cortaix-llm-inference"},
	}
	assert.False(t, reconciliationPaused(svc))
	svc.Annotations = map[string]string{AnnotationPauseReconciliation: "true"}
	assert.True(t, reconciliationPaused(svc))
	svc.Annotations[AnnotationPauseReconciliation] = "false"
	assert.False(t, reconciliationPaused(svc))
	assert.False(t, reconciliationPaused(nil))
}
