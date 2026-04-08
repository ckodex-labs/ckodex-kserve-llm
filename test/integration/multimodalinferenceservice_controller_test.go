/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// TestMultimodalInferenceService_LiquidAI_TTS verifies that a MultimodalInferenceService
// with task=text-to-speech and LiquidAI model URI is correctly reconciled
// with the --trust-remote-code flag and appropriate TTS settings.
func TestMultimodalInferenceService_LiquidAI_TTS(t *testing.T) {
	name := fmt.Sprintf("liquid-tts-%d", uniqueID())
	mmSvc := &servingv1alpha2.MultimodalInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: servingv1alpha2.MultimodalInferenceServiceSpec{
			Task: servingv1alpha2.MultimodalTaskTextToSpeech,
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://LiquidAI/LFM2.5-Audio-1.5B",
				Name: "lfm2.5-audio",
			},
			Runtime: servingv1alpha2.MultimodalRuntimeVLLM,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "runtime",
						Image: "ckodex/runtime:latest",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								"nvidia.com/gpu": resource.MustParse("1"),
							},
						},
					}},
				},
			},
		},
	}

	require.NoError(t, suite.client.Create(suite.ctx, mmSvc))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, mmSvc) })

	// Wait for the finalizer and condition to appear (reconciler ran).
	err := wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var s servingv1alpha2.MultimodalInferenceService
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(mmSvc), &s); err != nil {
				return false, nil
			}
			return len(s.Finalizers) > 0, nil
		},
	)
	require.NoError(t, err, "MultimodalInferenceService should be processed by controller")

	// Verify the Deployment was created with correct args.
	err = wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var s servingv1alpha2.MultimodalInferenceService
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(mmSvc), &s); err != nil {
				return false, nil
			}
			// The Deployment should have our expected args.
			// We can check this via the controller's logic indirectly or by fetching the Deployment.
			return true, nil
		},
	)
	require.NoError(t, err)

	// Fetch the underlying Deployment to verify args.
	var dep appsv1.Deployment
	require.NoError(t, suite.client.Get(suite.ctx, client.ObjectKeyFromObject(mmSvc), &dep))

	container := dep.Spec.Template.Spec.Containers[0]
	assert.Contains(t, container.Args, "--trust-remote-code")
	assert.Contains(t, container.Args, "--enforce-eager")
	assert.NotContains(t, container.Args, "--limit-mm-per-prompt") // Vision-only flag.

	// Mock deployment status to Ready
	patch := client.MergeFrom(dep.DeepCopy())
	dep.Status.Replicas = 1
	dep.Status.ReadyReplicas = 1
	require.NoError(t, suite.client.Status().Patch(suite.ctx, &dep, patch))

	// Verify status is updated to Ready
	err = wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(ctx context.Context) (bool, error) {
			var m servingv1alpha2.MultimodalInferenceService
			if err := suite.client.Get(ctx, client.ObjectKeyFromObject(mmSvc), &m); err != nil {
				return false, nil
			}
			cond := meta.FindStatusCondition(m.Status.Conditions, "Ready")
			return cond != nil && cond.Status == metav1.ConditionTrue, nil
		},
	)
	require.NoError(t, err)

	// Verify URL
	var m servingv1alpha2.MultimodalInferenceService
	require.NoError(t, suite.client.Get(suite.ctx, client.ObjectKeyFromObject(mmSvc), &m))
	assert.NotEmpty(t, m.Status.URL)
	assert.Contains(t, m.Status.URL, "/v1/audio/speech")
}

func TestMultimodalInferenceService_InvalidURI(t *testing.T) {
	name := fmt.Sprintf("mmsvc-invalid-%d", uniqueID())
	svc := &servingv1alpha2.MultimodalInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: servingv1alpha2.MultimodalInferenceServiceSpec{
			Task:    servingv1alpha2.MultimodalTaskVisionLanguage,
			Runtime: servingv1alpha2.MultimodalRuntimeVLLM,
			Model: servingv1alpha2.ModelSpec{
				Name: "invalid-repo",
				URI:  "s3://bucket/model", // Invalid URI, only hf:// supported for now
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "runtime",
						Image: "ckodex/runtime:latest",
					}},
				},
			},
		},
	}

	require.NoError(t, suite.client.Create(suite.ctx, svc))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, svc) })

	// Verify status is Not Ready due to InvalidURI
	err := wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(ctx context.Context) (bool, error) {
			var m servingv1alpha2.MultimodalInferenceService
			if err := suite.client.Get(ctx, client.ObjectKeyFromObject(svc), &m); err != nil {
				return false, nil
			}
			cond := meta.FindStatusCondition(m.Status.Conditions, "Ready")
			if cond != nil && cond.Status == metav1.ConditionFalse {
				return cond.Reason == "InvalidModelURI", nil
			}
			return false, nil
		},
	)
	require.NoError(t, err)
}
