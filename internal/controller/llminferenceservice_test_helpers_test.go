package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (r *LLMInferenceServiceReconciler) buildDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32) *appsv1.Deployment {
	hwType := r.HardwareCache.Get(ctx, r.Client, r.APIReader)
	return r.DeploymentBuilder.Build(ctx, llmSvc, replicas, hwType, nil)
}

func (r *LLMInferenceServiceReconciler) buildStorageInitializer(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, lmc *servingv1alpha2.LocalModelCache) *corev1.Container {
	hwType := r.HardwareCache.Get(ctx, r.Client, r.APIReader)
	return r.DeploymentBuilder.BuildStorageInitializer(ctx, llmSvc, hwType, lmc)
}
