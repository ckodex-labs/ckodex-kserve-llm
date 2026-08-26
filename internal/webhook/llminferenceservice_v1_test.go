/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package webhook_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	"github.com/ckodex-labs/kserve-llm-operator/internal/webhook"
)

func stableFromAlpha(t *testing.T) *servingv1.LLMInferenceService {
	t.Helper()
	stable := &servingv1.LLMInferenceService{}
	require.NoError(t, minimalValidSvc().ConvertTo(stable))
	stable.TypeMeta = metav1.TypeMeta{
		APIVersion: servingv1.SchemeGroupVersion.String(),
		Kind:       "LLMInferenceService",
	}
	return stable
}

func TestStableValidator_UsesSharedValidation(t *testing.T) {
	validator := &webhook.StableLLMInferenceServiceValidator{}
	warnings, err := validator.ValidateCreate(context.Background(), stableFromAlpha(t))
	assert.NoError(t, err)
	assert.Empty(t, warnings)

	invalid := stableFromAlpha(t)
	invalid.Spec.Model.URI = "ftp://invalid"
	_, err = validator.ValidateCreate(context.Background(), invalid)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.model.uri must start with one of")

	unsupportedEngine := stableFromAlpha(t)
	if unsupportedEngine.Spec.Experimental == nil {
		unsupportedEngine.Spec.Experimental = &servingv1.ExperimentalSpec{}
	}
	unsupportedEngine.Spec.Experimental.Engine = "other"
	_, err = validator.ValidateCreate(context.Background(), unsupportedEngine)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported inference engine")
}

func TestStableValidator_RefusesLossyInlineSchedulerConfig(t *testing.T) {
	stable := stableFromAlpha(t)
	stable.Spec.Router.Scheduler = &servingv1.SchedulerSpec{
		Pool: servingv1.InferencePoolSpec{},
		Config: &servingv1.SchedulerConfigSpec{
			Inline: &servingv1.EndpointPickerConfigSpec{Plugins: []string{"queue-scorer"}},
		},
	}

	_, err := (&webhook.StableLLMInferenceServiceValidator{}).ValidateCreate(context.Background(), stable)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.router.scheduler.config.inline")
}

func TestStableDefaulter_ConvertsDefaultsBackToV1(t *testing.T) {
	stable := stableFromAlpha(t)
	stable.Spec.Replicas = nil
	stable.Spec.Template.Spec.Containers[0].SecurityContext = nil
	stable.Spec.Template.Spec.Containers[0].Ports = nil

	err := (&webhook.StableLLMInferenceServiceDefaulter{}).Default(context.Background(), stable)
	require.NoError(t, err)
	require.NotNil(t, stable.Spec.Replicas)
	assert.Equal(t, int32(1), *stable.Spec.Replicas)
	require.NotNil(t, stable.Spec.Template.Spec.Containers[0].SecurityContext)
	require.NotNil(t, stable.Spec.Template.Spec.Containers[0].SecurityContext.RunAsNonRoot)
	assert.True(t, *stable.Spec.Template.Spec.Containers[0].SecurityContext.RunAsNonRoot)
	assert.Len(t, stable.Spec.Template.Spec.Containers[0].Ports, 2)
	assert.Equal(t, "serving.ckodex.com/v1", stable.APIVersion)
}
