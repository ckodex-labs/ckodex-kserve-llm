/* Copyright 2026 CKodex Authors. Licensed under the Apache License, Version 2.0. */
package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRegisterWithTargetService_Success(t *testing.T) {
	s := buildLoraScheme(t)
	requests := make(chan VLLMLoadLoraRequest, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/load_lora_adapter":
			var body VLLMLoadLoraRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			requests <- body
			w.WriteHeader(http.StatusOK)
		case "/v1/completions":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	lora := testLora("my-lora", "default", "my-llm")
	svc := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "llm-pod", Namespace: "default", Labels: map[string]string{"app.kubernetes.io/instance": "my-llm"}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "1.2.3.4"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10), HTTPClient: &http.Client{Transport: &roundTripperMock{targetURL: srv.URL}}}
	require.NoError(t, r.registerWithTargetService(context.Background(), lora, svc))
	select {
	case body := <-requests:
		assert.Equal(t, "sql-helper", body.LoraName)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for vLLM adapter-load request")
	}
	assert.Empty(t, requests)
}

func TestRegisterWithTargetService_NoPods(t *testing.T) {
	s := buildLoraScheme(t)
	lora, svc := testLora("my-lora", "default", "my-llm"), &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"}}
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	err := r.registerWithTargetService(context.Background(), lora, svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pods found")
}

func TestRegisterWithTargetService_HTTPError(t *testing.T) {
	s := buildLoraScheme(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	lora, svc := testLora("my-lora", "default", "my-llm"), &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "llm-pod", Namespace: "default", Labels: map[string]string{"app.kubernetes.io/instance": "my-llm"}}, Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "1.2.3.4"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10), HTTPClient: &http.Client{Transport: &roundTripperMock{targetURL: srv.URL}}}
	err := r.registerWithTargetService(context.Background(), lora, svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vLLM returned non-OK status 500")
}
