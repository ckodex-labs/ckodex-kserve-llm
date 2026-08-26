/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/cleanup"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/evidence"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/status"
	"k8s.io/client-go/tools/record"
)

func buildLLMScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, autoscalingv2.AddToScheme(s))
	require.NoError(t, policyv1.AddToScheme(s))
	require.NoError(t, networkingv1.AddToScheme(s))
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, gwapiv1.Install(s))
	return s
}
func makeLLMInferenceService(name, namespace string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       k8stypes.UID("test-uid-" + name),
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://test/model",
				Name: "test-model",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "vllm"},
					},
				},
			},
			Router: servingv1alpha2.RouterSpec{
				Gateway: servingv1alpha2.GatewaySpec{
					Managed: &servingv1alpha2.ManagedGatewaySpec{
						GatewayClassName: "envoy",
					},
				},
			},
		},
	}
}
func setupReconciler(cl client.Client, s *runtime.Scheme) *LLMInferenceServiceReconciler {
	rec := record.NewFakeRecorder(10)
	return &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client:          cl,
			EnableHardening: true,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client:     cl,
			Scheme:     s,
			EnableGRPC: false,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
		GovernanceReconciler: &evidence.GovernanceReconciler{
			Client: cl,
		},
	}
}

// TestLLMInferenceService_ReconcileNotFound returns no error when CR is missing.
