/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOptionalInferenceTemplatesRemainOmittedAfterFinalizerMutation(t *testing.T) {
	tests := []struct {
		name     string
		resource any
	}{
		{
			name: "asr",
			resource: &ASRInferenceService{
				ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"serving.ckodex.com/asr-finalizer"}},
				Spec: ASRInferenceServiceSpec{
					Model: ModelSpec{URI: "hf://Systran/faster-whisper-small"},
				},
			},
		},
		{
			name: "embedding",
			resource: &EmbeddingInferenceService{
				ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"serving.ckodex.com/embedding-finalizer"}},
				Spec: EmbeddingInferenceServiceSpec{
					Model: ModelSpec{URI: "hf://BAAI/bge-small-en-v1.5"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := serializedSpec(t, tc.resource)
			assert.NotContains(t, spec, "template",
				"an omitted template must not become an invalid empty PodTemplateSpec")
		})
	}
}

func TestExplicitInferenceTemplatesRemainSerialized(t *testing.T) {
	template := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "metrics", Image: "registry.example/metrics:v1"}},
		},
	}
	resource := &ASRInferenceService{
		Spec: ASRInferenceServiceSpec{
			Model:    ModelSpec{URI: "hf://Systran/faster-whisper-small"},
			Template: template,
		},
	}

	spec := serializedSpec(t, resource)
	require.Contains(t, spec, "template")

	var serializedTemplate corev1.PodTemplateSpec
	require.NoError(t, json.Unmarshal(spec["template"], &serializedTemplate))
	require.Len(t, serializedTemplate.Spec.Containers, 1)
	assert.Equal(t, "metrics", serializedTemplate.Spec.Containers[0].Name)
}

func serializedSpec(t *testing.T, resource any) map[string]json.RawMessage {
	t.Helper()

	data, err := json.Marshal(resource)
	require.NoError(t, err)

	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &object))
	require.Contains(t, object, "spec")

	var spec map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(object["spec"], &spec))
	return spec
}
