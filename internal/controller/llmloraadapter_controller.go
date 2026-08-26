/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/sony/gobreaker"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VLLMLoadLoraRequest struct {
	LoraName string `json:"lora_name"`
	LoraPath string `json:"lora_path"`
}

type LLMLoraAdapterReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	HTTPClient *http.Client
	Recorder   record.EventRecorder
	Audit      *observability.AuditLogger

	warmupMu          sync.Mutex
	warmupDone        map[string]bool
	circuitBreakersMu sync.Mutex
	circuitBreakers   map[loraCircuitBreakerKey]*gobreaker.CircuitBreaker
}

type loraCircuitBreakerOperation string

const (
	loraLoadOperation   loraCircuitBreakerOperation = "load"
	loraUnloadOperation loraCircuitBreakerOperation = "unload"
)

type loraCircuitBreakerKey struct {
	operation     loraCircuitBreakerOperation
	namespace     string
	targetService string
	adapter       string
}

func (r *LLMLoraAdapterReconciler) circuitBreaker(
	lora *servingv1alpha2.LLMLoraAdapter,
	operation loraCircuitBreakerOperation,
) *gobreaker.CircuitBreaker {
	key := loraCircuitBreakerKey{
		operation:     operation,
		namespace:     lora.Namespace,
		targetService: lora.Spec.TargetService,
		adapter:       lora.Name,
	}
	r.circuitBreakersMu.Lock()
	defer r.circuitBreakersMu.Unlock()
	if r.circuitBreakers == nil {
		r.circuitBreakers = make(map[loraCircuitBreakerKey]*gobreaker.CircuitBreaker)
	}
	if breaker, ok := r.circuitBreakers[key]; ok {
		return breaker
	}
	breaker := NewDefaultCircuitBreaker(CircuitBreakerSettings{
		Name: fmt.Sprintf("vllm-adapter-%s-%s-%s-%s", operation, lora.Namespace, lora.Spec.TargetService, lora.Name),
	}, r.Recorder, lora)
	r.circuitBreakers[key] = breaker
	return breaker
}

const (
	loraFinalizer             = "serving.ckodex.com/lora-finalizer"
	loraCacheManagedByLabel   = "serving.ckodex.com/managed-by"
	loraCacheOwnerNamespace   = "serving.ckodex.com/owner-namespace"
	loraCacheOwnerName        = "serving.ckodex.com/owner-name"
	loraCacheOwnerUID         = "serving.ckodex.com/owner-uid"
	loraCacheManagedByAdapter = "llmloraadapter"
)

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=localmodelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

func (r *LLMLoraAdapterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lora, original, result, err := r.fetchLora(ctx, req)
	if err != nil || lora == nil {
		return result, err
	}
	if lora.DeletionTimestamp != nil {
		return r.finalizeLora(ctx, lora, original)
	}
	if result, err := r.prepareLora(ctx, lora, original); err != nil || result != nil {
		return resultValue(result), err
	}
	cache, cacheResult, cacheErr := r.ensureLoraCache(ctx, lora)
	if cacheErr != nil || cacheResult != nil {
		return resultValue(cacheResult), cacheErr
	}
	if result, err := r.hydrateAndGovernLora(ctx, lora, cache); err != nil || result != nil {
		return resultValue(result), err
	}
	svc, targetResult, targetErr := r.ensureTargetReady(ctx, lora)
	if targetErr != nil || targetResult != nil {
		return resultValue(targetResult), targetErr
	}
	return r.registerAndMarkReady(ctx, lora, svc)
}

func resultValue(result *ctrl.Result) ctrl.Result {
	if result == nil {
		return ctrl.Result{}
	}
	return *result
}
