/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package integration

import (
	"context"
	"fmt"
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// TestLLMInferenceService_FinaliserAdded verifies that the operator adds its
// finalizer to a newly-created LLMInferenceService.
func TestLLMInferenceService_FinaliserAdded(t *testing.T) {
	t.Parallel()
	name := fmt.Sprintf("llmsvc-finalizer-%d", uniqueID())
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: name,
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "model-server",
						Image: "ckodex/model-server:latest",
					}},
				},
			},
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, svc))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, svc) })

	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var s servingv1alpha2.LLMInferenceService
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(svc), &s); err != nil {
				return false, nil
			}
			return slices.Contains(s.Finalizers, "serving.ckodex.com/finalizer"), nil
		},
	))
}

// TestLLMInferenceService_ConditionsSetOnReconcile verifies that the operator
// sets at least one condition on the LLMInferenceService after a reconcile pass.
func TestLLMInferenceService_ConditionsSetOnReconcile(t *testing.T) {
	t.Parallel()
	name := fmt.Sprintf("llmsvc-conditions-%d", uniqueID())
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: name,
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "model-server",
						Image: "ckodex/model-server:latest",
					}},
				},
			},
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, svc))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, svc) })

	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var s servingv1alpha2.LLMInferenceService
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(svc), &s); err != nil {
				return false, nil
			}
			return len(s.Status.Conditions) > 0, nil
		},
	))

	var s servingv1alpha2.LLMInferenceService
	require.NoError(t, suite.client.Get(suite.ctx, client.ObjectKeyFromObject(svc), &s))
	assert.NotEmpty(t, s.Status.Conditions)
}

// TestLLMInferenceService_DeletionRemovesFinalizer verifies that the finalizer
// is removed from the object when it is deleted, allowing the API server to
// complete the deletion.
func TestLLMInferenceService_DeletionRemovesFinalizer(t *testing.T) {
	t.Parallel()
	name := fmt.Sprintf("llmsvc-delete-%d", uniqueID())
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: name,
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "model-server",
						Image: "ckodex/model-server:latest",
					}},
				},
			},
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, svc))

	// Wait for finalizer to be set.
	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var s servingv1alpha2.LLMInferenceService
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(svc), &s); err != nil {
				return false, nil
			}
			return slices.Contains(s.Finalizers, "serving.ckodex.com/finalizer"), nil
		},
	))

	// Delete the object.
	require.NoError(t, suite.client.Delete(suite.ctx, svc))

	// Wait for the object to be fully removed (finalizer removed by controller).
	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var s servingv1alpha2.LLMInferenceService
			err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(svc), &s)
			if err != nil {
				// Not found = object deleted = success
				return true, nil
			}
			return false, nil
		},
	))
}

// TestLLMInferenceService_ReadyConditionAfterStatusPatch verifies that once the
// controller patches ModelReady=true into the status, the Ready condition
// becomes True.
func TestLLMInferenceService_ReadyConditionAfterStatusPatch(t *testing.T) {
	t.Parallel()
	name := fmt.Sprintf("llmsvc-ready-%d", uniqueID())
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: name,
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "model-server",
						Image: "ckodex/model-server:latest",
					}},
				},
			},
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, svc))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, svc) })

	// In addition to LLMInferenceService status, we must mock the Deployment status
	// because the controller fetches the Deployment to calculate ModelReady.
	// Since envtest doesn't run the deployment controller, ReadyReplicas is 0 by default.
	// Wait for the deployment to be created by the controller.
	var dep appsv1.Deployment
	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(ctx context.Context) (bool, error) {
			err := suite.client.Get(ctx, client.ObjectKeyFromObject(svc), &dep)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			return true, nil
		},
	))
	fmt.Printf("\n--- DEBUG Deployment Labels: %v\n", dep.Labels)
	fmt.Printf("--- DEBUG Deployment Selector: %v\n", dep.Spec.Selector.MatchLabels)
	fmt.Printf("--- DEBUG Deployment Template Labels: %v\n\n", dep.Spec.Template.Labels)
	patch := client.MergeFrom(dep.DeepCopy())
	dep.Status.Replicas = 1
	dep.Status.ReadyReplicas = 1
	require.NoError(t, suite.client.Status().Patch(suite.ctx, &dep, patch))

	// Manually set ModelReady (in production this is set by the deployment controller).
	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var s servingv1alpha2.LLMInferenceService
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(svc), &s); err != nil {
				return false, nil
			}
			// Only patch once the resource exists and the controller has touched it.
			if len(s.Finalizers) == 0 {
				return false, nil
			}
			patch := client.MergeFrom(s.DeepCopy())
			s.Status.ModelReady = true
			s.Status.Replicas = 1
			if err := suite.client.Status().Patch(suite.ctx, &s, patch); err != nil {
				return false, nil
			}
			return true, nil
		},
	))

	// Verify the Ready condition is True.
	err := wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var s servingv1alpha2.LLMInferenceService
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(svc), &s); err != nil {
				return false, nil
			}
			cond := meta.FindStatusCondition(s.Status.Conditions, string(servingv1alpha2.ConditionReady))
			if cond != nil && cond.Status == metav1.ConditionTrue {
				assert.Equal(t, "Ready", cond.Reason)
				assert.Equal(t, "Model is loaded and serving", cond.Message)
				return true, nil
			}
			return false, nil
		},
	)
	require.NoError(t, err)
}
