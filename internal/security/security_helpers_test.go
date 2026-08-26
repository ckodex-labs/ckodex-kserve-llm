/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ---- scheme helpers --------------------------------------------------------

func secScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, networkingv1.AddToScheme(s))

	// Register unstructured GVKs for isGVKAvailable checks
	s.AddKnownTypeWithName(ExternalSecretGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(ConstraintTemplateGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(TracingPolicyGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1beta1",
		Kind:    "Sidecar",
	}, &unstructured.Unstructured{})

	return s
}

func minimalLLMSvc(name, ns string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: name,
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
		},
	}
}
