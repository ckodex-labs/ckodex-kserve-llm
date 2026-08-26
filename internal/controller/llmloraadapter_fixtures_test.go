/* Copyright 2026 CKodex Authors. Licensed under the Apache License, Version 2.0. */
package controller

import (
	"net/http"
	"net/url"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

func buildLoraScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, discoveryv1.AddToScheme(s))
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	return s
}

func testLora(name, namespace, target string) *servingv1alpha2.LLMLoraAdapter {
	return &servingv1alpha2.LLMLoraAdapter{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: k8stypes.UID("lora-uid")}, Spec: servingv1alpha2.LLMLoraAdapterSpec{TargetService: target, AdapterName: "sql-helper", Model: servingv1alpha2.ModelSpec{URI: "hf://org/lora-weights", Name: "sql-helper"}}}
}

type roundTripperMock struct{ targetURL string }

func (m *roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	newReq := req.Clone(req.Context())
	target, err := url.Parse(m.targetURL)
	if err != nil {
		return nil, err
	}
	newReq.URL.Scheme, newReq.URL.Host = target.Scheme, target.Host
	return http.DefaultTransport.RoundTrip(newReq)
}
