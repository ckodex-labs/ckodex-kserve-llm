/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package validation

import (
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// SurfaceDisposition records how an accepted API field reaches an executable
// path. Refused fields are guarded by ValidateLLMInferenceServiceSurface.
type SurfaceDisposition string

const (
	SurfaceRendered SurfaceDisposition = "rendered"
	SurfaceObserved SurfaceDisposition = "observed"
	SurfaceRefused  SurfaceDisposition = "refused"
)

// SurfaceContract is the checked-in inventory for the primary
// LLMInferenceService spec, including nested operator-owned fields. Kubernetes
// pod templates and connector references are recorded as aggregate pass-through
// fields because their schemas belong to external APIs.
type SurfaceContract struct {
	Version     string
	Path        string
	Disposition SurfaceDisposition
	Owner       string
}

var llmInferenceServiceSurface = []SurfaceContract{
	{Version: "v1alpha2", Path: "model", Disposition: SurfaceObserved, Owner: "deployment storage and model identity"},
	{Version: "v1alpha2", Path: "replicas", Disposition: SurfaceObserved, Owner: "deployment replica reconciliation"},
	{Version: "v1alpha2", Path: "parallelism", Disposition: SurfaceRendered, Owner: "registered runtime adapter and KServe worker path"},
	{Version: "v1alpha2", Path: "scaling", Disposition: SurfaceObserved, Owner: "autoscaler reconciliation"},
	{Version: "v1alpha2", Path: "template", Disposition: SurfaceObserved, Owner: "deployment pod template"},
	{Version: "v1alpha2", Path: "prefill", Disposition: SurfaceObserved, Owner: "prefill deployment"},
	{Version: "v1alpha2", Path: "worker", Disposition: SurfaceObserved, Owner: "KServe worker overrides"},
	{Version: "v1alpha2", Path: "router", Disposition: SurfaceObserved, Owner: "gateway and scheduler reconciliation"},
	{Version: "v1alpha2", Path: "baseRefs", Disposition: SurfaceRefused, Owner: "ValidateLLMInferenceServiceSurface"},
	{Version: "v1alpha2", Path: "autoOptimize", Disposition: SurfaceRefused, Owner: "ValidateLLMInferenceServiceSurface"},
	{Version: "v1alpha2", Path: "allowedTenants", Disposition: SurfaceRefused, Owner: "ValidateLLMInferenceServiceSurface"},
	{Version: "v1alpha2", Path: "costAllocationTags", Disposition: SurfaceObserved, Owner: "deployment labels and telemetry attributes"},
	{Version: "v1alpha2", Path: "slo", Disposition: SurfaceRefused, Owner: "ValidateLLMInferenceServiceSurface"},
	{Version: "v1alpha2", Path: "canary", Disposition: SurfaceObserved, Owner: "HTTPRoute traffic split"},
	{Version: "v1alpha2", Path: "speculativeDecoding", Disposition: SurfaceRendered, Owner: "registered runtime adapter"},
	{Version: "v1alpha2", Path: "kvCache", Disposition: SurfaceRendered, Owner: "vllm runtime and KV transfer configuration"},
	{Version: "v1alpha2", Path: "quantization", Disposition: SurfaceRendered, Owner: "vllm engine selection"},
	{Version: "v1alpha2", Path: "engine", Disposition: SurfaceObserved, Owner: "engine selection and admission"},
	{Version: "v1alpha2", Path: "toolSurface", Disposition: SurfaceObserved, Owner: "network policy and tool isolation"},
	{Version: "v1alpha2", Path: "observability", Disposition: SurfaceObserved, Owner: "Vector configuration reconciliation"},

	{Version: "v1", Path: "model", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "replicas", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "parallelism", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "scaling", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "template", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "router", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "baseRefs", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "autoOptimize", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "allowedTenants", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "costAllocationTags", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "slo", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "canary", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
	{Version: "v1", Path: "experimental", Disposition: SurfaceObserved, Owner: "v1 to v1alpha2 conversion"},
}

// LLMInferenceServiceSurfaceContracts returns a copy of the governed field
// inventory so callers cannot mutate the package-level contract.
func LLMInferenceServiceSurfaceContracts() []SurfaceContract {
	contracts := make([]SurfaceContract, 0, len(llmInferenceServiceSurface)+len(nestedSurfaceContracts()))
	contracts = append(contracts, llmInferenceServiceSurface...)
	return append(contracts, nestedSurfaceContracts()...)
}

func nestedSurfaceContracts() []SurfaceContract {
	contracts := make([]SurfaceContract, 0, 100)
	add := func(version string, disposition SurfaceDisposition, owner string, paths ...string) {
		for _, path := range paths {
			contracts = append(contracts, SurfaceContract{Version: version, Path: path, Disposition: disposition, Owner: owner})
		}
	}
	add("v1alpha2", SurfaceObserved, "model and storage conversion", "model.uri", "model.revision", "model.name", "model.storage.secretRef", "model.storage.serviceAccountName", "model.storage.storageContainerRef", "model.storage.vaultRef", "model.storage.vaultAddr", "model.storage.externalSecret.secretStoreRef.name", "model.storage.externalSecret.secretStoreRef.kind", "model.storage.externalSecret.refreshInterval", "model.storage.externalSecret.data.secretKey", "model.storage.externalSecret.data.remoteRef.key", "model.storage.externalSecret.data.remoteRef.property", "model.hardwareAware")
	add("v1alpha2", SurfaceRendered, "vllm runtime adapter", "parallelism.tensor", "parallelism.data", "parallelism.dataLocal", "parallelism.expert", "parallelism.gpuDevices", "parallelism.pipeline", "parallelism.eplbEnabled", "speculativeDecoding.method", "speculativeDecoding.numTokens", "speculativeDecoding.draftModel", "kvCache.dtype", "kvCache.swapSpaceGB", "kvCache.transfer.connector", "kvCache.transfer.role", "kvCache.transfer.extraConfig", "kvCache.transfer.lmcache.mode", "kvCache.transfer.lmcache.chunkSize", "kvCache.transfer.lmcache.localCPU", "kvCache.transfer.lmcache.localCPUSizeGiB", "quantization.method")
	add("v1alpha2", SurfaceRefused, "ValidateLLMInferenceServiceSurface", "quantization.checkpointPath")
	add("v1alpha2", SurfaceObserved, "owner-level controller reconciliation", "scaling.minReplicas", "scaling.maxReplicas", "scaling.wva.variantCost", "scaling.keda.pollingInterval", "scaling.keda.cooldownPeriod", "scaling.keda.initialCooldownPeriod", "scaling.keda.idleReplicaCount", "scaling.keda.fallback.failureThreshold", "scaling.keda.fallback.replicas", "scaling.hpa.targetCPUUtilizationPercentage", "prefill.replicas", "prefill.template", "worker.template", "router.gateway.managed.gatewayClassName", "router.gateway.existingRef.name", "router.gateway.existingRef.namespace", "router.route.httpRoute.hostnames", "router.route.httpRoute.resilience.timeout", "router.route.httpRoute.resilience.maxRetries", "router.route.httpRoute.resilience.retryOn", "router.scheduler.pool.selector", "router.scheduler.config.inline", "router.scheduler.config.ref.name", "router.scheduler.config.ref.key", "router.scheduler.replicas", "canary.weight", "canary.baseModel", "kvCache.transfer.env", "kvCache.transfer.lmcache.engineRef", "toolSurface.allowedApis", "toolSurface.allowedCidrs", "observability.sink.type", "observability.sink.endpoint")
	add("v1", SurfaceObserved, "v1 to v1alpha2 conversion", "model.uri", "model.revision", "model.name", "model.storage.secretRef", "model.storage.serviceAccountName", "model.storage.storageContainerRef", "model.storage.vaultRef", "model.storage.vaultAddr", "model.storage.externalSecret.secretStoreRef.name", "model.storage.externalSecret.secretStoreRef.kind", "model.storage.externalSecret.refreshInterval", "model.storage.externalSecret.data.secretKey", "model.storage.externalSecret.data.remoteRef.key", "model.storage.externalSecret.data.remoteRef.property", "model.hardwareAware", "parallelism.tensor", "parallelism.data", "parallelism.dataLocal", "parallelism.expert", "parallelism.gpuDevices", "parallelism.pipeline", "parallelism.eplbEnabled", "scaling.minReplicas", "scaling.maxReplicas", "scaling.wva.variantCost", "scaling.keda.pollingInterval", "scaling.keda.cooldownPeriod", "scaling.keda.initialCooldownPeriod", "scaling.keda.idleReplicaCount", "scaling.keda.fallback.failureThreshold", "scaling.keda.fallback.replicas", "scaling.hpa.targetCPUUtilizationPercentage", "baseRefs.name", "slo.targetP99LatencyMs", "slo.targetAvailability", "slo.errorBudgetDays", "canary.weight", "canary.baseModel", "experimental.prefill.template", "experimental.worker.template", "experimental.kvCache.transfer.env", "experimental.kvCache.transfer.lmcache.engineRef")
	add("v1", SurfaceRefused, "convertStableToAlpha", "router.scheduler.config.inline")
	add("v1", SurfaceObserved, "v1 to v1alpha2 conversion", "router.gateway.managed.gatewayClassName", "router.gateway.existingRef.name", "router.gateway.existingRef.namespace", "router.route.httpRoute.hostnames", "router.route.httpRoute.resilience.timeout", "router.route.httpRoute.resilience.maxRetries", "router.route.httpRoute.resilience.retryOn", "router.scheduler.pool.selector", "router.scheduler.config.ref.name", "router.scheduler.config.ref.key", "router.scheduler.replicas")
	add("v1", SurfaceObserved, "v1 to v1alpha2 conversion", "experimental.prefill.replicas", "experimental.kvCache.dtype", "experimental.kvCache.swapSpaceGB", "experimental.kvCache.transfer.connector", "experimental.kvCache.transfer.role", "experimental.kvCache.transfer.extraConfig", "experimental.kvCache.transfer.lmcache.mode", "experimental.kvCache.transfer.lmcache.chunkSize", "experimental.kvCache.transfer.lmcache.localCPU", "experimental.kvCache.transfer.lmcache.localCPUSizeGiB", "experimental.speculativeDecoding.method", "experimental.speculativeDecoding.numTokens", "experimental.speculativeDecoding.draftModel", "experimental.quantization.method", "experimental.engine", "experimental.toolSurface.allowedApis", "experimental.toolSurface.allowedCidrs", "experimental.observability.sink.type", "experimental.observability.sink.endpoint")
	add("v1", SurfaceRefused, "ValidateLLMInferenceServiceSurface", "experimental.quantization.checkpointPath")
	return contracts
}

// ValidateLLMInferenceServiceSurface refuses accepted fields whose runtime
// behavior is not implemented. Empty optional values retain their default
// semantics; a non-empty value must either execute or be rejected.
func ValidateLLMInferenceServiceSurface(service *servingv1alpha2.LLMInferenceService) field.ErrorList {
	if service == nil {
		return field.ErrorList{field.Required(field.NewPath("spec"), "service is required")}
	}

	errs := field.ErrorList{}
	if len(service.Spec.BaseRefs) > 0 {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "baseRefs"), "configuration references are not resolved by the runtime"))
	}
	if service.Spec.AutoOptimize != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "autoOptimize"), "automatic optimization is not implemented on the active reconciliation path"))
	}
	if len(service.Spec.AllowedTenants) > 0 {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "allowedTenants"), "tenant allow-list enforcement is not wired to this service"))
	}
	if service.Spec.SLO != nil {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "slo"), "service-specific SLO targets are not rendered or evaluated"))
	}
	if service.Spec.Quantization != nil && service.Spec.Quantization.CheckpointPath != "" {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "quantization", "checkpointPath"), "checkpoint paths are not consumed by the active runtime"))
	}
	return errs
}
