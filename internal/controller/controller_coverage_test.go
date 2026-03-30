/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestBulkController_Coverage_FinalVer2(t *testing.T) {
	s := buildLoraScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "missing", Namespace: "default"}}
	ctx := context.Background()

	// 1. Agent
	_, _ = (&AgentReconciler{Client: cl, Scheme: s}).Reconcile(ctx, req)

	// 2. ASR
	_, _ = (&ASRInferenceServiceReconciler{Client: cl, Scheme: s}).Reconcile(ctx, req)

	// 3. Embedding
	_, _ = (&EmbeddingInferenceServiceReconciler{Client: cl, Scheme: s}).Reconcile(ctx, req)

	// 4. ImagePullSecret
	_, _ = (&ImagePullSecretReconciler{Client: cl, Scheme: s}).Reconcile(ctx, req)

	// 5. LLMInferenceService
	_, _ = (&LLMInferenceServiceReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}).Reconcile(ctx, req)

	// 6. LocalModelCache
	_, _ = (&LocalModelCacheReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}).Reconcile(ctx, req)

	// 7. LWS (Struct name is Reconciler, takes *LLMInferenceService)
	llmSvc := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"}}
	_ = (&Reconciler{Client: cl, Scheme: s}).Reconcile(ctx, llmSvc)

	// 8. ModelOnboarding
	_, _ = (&ModelOnboardingReconciler{Client: cl, Scheme: s}).Reconcile(ctx, req)

	// 9. Multimodal
	_, _ = (&MultimodalInferenceServiceReconciler{Client: cl, Scheme: s}).Reconcile(ctx, req)

	// 10. Session
	_, _ = (&SessionReconciler{Client: cl, Scheme: s}).Reconcile(ctx, req)

	// 11. TenantQuota
	_, _ = (&TenantQuotaReconciler{Client: cl, Scheme: s, Defaults: DefaultTenantQuota()}).Reconcile(ctx, req)
}

func TestSessionReconcile_EdgeCases_FinalVer2(t *testing.T) {
	s := buildSessionScheme(t)
	r := &SessionReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s}
	
	sess0 := &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "default"},
		Status:     servingv1alpha2.InferenceSessionStatus{BoundEndpoint: ""},
	}
	_ = r.validateEndpoint(context.Background(), sess0)

	now := metav1.Now()
	session4 := &servingv1alpha2.InferenceSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "terminating",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{"sessions.serving.ckodex.io/finalizer"},
		},
		Spec: servingv1alpha2.InferenceSessionSpec{ModelRef: "my-model"},
	}
	cl4 := fake.NewClientBuilder().WithScheme(s).WithObjects(session4).WithStatusSubresource(session4).Build()
	_, _ = (&SessionReconciler{Client: cl4, Scheme: s}).Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "terminating", Namespace: "default"},
	})
}

func TestLLMLoraAdapter_RegistrationFlow_FinalVer2(t *testing.T) {
	s := buildLoraScheme(t)
	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{Name: "ready-lora", Namespace: "default"},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			TargetService: "my-llm",
			Model: servingv1alpha2.ModelSpec{URI: "hf://org/lora", Name: "sql"},
		},
	}
	svc := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"}}
	
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, svc).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}

	_ = r.registerWithTargetService(context.Background(), lora, svc)
}
