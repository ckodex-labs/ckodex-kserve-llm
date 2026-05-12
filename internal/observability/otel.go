/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package observability implements OTel, structured logging, and eBPF hooks.
package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// TupleTypeAttr returns the standard metric attribute for a vector state
// forbidden-tuple type. Valid values: anti_execute, active_untrusted,
// negative_escalation_skipped, empty_high_dal.
func TupleTypeAttr(tupleType string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("tuple_type", tupleType))
}

// SetMeterProvider swaps the global OTel MeterProvider and returns the
// previous one. Used in tests to inject an in-process SDK provider.
func SetMeterProvider(mp metric.MeterProvider) metric.MeterProvider {
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	return prev
}

const (
	// InstrumentationName is the OTel instrumentation scope name.
	InstrumentationName = "ckodex.com/kserve-llm-operator"
)

// Instrumentation holds OTel instruments for the operator.
type Instrumentation struct {
	Tracer            trace.Tracer
	Meter             metric.Meter
	ReconcileDuration metric.Float64Histogram
	ReconcileCount    metric.Int64Counter
	ActiveModels      metric.Int64UpDownCounter
	InferenceRequests metric.Int64Counter
	TokensPerSecond   metric.Float64Histogram
	QueueDepth        metric.Int64Gauge

	// Chargeback metrics — scoped by tenant_id + model_name for FinOps billing.
	// ActiveGPUSeconds is the denominator for per-tenant GPU cost allocation.
	// TokensConsumed is the numerator for per-tenant token billing.
	// GPUUtilization is recorded as a histogram so P50/P95/P99 are available.
	GPUUtilization   metric.Float64Histogram
	TokensConsumed   metric.Int64Counter
	ActiveGPUSeconds metric.Float64Counter

	// ForbiddenTupleCounter counts vector state forbidden-tuple violations.
	// Attributes: tuple_type (anti_execute|active_untrusted|negative_escalation_skipped|empty_high_dal)
	ForbiddenTupleCounter metric.Int64Counter
}

// NewInstrumentation creates OTel instruments for the operator.
func NewInstrumentation() (*Instrumentation, error) {
	tracer := otel.Tracer(InstrumentationName)
	meter := otel.Meter(InstrumentationName)

	reconcileDuration, err := meter.Float64Histogram("ckodex.reconcile.duration",
		metric.WithDescription("Reconcile loop duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	reconcileCount, err := meter.Int64Counter("ckodex.reconcile.count",
		metric.WithDescription("Total reconcile loop executions"),
	)
	if err != nil {
		return nil, err
	}

	activeModels, err := meter.Int64UpDownCounter("ckodex.models.active",
		metric.WithDescription("Number of active models being served"),
	)
	if err != nil {
		return nil, err
	}

	inferenceRequests, err := meter.Int64Counter("ckodex.inference.requests",
		metric.WithDescription("Total inference requests"),
	)
	if err != nil {
		return nil, err
	}

	tokensPerSecond, err := meter.Float64Histogram("ckodex.inference.tokens_per_second",
		metric.WithDescription("Token generation throughput"),
		metric.WithUnit("tokens/s"),
	)
	if err != nil {
		return nil, err
	}

	queueDepth, err := meter.Int64Gauge("ckodex.scheduler.queue_depth",
		metric.WithDescription("Pending requests in scheduler queue"),
	)
	if err != nil {
		return nil, err
	}

	gpuUtilization, err := meter.Float64Histogram("ckodex.tenant.gpu_utilization",
		metric.WithDescription("GPU utilization fraction per tenant per model"),
		metric.WithUnit("1"),
		metric.WithExplicitBucketBoundaries(0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0),
	)
	if err != nil {
		return nil, err
	}

	tokensConsumed, err := meter.Int64Counter("ckodex.tenant.tokens_consumed",
		metric.WithDescription("Total tokens consumed per tenant per model (chargeback numerator)"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	activeGPUSeconds, err := meter.Float64Counter("ckodex.tenant.active_gpu_seconds",
		metric.WithDescription("GPU-seconds actively used per tenant per model (chargeback denominator)"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	forbiddenTupleCounter, err := meter.Int64Counter(
		"ckodex.vector.forbidden_tuple",
		metric.WithDescription("Count of vector state forbidden-tuple violations detected"),
		metric.WithUnit("{violation}"),
	)
	if err != nil {
		return nil, err
	}

	return &Instrumentation{
		Tracer:                tracer,
		Meter:                 meter,
		ReconcileDuration:     reconcileDuration,
		ReconcileCount:        reconcileCount,
		ActiveModels:          activeModels,
		InferenceRequests:     inferenceRequests,
		TokensPerSecond:       tokensPerSecond,
		QueueDepth:            queueDepth,
		GPUUtilization:        gpuUtilization,
		TokensConsumed:        tokensConsumed,
		ActiveGPUSeconds:      activeGPUSeconds,
		ForbiddenTupleCounter: forbiddenTupleCounter,
	}, nil
}

// RecordReconcile records a reconcile event.
func (i *Instrumentation) RecordReconcile(ctx context.Context, model string, durationSec float64, success bool) {
	attrs := []attribute.KeyValue{
		attribute.String("model", model),
		attribute.Bool("success", success),
	}
	i.ReconcileDuration.Record(ctx, durationSec, metric.WithAttributes(attrs...))
	i.ReconcileCount.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordTokensConsumed increments the per-tenant token chargeback counter.
// costTags are propagated from spec.costAllocationTags and added as metric attributes
// so billing dashboards can group by cost-center, project, or team.
func (i *Instrumentation) RecordTokensConsumed(ctx context.Context, tenantID, modelName string, prompt, completion int64, costTags map[string]string) {
	attrs := tenantModelAttrs(tenantID, modelName)
	attrs = appendCostTagAttrs(attrs, costTags)

	// OIS v0.1: Economic Semantics
	attrs = append(attrs, attribute.Int64(AttrCostTokensInput, prompt))
	attrs = append(attrs, attribute.Int64(AttrCostTokensOutput, completion))
	attrs = append(attrs, attribute.Int64(AttrCostTokensTotal, prompt+completion))

	i.TokensConsumed.Add(ctx, prompt+completion, metric.WithAttributes(attrs...))
}

// RecordGPUUtilization records a GPU utilization sample for a tenant's model replica.
// utilization is in [0,1]. Call this periodically (e.g., every 15s) from a metrics scrape loop.
func (i *Instrumentation) RecordGPUUtilization(ctx context.Context, tenantID, modelName string, utilization float64, costTags map[string]string) {
	attrs := tenantModelAttrs(tenantID, modelName)
	attrs = appendCostTagAttrs(attrs, costTags)
	i.GPUUtilization.Record(ctx, utilization, metric.WithAttributes(attrs...))
}

// RecordActiveGPUSeconds adds GPU-seconds to the chargeback denominator.
// Call this at the end of a billing window (or periodically) with the elapsed seconds.
func (i *Instrumentation) RecordActiveGPUSeconds(ctx context.Context, tenantID, modelName string, seconds float64, costTags map[string]string) {
	attrs := tenantModelAttrs(tenantID, modelName)
	attrs = appendCostTagAttrs(attrs, costTags)
	i.ActiveGPUSeconds.Add(ctx, seconds, metric.WithAttributes(attrs...))
}

// tenantModelAttrs returns the base attribute set for all chargeback metrics.
func tenantModelAttrs(tenantID, modelName string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("ckodex.tenant_id", tenantID),
		attribute.String("ckodex.model", modelName),
		// OIS mapping
		attribute.String(AttrActorID, tenantID),
		attribute.String(AttrActorURN, URN("actor", tenantID)),
		attribute.String(AttrActorType, "service"),
		attribute.String(AttrModelBaseID, modelName),
		attribute.String(AttrModelBaseURN, URN("model", modelName)),
		attribute.String(AttrEngineRuntime, "vllm"), // Default runtime
	}
}

// appendCostTagAttrs adds spec.costAllocationTags as metric attributes with the
// "ckodex.cost." prefix so they are queryable in Prometheus/Grafana dashboards.
func appendCostTagAttrs(attrs []attribute.KeyValue, tags map[string]string) []attribute.KeyValue {
	for k, v := range tags {
		attrs = append(attrs, attribute.String("ckodex.cost."+k, v))
	}
	return attrs
}
