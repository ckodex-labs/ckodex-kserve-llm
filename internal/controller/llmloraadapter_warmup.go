package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (r *LLMLoraAdapterReconciler) scheduleWarmup(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter, pod corev1.Pod) {
	r.warmupMu.Lock()
	warmupKey := fmt.Sprintf("%s/%s/%s", pod.Name, lora.Name, lora.Spec.AdapterName)
	if r.warmupDone[warmupKey] {
		r.warmupMu.Unlock()
		return
	}
	r.warmupMu.Unlock()
	go r.performWarmup(ctx, pod.Status.PodIP, lora.Spec.AdapterName)
	r.warmupMu.Lock()
	if r.warmupDone == nil {
		r.warmupDone = make(map[string]bool)
	}
	r.warmupDone[warmupKey] = true
	r.warmupMu.Unlock()
}

func (r *LLMLoraAdapterReconciler) performWarmup(ctx context.Context, podIP, adapterName string) {
	logger := log.FromContext(ctx)
	url := fmt.Sprintf("http://%s:8000/v1/completions", podIP)
	body, err := json.Marshal(map[string]interface{}{"model": adapterName, "prompt": " ", "max_tokens": 1, "echo": false})
	if err != nil {
		logger.Error(err, "Warmup request encoding failed", "pod", podIP)
		return
	}
	httpClient := r.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := postJSON(ctx, httpClient, url, body)
	if err != nil {
		logger.Error(err, "Warmup request failed", "pod", podIP)
		return
	}
	if resp.Body != nil {
		if err := resp.Body.Close(); err != nil {
			logger.Error(err, "Warmup response body close failed", "pod", podIP, "adapter", adapterName)
			return
		}
	}
	logger.Info("Proactive warmup complete", "pod", podIP, "adapter", adapterName)
}

func (r *LLMLoraAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.setupWithManager(mgr, "llmloraadapter")
}

func (r *LLMLoraAdapterReconciler) setupWithManager(mgr ctrl.Manager, controllerName string) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.LLMLoraAdapter{}).
		Watches(&servingv1alpha2.LocalModelCache{}, handler.EnqueueRequestsFromMapFunc(mapCacheToLora)).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.mapPodToLoras(mgr))).
		Complete(r)
}

func mapCacheToLora(_ context.Context, obj client.Object) []reconcile.Request {
	annotations := obj.GetAnnotations()
	if obj.GetLabels()[loraCacheManagedByLabel] != loraCacheManagedByAdapter {
		return nil
	}
	namespace, name := annotations[loraCacheOwnerNamespace], annotations[loraCacheOwnerName]
	if namespace == "" || name == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}}
}

func (r *LLMLoraAdapterReconciler) mapPodToLoras(mgr ctrl.Manager) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil
		}
		svcName, ok := pod.Labels["app.kubernetes.io/instance"]
		if !ok {
			return nil
		}
		var adapters servingv1alpha2.LLMLoraAdapterList
		if err := mgr.GetClient().List(ctx, &adapters, client.InNamespace(pod.Namespace)); err != nil {
			return nil
		}
		return requestsForTarget(adapters.Items, svcName)
	}
}

func requestsForTarget(adapters []servingv1alpha2.LLMLoraAdapter, serviceName string) []reconcile.Request {
	requests := make([]reconcile.Request, 0)
	for _, adapter := range adapters {
		if adapter.Spec.TargetService == serviceName {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: adapter.Name, Namespace: adapter.Namespace}})
		}
	}
	return requests
}
