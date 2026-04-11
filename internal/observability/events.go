/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// OTel span event name constants.
// Span events are named points in time within a span — they appear in Jaeger /
// Grafana Tempo as timeline markers and can be queried with TraceQL.
// Convention: "ckodex.<domain>.<event>"
const (
	// Reconcile lifecycle events.
	EventReconcileStart    = "ckodex.reconcile.start"
	EventReconcileComplete = "ckodex.reconcile.complete"
	EventDeploymentApplied = "ckodex.deployment.applied"
	EventServiceApplied    = "ckodex.service.applied"

	// Model lifecycle events.
	EventModelDownloadStart = "ckodex.model.download_start"
	EventModelDownloadDone  = "ckodex.model.download_done"
	EventModelLoaded        = "ckodex.model.loaded"
	EventModelUnloaded      = "ckodex.model.unloaded"
	EventModelPromoted      = "ckodex.model.promoted"

	// Inference events.
	EventInferenceQueued      = "ckodex.inference.queued"
	EventInferencePrefillDone = "ckodex.inference.prefill_done"
	EventInferenceFirstToken  = "ckodex.inference.first_token"
	EventInferenceComplete    = "ckodex.inference.complete"
	EventKVCacheHit           = "ckodex.kvcache.hit"
	EventKVCacheMiss          = "ckodex.kvcache.miss"
	EventKVCacheTransferDone  = "ckodex.kvcache.transfer_done"

	// Scaling events.
	EventScaleOut      = "ckodex.scale.out"
	EventScaleIn       = "ckodex.scale.in"
	EventScaleToZero   = "ckodex.scale.to_zero"
	EventScaleFromZero = "ckodex.scale.from_zero"

	// LoRA adapter events.
	EventLoRASwapStart = "ckodex.lora.swap_start"
	EventLoRASwapDone  = "ckodex.lora.swap_done"

	// Auth / security events.
	EventAuthVerified       = "ckodex.auth.verified"
	EventAuthDenied         = "ckodex.auth.denied"
	EventBudgetExceeded     = "ckodex.budget.exceeded"
	EventPolicyViolation    = "ckodex.policy.violation"
	EventVaultSecretFetched = "ckodex.vault.secret_fetched"
)

// AddReconcileStartEvent adds the reconcile start span event with resource identity.
func AddReconcileStartEvent(span trace.Span, name, namespace, resourceVersion string) {
	span.AddEvent(EventReconcileStart, trace.WithAttributes(
		attribute.String("resource.name", name),
		attribute.String("resource.namespace", namespace),
		attribute.String("resource.version", resourceVersion),
	))
}

// AddReconcileCompleteEvent adds the reconcile complete event with outcome.
func AddReconcileCompleteEvent(span trace.Span, name string, requeued bool) {
	span.AddEvent(EventReconcileComplete, trace.WithAttributes(
		attribute.String("resource.name", name),
		attribute.Bool("requeued", requeued),
	))
}

// AddInferenceFirstTokenEvent marks the time-to-first-token point on the span.
func AddInferenceFirstTokenEvent(span trace.Span, ttftMs int64, tenantID, modelName string) {
	span.AddEvent(EventInferenceFirstToken, trace.WithAttributes(
		attribute.Int64(AttrPerfFirstTokenMS, ttftMs),
		attribute.String(AttrActorID, tenantID),
		attribute.String(AttrModelBaseID, modelName),
	))
}

// AddInferenceCompleteEvent marks completion of an inference request.
func AddInferenceCompleteEvent(span trace.Span, promptTokens, completionTokens int64, latencyMs int64) {
	span.AddEvent(EventInferenceComplete, trace.WithAttributes(
		attribute.Int64(AttrCostTokensInput, promptTokens),
		attribute.Int64(AttrCostTokensOutput, completionTokens),
		attribute.Int64(AttrCostTokensTotal, promptTokens+completionTokens),
		attribute.Int64(AttrPerfLatencyMS, latencyMs),
	))
}

// AddScaleEvent records a scaling decision on the active span.
func AddScaleEvent(span trace.Span, eventName string, from, to int32, reason string) {
	span.AddEvent(eventName, trace.WithAttributes(
		attribute.Int("replicas.from", int(from)),
		attribute.Int("replicas.to", int(to)),
		attribute.String("reason", reason),
	))
}

// AddLoRASwapEvent records a LoRA hot-swap event.
func AddLoRASwapEvent(span trace.Span, done bool, fromAdapter, toAdapter string) {
	name := EventLoRASwapStart
	if done {
		name = EventLoRASwapDone
	}
	span.AddEvent(name, trace.WithAttributes(
		attribute.String("adapter.from", fromAdapter),
		attribute.String("adapter.to", toAdapter),
	))
}

// AddKVCacheEvent records a KV-cache hit or miss.
func AddKVCacheEvent(span trace.Span, hit bool, sessionID string) {
	name := EventKVCacheMiss
	if hit {
		name = EventKVCacheHit
	}
	span.AddEvent(name, trace.WithAttributes(
		attribute.String("session.id", sessionID),
	))
}

// AddPolicyViolationEvent records an OPA policy violation on the span.
func AddPolicyViolationEvent(span trace.Span, policy, violation, tenantID string) {
	span.AddEvent(EventPolicyViolation, trace.WithAttributes(
		attribute.String("policy", policy),
		attribute.String("violation", violation),
		attribute.String("ckodex.tenant_id", tenantID),
	))
}
