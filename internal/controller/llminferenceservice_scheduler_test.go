/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestReconcileSchedulerBlocked_UpdatesStatusAcrossBranches(t *testing.T) {
	tests := []struct {
		name         string
		condition    metav1.Condition
		cause        error
		requeueAfter time.Duration
		wantError    bool
	}{
		{
			name: "pending readiness",
			condition: metav1.Condition{
				Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
				Reason: "EndpointPickerUnavailable", Message: "waiting",
			},
			requeueAfter: 5 * time.Second,
		},
		{
			name: "feature disabled",
			condition: metav1.Condition{
				Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
				Reason: "SchedulerFeatureDisabled", Message: "scheduler is disabled",
			},
			cause:     errors.New("scheduler is disabled"),
			wantError: true,
		},
		{
			name: "reconcile failed",
			condition: metav1.Condition{
				Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionFalse,
				Reason: "SchedulerReconcileFailed", Message: "scheduler failed",
			},
			cause:     errors.New("scheduler failed"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := buildLLMScheme(t)
			llmSvc := makeLLMInferenceService("scheduler-test", "default")
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(llmSvc).
				WithStatusSubresource(llmSvc).
				Build()
			r := &LLMInferenceServiceReconciler{Client: client}

			result, err := r.reconcileSchedulerBlocked(
				context.Background(), llmSvc, llmSvc.DeepCopy(), tt.condition, tt.cause, tt.requeueAfter,
			)

			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.requeueAfter, result.RequeueAfter)

			var updated servingv1alpha2.LLMInferenceService
			require.NoError(t, client.Get(context.Background(), clientKey(llmSvc), &updated))
			condition := meta.FindStatusCondition(updated.Status.Conditions, servingv1alpha2.ConditionSchedulerReady)
			require.NotNil(t, condition)
			assert.Equal(t, tt.condition.Reason, condition.Reason)
			ready := meta.FindStatusCondition(updated.Status.Conditions, servingv1alpha2.ConditionReady)
			require.NotNil(t, ready)
			assert.Equal(t, metav1.ConditionFalse, ready.Status)
		})
	}
}

func clientKey(obj *servingv1alpha2.LLMInferenceService) client.ObjectKey {
	return client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}
}
