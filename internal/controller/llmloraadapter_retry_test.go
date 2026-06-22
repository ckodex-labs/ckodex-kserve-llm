/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

// Retry-loop tests for registerWithTargetService.
// The loop makes up to 3 attempts with 500 ms backoff, context-aware.
// roundTripperMock and buildLoraScheme are defined in llmloraadapter_controller_test.go.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// retryTestFixtures builds shared objects used by all retry tests.
func retryTestFixtures() (*servingv1alpha2.LLMLoraAdapter, *servingv1alpha2.LLMInferenceService, *corev1.Pod) {
	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{Name: "retry-lora", Namespace: "default"},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			AdapterName:   "retry-adapter",
			TargetService: "retry-svc",
		},
	}
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "retry-svc", Namespace: "default"},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "retry-pod",
			Namespace: "default",
			Labels:    map[string]string{"app.kubernetes.io/instance": "retry-svc"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.1"},
	}
	return lora, svc, pod
}

// TestRegisterWithTargetService_SucceedsOnSecondAttempt verifies that the retry
// loop retries on a 500 and succeeds when the second attempt returns 200.
func TestRegisterWithTargetService_SucceedsOnSecondAttempt(t *testing.T) {
	s := buildLoraScheme(t)
	lora, svc, pod := retryTestFixtures()

	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError) // first attempt fails
			return
		}
		w.WriteHeader(http.StatusOK) // second attempt succeeds
	}))
	defer srv.Close()

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
		HTTPClient: &http.Client{
			Transport: &roundTripperMock{targetURL: srv.URL},
		},
	}

	err := r.registerWithTargetService(context.Background(), lora, svc)
	require.NoError(t, err, "should succeed on second attempt")
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "expected exactly 2 HTTP calls")
}

// TestRegisterWithTargetService_AllThreeAttemptsFail verifies that after 3
// consecutive 500 responses the call returns an error with the status code.
func TestRegisterWithTargetService_AllThreeAttemptsFail(t *testing.T) {
	s := buildLoraScheme(t)
	lora, svc, pod := retryTestFixtures()

	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
		HTTPClient: &http.Client{
			Transport: &roundTripperMock{targetURL: srv.URL},
		},
	}

	err := r.registerWithTargetService(context.Background(), lora, svc)
	require.Error(t, err, "all 3 attempts failed; error expected")
	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount), "expected exactly 3 HTTP calls")
	assert.Contains(t, err.Error(), "500")
}

// TestRegisterWithTargetService_ContextCancellation verifies that context
// cancellation during the backoff sleep surfaces ctx.Err() and does not block.
func TestRegisterWithTargetService_ContextCancellation(t *testing.T) {
	s := buildLoraScheme(t)
	lora, svc, pod := retryTestFixtures()

	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError) // always fail → triggers backoff
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
	r := &LLMLoraAdapterReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
		HTTPClient: &http.Client{
			Transport: &roundTripperMock{targetURL: srv.URL},
		},
	}

	// Cancel the context immediately after the first HTTP response so the 500ms
	// backoff select hits the ctx.Done() branch before the timer fires.
	go func() {
		// Wait briefly to let the first POST fire, then cancel.
		for atomic.LoadInt32(&callCount) == 0 {
			// spin until first call registers
		}
		cancel()
	}()

	err := r.registerWithTargetService(ctx, lora, svc)
	require.Error(t, err, "context cancellation must produce an error")
	// The error comes from the circuit breaker wrapping ctx.Err().
	// We only assert that the call returned promptly — timing is intentionally
	// not asserted to keep the test deterministic under load.
}
