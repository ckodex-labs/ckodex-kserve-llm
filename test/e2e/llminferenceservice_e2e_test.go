//go:build e2e

/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ckodexFinalizer is the finalizer the operator registers on LLMInferenceService.
const ckodexFinalizer = "serving.ckodex.com/finalizer"

// sanitiseName converts a test name into a valid Kubernetes resource name.
var sanitiseName = regexp.MustCompile(`[^a-z0-9-]`)

func resourceName(t *testing.T, prefix string) string {
	t.Helper()
	raw := fmt.Sprintf("%s-%s", prefix, t.Name())
	lower := []byte(raw)
	for i := range lower {
		lower[i] = lowerByte(lower[i])
	}
	safe := sanitiseName.ReplaceAllString(string(lower), "-")
	if len(safe) > 60 {
		safe = safe[:60]
	}
	return safe
}

func lowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

// newGPT2Service builds an LLMInferenceService spec mirroring
// config/samples/inference_test_gpt2.yaml, with a custom name.
func newGPT2Service(name string) *servingv1alpha2.LLMInferenceService {
	replicas := int32(1)
	minReplicas := int32(1)
	cpuLimit := "2"
	memLimit := "4Gi"

	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: e2eNamespace,
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://openai-community/gpt2",
				Name: "gpt2",
			},
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "vllm",
                            Image: "vllm/vllm-openai-cpu:v0.25.1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    mustQuantity(cpuLimit),
									corev1.ResourceMemory: mustQuantity(memLimit),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    mustQuantity(cpuLimit),
									corev1.ResourceMemory: mustQuantity(memLimit),
								},
							},
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8000},
							},
						},
					},
				},
			},
			Router: servingv1alpha2.RouterSpec{
				Gateway: servingv1alpha2.GatewaySpec{
					Managed: &servingv1alpha2.ManagedGatewaySpec{
						GatewayClassName: "envoy",
					},
				},
				Route: servingv1alpha2.RouteSpec{
					HTTPRoute: &servingv1alpha2.HTTPRouteSpec{
						Hostnames: []string{name + ".local"},
					},
				},
				Scheduler: servingv1alpha2.SchedulerSpec{
					Pool: servingv1alpha2.InferencePoolSpec{
						Selector: map[string]string{
							"app.kubernetes.io/instance": name,
						},
					},
				},
			},
			Scaling: &servingv1alpha2.ScalingSpec{
				MinReplicas: &minReplicas,
			},
		},
	}
}

// mustQuantity parses a resource quantity string or panics — programming error only.
// FACT: resource.MustParse panics on invalid input; only well-known constants are passed.
func mustQuantity(s string) resource.Quantity {
	return resource.MustParse(s)
}

// TestE2E_LLMInferenceService_BasicLifecycle tests the full create → verify → delete lifecycle.
func TestE2E_LLMInferenceService_BasicLifecycle(t *testing.T) {
	name := resourceName(t, "llmsvc")
	svc := newGPT2Service(name)

	ctx := context.Background()
	require.NoError(t, k8sClient.Create(ctx, svc))
	key := client.ObjectKeyFromObject(svc)

	t.Cleanup(func() {
		fresh := &servingv1alpha2.LLMInferenceService{}
		if err := k8sClient.Get(context.Background(), key, fresh); err == nil {
			_ = k8sClient.Delete(context.Background(), fresh)
		}
	})

	// Step 1: Wait for finalizer to be set.
	t.Log("waiting for finalizer...")
	require.NoError(t,
		waitForCondition(t, key, &servingv1alpha2.LLMInferenceService{}, shortTimeout,
			func(obj client.Object) (bool, error) {
				s := obj.(*servingv1alpha2.LLMInferenceService)
				return slices.Contains(s.Finalizers, ckodexFinalizer), nil
			},
		), "finalizer must be added within %s", shortTimeout,
	)

	// Step 2: Wait for Deployment to exist in the same namespace.
	t.Log("waiting for Deployment...")
	require.NoError(t,
		waitForDeploymentExists(t, name, shortTimeout),
		"Deployment must be created within %s", shortTimeout,
	)

	// Step 3: Wait for Service to exist.
	t.Log("waiting for Service...")
	require.NoError(t,
		waitForServiceExists(t, name, shortTimeout),
		"Service must be created within %s", shortTimeout,
	)

	// Step 4 (slow): Wait for Ready=True and probe inference.
	// Skipped by default because it requires vLLM to pull the model image,
	// download the model from HuggingFace, and load it into memory — easily
	// 10-20 minutes on a cold KIND cluster.
	// Enable with: E2E_FULL_LIFECYCLE=true
	if os.Getenv("E2E_FULL_LIFECYCLE") == "true" {
		t.Log("waiting for Ready=True condition (full lifecycle mode)...")
		require.NoError(t,
			waitForCondition(t, key, &servingv1alpha2.LLMInferenceService{}, 20*time.Minute,
				func(obj client.Object) (bool, error) {
					s := obj.(*servingv1alpha2.LLMInferenceService)
					cond := meta.FindStatusCondition(s.Status.Conditions, servingv1alpha2.ConditionReady)
					return cond != nil && cond.Status == metav1.ConditionTrue, nil
				},
			), "Ready condition must be True within 20m",
		)

		t.Log("probing inference endpoint via port-forward...")
		probeInference(t, name)
	}

	// Step 5: Delete CR and verify finalizer is removed (object gone).
	t.Log("deleting LLMInferenceService...")
	fresh := &servingv1alpha2.LLMInferenceService{}
	require.NoError(t, k8sClient.Get(ctx, key, fresh))
	require.NoError(t, k8sClient.Delete(ctx, fresh))

	t.Log("waiting for CR to disappear...")
	require.NoError(t,
		waitForAbsence(t, key, &servingv1alpha2.LLMInferenceService{}, shortTimeout),
		"CR must be fully deleted within %s", shortTimeout,
	)
}

// TestE2E_LLMInferenceService_StatusConditions verifies the operator sets conditions
// even when the model URI is invalid (unhappy path).
func TestE2E_LLMInferenceService_StatusConditions(t *testing.T) {
	name := resourceName(t, "llmsvc-inv")
	replicas := int32(1)

	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: e2eNamespace,
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				// FACT: valid scheme (passes webhook) but non-existent model so the
				// storage initializer fails and the operator sets Ready=False.
				// Using "invalid://" would be rejected at admission time, not by the operator.
				URI:  "hf://ckodex-test/nonexistent-model-xyz-12345",
				Name: "not-real",
			},
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "vllm",
                            Image: "vllm/vllm-openai-cpu:v0.25.1",
						},
					},
				},
			},
			Router: servingv1alpha2.RouterSpec{
				Gateway: servingv1alpha2.GatewaySpec{
					Managed: &servingv1alpha2.ManagedGatewaySpec{GatewayClassName: "envoy"},
				},
				Route: servingv1alpha2.RouteSpec{
					HTTPRoute: &servingv1alpha2.HTTPRouteSpec{
						Hostnames: []string{name + ".local"},
					},
				},
				Scheduler: servingv1alpha2.SchedulerSpec{
					Pool: servingv1alpha2.InferencePoolSpec{
						Selector: map[string]string{"app.kubernetes.io/instance": name},
					},
				},
			},
		},
	}

	ctx := context.Background()
	require.NoError(t, k8sClient.Create(ctx, svc))
	key := client.ObjectKeyFromObject(svc)

	t.Cleanup(func() {
		fresh := &servingv1alpha2.LLMInferenceService{}
		if err := k8sClient.Get(context.Background(), key, fresh); err == nil {
			_ = k8sClient.Delete(context.Background(), fresh)
		}
	})

	// Wait for at least one condition (any status) — proves operator is reconciling.
	t.Log("waiting for operator to set at least one status condition...")
	require.NoError(t,
		waitForCondition(t, key, &servingv1alpha2.LLMInferenceService{}, time.Minute,
			func(obj client.Object) (bool, error) {
				s := obj.(*servingv1alpha2.LLMInferenceService)
				return len(s.Status.Conditions) > 0, nil
			},
		), "operator must set at least one condition within 1m",
	)

	final := &servingv1alpha2.LLMInferenceService{}
	require.NoError(t, k8sClient.Get(ctx, key, final))
	assert.NotEmpty(t, final.Status.Conditions, "status.conditions must be non-empty")
}

// TestE2E_LLMInferenceService_FinaliserAdded verifies the operator registers its
// finalizer on a newly-created LLMInferenceService.
func TestE2E_LLMInferenceService_FinaliserAdded(t *testing.T) {
	name := resourceName(t, "llmsvc-fin")
	svc := newGPT2Service(name)

	ctx := context.Background()
	require.NoError(t, k8sClient.Create(ctx, svc))
	key := client.ObjectKeyFromObject(svc)

	t.Cleanup(func() {
		fresh := &servingv1alpha2.LLMInferenceService{}
		if err := k8sClient.Get(context.Background(), key, fresh); err == nil {
			_ = k8sClient.Delete(context.Background(), fresh)
		}
	})

	require.NoError(t,
		waitForCondition(t, key, &servingv1alpha2.LLMInferenceService{}, shortTimeout,
			func(obj client.Object) (bool, error) {
				s := obj.(*servingv1alpha2.LLMInferenceService)
				return slices.Contains(s.Finalizers, ckodexFinalizer), nil
			},
		), "finalizer %q must be present within %s", ckodexFinalizer, shortTimeout,
	)

	final := &servingv1alpha2.LLMInferenceService{}
	require.NoError(t, k8sClient.Get(ctx, key, final))
	assert.True(t, slices.Contains(final.Finalizers, ckodexFinalizer),
		"expected finalizer %q in %v", ckodexFinalizer, final.Finalizers)
}

// TestE2E_LLMInferenceService_LiveInference verifies the already-deployed
// inference service (llama3-8b in the ckodex-inference namespace, running Qwen3-0.6B)
// responds to a /v1/chat/completions request with HTTP 200.
//
// This test intentionally uses the pre-existing service rather than creating a
// new one — model image pull + HuggingFace download + vLLM warm-up takes 10-20
// minutes on a cold cluster, which is unsuitable for a default E2E gate.
//
// The service must be pre-deployed:
//
//	kubectl create namespace ckodex-inference --dry-run=client -o yaml | kubectl apply -f -
//	kubectl apply -f config/samples/inference_test_gpt2.yaml
func TestE2E_LLMInferenceService_LiveInference(t *testing.T) {
	const (
		liveSvcName = "llama3-8b"
		liveNS      = "ckodex-inference"
		liveModel   = "qwen-0.5b" // operator assigns name from spec.model.name
	)

	ctx := context.Background()

	// Verify the service exists and is Ready before attempting inference.
	key := client.ObjectKey{Namespace: liveNS, Name: liveSvcName}
	var liveSvc servingv1alpha2.LLMInferenceService
	if err := k8sClient.Get(ctx, key, &liveSvc); err != nil {
		t.Skipf("pre-deployed service %s/%s not found — skipping live inference test: %v", liveNS, liveSvcName, err)
	}

	cond := meta.FindStatusCondition(liveSvc.Status.Conditions, servingv1alpha2.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Skipf("service %s/%s is not Ready (condition=%v) — skipping live inference test", liveNS, liveSvcName, cond)
	}

	// Port-forward to the service and send a minimal chat completion request.
	t.Logf("live service %s/%s is Ready — probing inference", liveNS, liveSvcName)

	payload := map[string]any{
		"model": liveModel,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with the single word: hello"},
		},
		"max_tokens": 10,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	// FACT: in-cluster FQDN reachable from within the pod network.
	// When running from outside the cluster (host machine), use port-forwarding.
	// The exec-based probe in the summary confirmed in-pod connectivity works.
	endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local/v1/chat/completions",
		liveSvcName, liveNS)

	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body)) //nolint:gosec // test-only
	if err != nil {
		t.Skipf("cannot reach in-cluster endpoint %s (not running in-cluster?) — skipping: %v", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	t.Logf("inference response: status=%d body=%s", resp.StatusCode, string(respBody))
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"live inference endpoint must return 200 OK; body: %s", string(respBody))
}

// --- helpers ---

// waitForDeploymentExists polls until a Deployment named `name` exists in e2eNamespace.
func waitForDeploymentExists(t *testing.T, name string, timeout time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	key := client.ObjectKey{Namespace: e2eNamespace, Name: name}
	return wait.PollUntilContextTimeout(ctx, eventuallyPoll, timeout, true,
		func(_ context.Context) (bool, error) {
			dep := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, key, dep); err != nil {
				if errors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			return true, nil
		},
	)
}

// waitForServiceExists polls until a Service named `name` exists in e2eNamespace.
func waitForServiceExists(t *testing.T, name string, timeout time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	key := client.ObjectKey{Namespace: e2eNamespace, Name: name}
	return wait.PollUntilContextTimeout(ctx, eventuallyPoll, timeout, true,
		func(_ context.Context) (bool, error) {
			svc := &corev1.Service{}
			if err := k8sClient.Get(ctx, key, svc); err != nil {
				if errors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			return true, nil
		},
	)
}

// probeInference performs a minimal /v1/chat/completions POST against the service
// via a simple in-cluster address. Skipped when E2E_SKIP_INFERENCE=true.
// ASSUMPTION: the test runner has network access to the e2eNamespace service FQDN.
func probeInference(t *testing.T, svcName string) {
	t.Helper()

	// FACT: in-cluster FQDN format for a Service.
	endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:8000/v1/chat/completions",
		svcName, e2eNamespace)

	payload := map[string]interface{}{
		"model": "gpt2",
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
		"max_tokens": 8,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body)) //nolint:gosec // test-only
	require.NoError(t, err, "POST %s", endpoint)
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"inference endpoint must return 200 OK")
}
