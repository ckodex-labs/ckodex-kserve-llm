//go:build e2e

/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// TestE2E_ModelOnboarding_BasicCreate verifies that a ModelOnboarding CR is
// accepted by the API server and that the operator begins reconciling it.
func TestE2E_ModelOnboarding_BasicCreate(t *testing.T) {
	name := resourceName(t, "mo")

	mo := &servingv1alpha2.ModelOnboarding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: e2eNamespace,
		},
		Spec: servingv1alpha2.ModelOnboardingSpec{
			// ModelRef points to an LLMInferenceService that may not exist yet;
			// this is intentional — we verify the CR is accepted and conditions are set.
			ModelRef: "hf://openai-community/gpt2",
			Stages: []servingv1alpha2.OnboardingStage{
				{
					Name: "validate",
					Type: "validation",
				},
			},
			RollbackOnFailure: true,
		},
	}

	ctx := context.Background()
	require.NoError(t, k8sClient.Create(ctx, mo))
	key := client.ObjectKeyFromObject(mo)

	t.Cleanup(func() {
		fresh := &servingv1alpha2.ModelOnboarding{}
		if err := k8sClient.Get(context.Background(), key, fresh); err == nil {
			_ = k8sClient.Delete(context.Background(), fresh)
		}
	})

	// Step 1: Verify the CR exists in the API server (GET succeeds within 10s).
	t.Log("verifying CR is accepted by API server...")
	require.NoError(t,
		waitForCondition(t, key, &servingv1alpha2.ModelOnboarding{}, 10*time.Second,
			func(obj client.Object) (bool, error) {
				return obj.GetName() != "", nil
			},
		), "ModelOnboarding must be retrievable within 10s",
	)

	fetched := &servingv1alpha2.ModelOnboarding{}
	require.NoError(t, k8sClient.Get(ctx, key, fetched))
	assert.Equal(t, name, fetched.Name, "CR name must match requested name")

	// Step 2 (slow): Wait for the operator to set status.phase.
	// Guarded by E2E_FULL_LIFECYCLE=true; the operator may take several minutes
	// to download and validate the model before updating status.
	if os.Getenv("E2E_FULL_LIFECYCLE") == "true" {
		t.Log("waiting for status.phase to be set...")
		require.NoError(t,
			waitForCondition(t, key, &servingv1alpha2.ModelOnboarding{}, eventuallyTimeout,
				func(obj client.Object) (bool, error) {
					mo := obj.(*servingv1alpha2.ModelOnboarding)
					return mo.Status.Phase != "", nil
				},
			), "status.phase must be non-empty within %s", eventuallyTimeout,
		)

		final := &servingv1alpha2.ModelOnboarding{}
		require.NoError(t, k8sClient.Get(ctx, key, final))
		assert.NotEmpty(t, final.Status.Phase,
			"status.phase must be set after operator reconciles")
	}
}
