/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package multinode contains the read-only two-node GPU acceptance gate.
package multinode

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const (
	kubeconfigEnv          = "E2E_KUBECONFIG"
	multiNodeEnabledEnv    = "E2E_KSERVE_MULTINODE"
	multiNodeNamespaceEnv  = "E2E_KSERVE_MULTINODE_NAMESPACE"
	multiNodeNameEnv       = "E2E_KSERVE_MULTINODE_NAME"
	multiNodeModelEnv      = "E2E_KSERVE_MULTINODE_MODEL"
	multiNodeEndpointEnv   = "E2E_KSERVE_MULTINODE_ENDPOINT"
	multiNodeAPIKeyEnv     = "E2E_KSERVE_MULTINODE_API_KEY"
	defaultNamespace       = "models"
	defaultName            = "gemma4-multinode"
	readyTimeout           = 30 * time.Minute
	requestTimeout         = 2 * time.Minute
	pollInterval           = 2 * time.Second
	maximumResponseBodyLen = 1 << 20
)

var inferenceServiceGVK = schema.GroupVersionKind{
	Group: "serving.kserve.io", Version: "v1beta1", Kind: "InferenceService",
}

// TestKServeMultiNodeOpenAIRequest is the hardware acceptance gate for issue
// #18. It reads a pre-deployed service and never creates, updates, or deletes a
// Kubernetes object. Once explicitly enabled, missing configuration,
// single-node placement, readiness failures, and inference failures fail.
func TestKServeMultiNodeOpenAIRequest(t *testing.T) {
	if os.Getenv(multiNodeEnabledEnv) != "true" {
		t.Skipf("set %s=true to run the two-node GPU acceptance gate", multiNodeEnabledEnv)
	}

	k8sClient := loadClient(t)
	namespace := envOrDefault(multiNodeNamespaceEnv, defaultNamespace)
	name := envOrDefault(multiNodeNameEnv, defaultName)
	model := requireEnv(t, multiNodeModelEnv)
	endpoint := requireEnv(t, multiNodeEndpointEnv)
	key := client.ObjectKey{Namespace: namespace, Name: name}

	waitForCKodexReady(t, k8sClient, key)
	waitForKServeReady(t, k8sClient, key)
	pods := waitForPodsReady(t, k8sClient, namespace, name)
	assertPodsRequestGPUs(t, pods)
	assertPodsSpanNodes(t, pods, 2)
	probeOpenAI(t, endpoint, model, os.Getenv(multiNodeAPIKeyEnv))
}

func loadClient(t *testing.T) client.Client {
	t.Helper()
	kubeconfig := requireEnv(t, kubeconfigEnv)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("load kubeconfig %s: %v", kubeconfig, err)
	}
	scheme := k8sruntime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("register Kubernetes scheme: %v", err)
	}
	if err := servingv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("register CKodex scheme: %v", err)
	}
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("build Kubernetes client: %v", err)
	}
	return k8sClient
}

func waitForCKodexReady(t *testing.T, k8sClient client.Client, key client.ObjectKey) {
	t.Helper()
	service := &servingv1alpha2.LLMInferenceService{}
	err := waitForObject(t, k8sClient, key, service, func() bool {
		condition := meta.FindStatusCondition(
			service.Status.Conditions,
			servingv1alpha2.ConditionReady,
		)
		return condition != nil && condition.Status == metav1.ConditionTrue
	})
	if err != nil {
		t.Fatalf("CKodex multi-node service %s did not become Ready: %v", key, err)
	}
}

func waitForKServeReady(t *testing.T, k8sClient client.Client, key client.ObjectKey) {
	t.Helper()
	inferenceService := &unstructured.Unstructured{}
	inferenceService.SetGroupVersionKind(inferenceServiceGVK)
	err := waitForObject(t, k8sClient, key, inferenceService, func() bool {
		conditions, _, nestedErr := unstructured.NestedSlice(
			inferenceService.Object,
			"status", "conditions",
		)
		return nestedErr == nil && unstructuredConditionTrue(conditions, "Ready")
	})
	if err != nil {
		t.Fatalf("KServe InferenceService %s did not become Ready: %v", key, err)
	}
	assertKServeSingleHead(t, inferenceService)
}

func assertKServeSingleHead(t *testing.T, inferenceService *unstructured.Unstructured) {
	t.Helper()
	predictor, found, err := unstructured.NestedMap(inferenceService.Object, "spec", "predictor")
	if err != nil || !found {
		t.Fatalf("KServe InferenceService has no predictor: found=%t err=%v", found, err)
	}
	minReplicas, minFound, minErr := unstructured.NestedInt64(predictor, "minReplicas")
	maxReplicas, maxFound, maxErr := unstructured.NestedInt64(predictor, "maxReplicas")
	if minErr != nil || maxErr != nil || !minFound || !maxFound ||
		minReplicas != 1 || maxReplicas != 1 {
		t.Fatalf(
			"KServe head replica contract is min=%d max=%d, want exactly one; errors=%v/%v",
			minReplicas, maxReplicas, minErr, maxErr,
		)
	}
	annotations := inferenceService.GetAnnotations()
	if annotations["serving.kserve.io/deploymentMode"] != "Standard" ||
		annotations["serving.kserve.io/autoscalerClass"] != "none" {
		t.Fatalf("KServe mode/autoscaler annotations are invalid: %v", annotations)
	}
}

func waitForObject(
	t *testing.T,
	k8sClient client.Client,
	key client.ObjectKey,
	object client.Object,
	ready func() bool,
) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), readyTimeout)
	defer cancel()
	return poll(ctx, func() (bool, error) {
		if err := k8sClient.Get(ctx, key, object); err != nil {
			return false, client.IgnoreNotFound(err)
		}
		return ready(), nil
	})
}

func waitForPodsReady(
	t *testing.T,
	k8sClient client.Client,
	namespace, name string,
) []corev1.Pod {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), readyTimeout)
	defer cancel()

	var readyPods []corev1.Pod
	err := poll(ctx, func() (bool, error) {
		pods := &corev1.PodList{}
		if err := k8sClient.List(
			ctx,
			pods,
			client.InNamespace(namespace),
			client.MatchingLabels{"serving.kserve.io/inferenceservice": name},
		); err != nil {
			return false, err
		}
		readyPods = readyPods[:0]
		for i := range pods.Items {
			if pods.Items[i].Spec.NodeName != "" && podReady(&pods.Items[i]) {
				readyPods = append(readyPods, pods.Items[i])
			}
		}
		return len(readyPods) >= 2, nil
	})
	if err != nil {
		t.Fatalf(
			"fewer than two Ready KServe pods for %s/%s after %s: %v",
			namespace, name, readyTimeout, err,
		)
	}
	return readyPods
}

func poll(ctx context.Context, condition func() (bool, error)) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		ready, err := condition()
		if err != nil || ready {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func assertPodsSpanNodes(t *testing.T, pods []corev1.Pod, minimumNodes int) {
	t.Helper()
	nodes := make(map[string][]string)
	for i := range pods {
		nodes[pods[i].Spec.NodeName] = append(nodes[pods[i].Spec.NodeName], pods[i].Name)
	}
	if len(nodes) < minimumNodes {
		t.Fatalf(
			"Ready KServe pods occupy %d node(s), want at least %d: %v",
			len(nodes), minimumNodes, nodes,
		)
	}
	t.Logf("Ready KServe pods span %d nodes: %v", len(nodes), nodes)
}

func assertPodsRequestGPUs(t *testing.T, pods []corev1.Pod) {
	t.Helper()
	for i := range pods {
		hasRequest, hasLimit := false, false
		for _, container := range pods[i].Spec.Containers {
			request, requestFound := container.Resources.Requests[corev1.ResourceName("nvidia.com/gpu")]
			limit, limitFound := container.Resources.Limits[corev1.ResourceName("nvidia.com/gpu")]
			hasRequest = hasRequest || requestFound && !request.IsZero()
			hasLimit = hasLimit || limitFound && !limit.IsZero()
		}
		if !hasRequest || !hasLimit {
			t.Fatalf(
				"Ready KServe pod %s has no positive nvidia.com/gpu request and limit",
				pods[i].Name,
			)
		}
	}
}

func probeOpenAI(t *testing.T, endpoint, model, apiKey string) {
	t.Helper()
	request, err := newOpenAIRequest(endpoint, model, apiKey)
	if err != nil {
		t.Fatalf("build OpenAI request: %v", err)
	}

	response, err := (&http.Client{Timeout: requestTimeout}).Do(request)
	if err != nil {
		t.Fatalf("POST multi-node OpenAI endpoint %s: %v", endpoint, err)
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBodyLen))
	if err != nil {
		t.Fatalf("read OpenAI response: %v", err)
	}
	validateOpenAIResponse(t, response, responseBody)
}

func newOpenAIRequest(endpoint, model, apiKey string) (*http.Request, error) {
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with the single word: hello"},
		},
		"max_tokens": 8,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return request, nil
}

func validateOpenAIResponse(t *testing.T, response *http.Response, responseBody []byte) {
	t.Helper()
	if response.StatusCode != http.StatusOK {
		t.Fatalf(
			"OpenAI endpoint returned %s: %s",
			response.Status,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var completion struct {
		Choices []json.RawMessage `json:"choices"`
		Error   json.RawMessage   `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		t.Fatalf("decode OpenAI response: %v; body=%s", err, responseBody)
	}
	if len(completion.Error) != 0 && string(completion.Error) != "null" {
		t.Fatalf("OpenAI response contains an error: %s", completion.Error)
	}
	if len(completion.Choices) == 0 {
		t.Fatalf("OpenAI response has no choices: %s", responseBody)
	}
}

func unstructuredConditionTrue(conditions []interface{}, conditionType string) bool {
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if condition["type"] == conditionType && condition["status"] == string(metav1.ConditionTrue) {
			return true
		}
	}
	return false
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func requireEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s must be set when %s=true", name, multiNodeEnabledEnv)
	}
	return value
}
