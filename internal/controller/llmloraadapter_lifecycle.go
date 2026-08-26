package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/sony/gobreaker"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *LLMLoraAdapterReconciler) finalizeLora(ctx context.Context, lora, original *servingv1alpha2.LLMLoraAdapter) (ctrl.Result, error) {
	if !containsString(lora.Finalizers, loraFinalizer) {
		return ctrl.Result{}, nil
	}
	if err := r.unloadFromTargetService(ctx, lora); err != nil {
		return ctrl.Result{}, fmt.Errorf("unload LoRA adapter: %w", err)
	}
	if err := r.deleteLoraCache(ctx, lora); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete LoRA cache: %w", err)
	}
	lora.Finalizers = removeString(lora.Finalizers, loraFinalizer)
	return ctrl.Result{}, r.Patch(ctx, lora, client.MergeFrom(original))
}

func (r *LLMLoraAdapterReconciler) unloadFromTargetService(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter) error {
	breaker := r.ensureUnloadCircuitBreaker(lora)
	var svc servingv1alpha2.LLMInferenceService
	if err := r.Get(ctx, client.ObjectKey{Name: lora.Spec.TargetService, Namespace: lora.Namespace}, &svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(svc.Namespace), client.MatchingLabels{"app.kubernetes.io/instance": svc.Name}); err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Status.PodIP == "" {
			continue
		}
		url := fmt.Sprintf("http://%s:8000/v1/unload_lora_adapter", pod.Status.PodIP)
		body, err := json.Marshal(map[string]string{"lora_name": lora.Spec.AdapterName})
		if err != nil {
			return err
		}
		_, err = breaker.Execute(func() (interface{}, error) {
			httpClient := r.HTTPClient
			if httpClient == nil {
				httpClient = http.DefaultClient
			}
			resp, err := postJSON(ctx, httpClient, url, body)
			if err != nil {
				return nil, err
			}
			if resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					return nil, fmt.Errorf("close unload response body: %w", err)
				}
			}
			return nil, nil
		})
		if err != nil {
			return fmt.Errorf("unload adapter %q from pod %q: %w", lora.Spec.AdapterName, pod.Name, err)
		}
	}
	return nil
}

func (r *LLMLoraAdapterReconciler) ensureUnloadCircuitBreaker(lora *servingv1alpha2.LLMLoraAdapter) *gobreaker.CircuitBreaker {
	return r.circuitBreaker(lora, loraUnloadOperation)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

func postJSON(ctx context.Context, httpClient *http.Client, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return httpClient.Do(req)
}
