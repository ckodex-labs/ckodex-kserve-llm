/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestRerankerSyncStatusUnavailableAndReady(t *testing.T) {
	for _, test := range []struct {
		name      string
		ready     int32
		condition metav1.ConditionStatus
		reason    string
	}{
		{name: "unavailable", ready: 0, condition: metav1.ConditionFalse, reason: "Unavailable"},
		{name: "available", ready: 2, condition: metav1.ConditionTrue, reason: "Available"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scheme := rerankerTestScheme(t)
			service := newRerankerSvc("status-"+test.name, "default")
			service.Generation = 7
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: service.Name, Namespace: service.Namespace},
				Status:     appsv1.DeploymentStatus{Replicas: 3, ReadyReplicas: test.ready},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(service, deployment).
				WithStatusSubresource(&servingv1alpha2.RerankerInferenceService{}).
				Build()
			r := &RerankerInferenceServiceReconciler{Client: client, Scheme: scheme}

			require.NoError(t, r.syncStatus(context.Background(), service))
			updated := &servingv1alpha2.RerankerInferenceService{}
			require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, updated))
			assert.Equal(t, test.ready, updated.Status.Replicas)
			assert.Equal(t, int64(7), updated.Status.ObservedGeneration)
			assert.Equal(t, "http://"+service.Name+"."+service.Namespace+".svc.cluster.local/rerank", updated.Status.Endpoint)
			assert.Equal(t, test.condition, conditionFor(updated, servingv1alpha2.RerankerConditionReady).Status)
			assert.Equal(t, test.reason, conditionFor(updated, servingv1alpha2.RerankerConditionReady).Reason)
		})
	}
}

func TestRerankerSetCondition(t *testing.T) {
	scheme := rerankerTestScheme(t)
	service := newRerankerSvc("condition", "default")
	client := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(service).
		WithStatusSubresource(&servingv1alpha2.RerankerInferenceService{}).
		Build()
	r := &RerankerInferenceServiceReconciler{Client: client, Scheme: scheme}

	require.NoError(t, r.setCondition(context.Background(), service,
		servingv1alpha2.RerankerConditionDeploymentReady, metav1.ConditionFalse, "ReconcileError", "failed"))
	updated := &servingv1alpha2.RerankerInferenceService{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, updated))
	condition := conditionFor(updated, servingv1alpha2.RerankerConditionDeploymentReady)
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, "ReconcileError", condition.Reason)
}

func conditionFor(service *servingv1alpha2.RerankerInferenceService, conditionType string) metav1.Condition {
	for _, condition := range service.Status.Conditions {
		if condition.Type == conditionType {
			return condition
		}
	}
	return metav1.Condition{}
}
