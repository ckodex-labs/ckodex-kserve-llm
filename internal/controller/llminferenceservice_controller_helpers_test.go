/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/cleanup"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/status"
)

// --- helpers ---

func nodeWithArch(arch string, capacity map[corev1.ResourceName]resource.Quantity, labels map[string]string) corev1.Node {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-" + arch,
			Labels: labels,
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				Architecture: arch,
			},
		},
	}
	if capacity != nil {
		node.Status.Capacity = capacity
	}
	return node
}

func reconcilerWithNodes(t *testing.T, nodes ...corev1.Node) (*LLMInferenceServiceReconciler, context.Context) {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)
	_ = servingv1alpha2.AddToScheme(scheme)

	cb := fake.NewClientBuilder().WithScheme(scheme)
	for _, n := range nodes {
		n := n
		cb = cb.WithObjects(&n)
	}

	cl := cb.Build()
	rec := record.NewFakeRecorder(10)
	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client: cl,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: scheme,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client: cl,
			Scheme: scheme,
		},
	}
	return r, context.Background()
}

func basePodSpec() *corev1.PodSpec {
	return &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "vllm",
				Image: "", // empty = let the controller set it
			},
		},
	}
}

func baseLLMInferenceService() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
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
		},
	}
}

func assertEnvVar(t *testing.T, envs []corev1.EnvVar, name, expected string) {
	t.Helper()
	for _, ev := range envs {
		if ev.Name == name {
			if ev.Value != expected {
				t.Errorf("env %s = %q, want %q", name, ev.Value, expected)
			}
			return
		}
	}
	t.Errorf("env %s not found", name)
}

func assertArgPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			if args[i+1] != value {
				t.Errorf("arg %s = %q, want %q", flag, args[i+1], value)
			}
			return
		}
	}
	t.Errorf("arg %s not found in %v", flag, args)
}
