package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/sony/gobreaker"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *LLMLoraAdapterReconciler) ensureTargetReady(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter) (*servingv1alpha2.LLMInferenceService, *ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var svc servingv1alpha2.LLMInferenceService
	key := client.ObjectKey{Name: lora.Spec.TargetService, Namespace: lora.Namespace}
	if err := r.Get(ctx, key, &svc); err != nil {
		logger.Error(err, "Target LLMInferenceService not found", "Target", lora.Spec.TargetService)
		return nil, &ctrl.Result{}, nil
	}
	if !svc.Status.ModelReady {
		logger.Info("Target service is not ready yet. Waiting to inject LoRA.")
		return nil, resultAfter(5 * time.Second), nil
	}
	return &svc, nil, nil
}

func (r *LLMLoraAdapterReconciler) registerAndMarkReady(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter, svc *servingv1alpha2.LLMInferenceService) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if err := r.registerWithTargetService(ctx, lora, svc); err != nil {
		logger.Error(err, "Failed to register LoRA with target service pods")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return r.markLoraReady(ctx, lora)
}

func (r *LLMLoraAdapterReconciler) markLoraReady(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter) (ctrl.Result, error) {
	if lora.Status.ActiveRevision != 0 {
		return ctrl.Result{}, nil
	}
	lora.Status.ActiveRevision = 1
	lora.Status.Conditions = append(lora.Status.Conditions, metav1.Condition{
		Type: servingv1alpha2.AdapterConditionReady, Status: metav1.ConditionTrue,
		Reason: "AdapterLoaded", Message: "Adapter successfully hot-swapped into vLLM runtime",
		LastTransitionTime: metav1.Now(),
	})
	if err := r.Status().Update(ctx, lora); err != nil {
		return ctrl.Result{}, err
	}
	log.FromContext(ctx).Info("LoRA Adapter hot-swapped successfully!", "AdapterName", lora.Spec.AdapterName)
	return ctrl.Result{}, nil
}

func (r *LLMLoraAdapterReconciler) registerWithTargetService(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter, svc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)
	podList := &corev1.PodList{}
	labels := map[string]string{"app.kubernetes.io/instance": svc.Name}
	if err := r.List(ctx, podList, client.InNamespace(svc.Namespace), client.MatchingLabels(labels)); err != nil {
		return err
	}
	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods found for target service %s", svc.Name)
	}
	for _, pod := range podList.Items {
		registered, err := r.registerOnPod(ctx, lora, pod)
		if err != nil {
			logger.Error(err, "Failed to load LoRA on pod", "pod", pod.Name)
			r.Recorder.Eventf(lora, corev1.EventTypeWarning, "RegistrationFailed", "Circuit breaker or vLLM error on pod %s: %v", pod.Name, err)
			return err
		}
		if !registered {
			continue
		}
		logger.Info("Successfully sent load_lora_adapter request", "pod", pod.Name)
		r.Recorder.Eventf(lora, corev1.EventTypeNormal, "Registered", "Successfully loaded LoRA adapter on pod %s", pod.Name)
		r.scheduleWarmup(ctx, lora, pod)
	}
	return nil
}

func (r *LLMLoraAdapterReconciler) ensureLoadCircuitBreaker(lora *servingv1alpha2.LLMLoraAdapter) *gobreaker.CircuitBreaker {
	return r.circuitBreaker(lora, loraLoadOperation)
}

func (r *LLMLoraAdapterReconciler) registerOnPod(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter, pod corev1.Pod) (bool, error) {
	if pod.Status.Phase != corev1.PodRunning || pod.Status.PodIP == "" {
		return false, nil
	}
	url := fmt.Sprintf("http://%s:8000/v1/load_lora_adapter", pod.Status.PodIP)
	body, err := json.Marshal(VLLMLoadLoraRequest{LoraName: lora.Spec.AdapterName, LoraPath: fmt.Sprintf("%s/lora-%s", ModelMountPath, lora.Name)})
	if err != nil {
		return false, err
	}
	_, err = r.ensureLoadCircuitBreaker(lora).Execute(func() (interface{}, error) {
		return nil, r.postWithRetry(ctx, url, body)
	})
	return true, err
}

func (r *LLMLoraAdapterReconciler) postWithRetry(ctx context.Context, url string, body []byte) error {
	httpClient := r.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := postJSON(ctx, httpClient, url, body)
		if err == nil {
			var closeErr error
			if resp.Body != nil {
				closeErr = resp.Body.Close()
			}
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
				if closeErr != nil {
					return fmt.Errorf("close vLLM response body: %w", closeErr)
				}
				return nil
			}
			lastErr = fmt.Errorf("vLLM returned non-OK status %d", resp.StatusCode)
			if closeErr != nil {
				lastErr = errors.Join(lastErr, fmt.Errorf("close vLLM response body: %w", closeErr))
			}
		} else {
			lastErr = err
		}
		if attempt < 2 {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return lastErr
}
