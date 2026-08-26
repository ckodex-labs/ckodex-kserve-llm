/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
)

func convertBaseRefsToV1(src []ConfigReference) []servingv1.ConfigReference {
	if src == nil {
		return nil
	}
	dst := make([]servingv1.ConfigReference, len(src))
	for i, r := range src {
		dst[i] = servingv1.ConfigReference{Name: r.Name}
	}
	return dst
}

func convertBaseRefsFromV1(src []servingv1.ConfigReference) []ConfigReference {
	if src == nil {
		return nil
	}
	dst := make([]ConfigReference, len(src))
	for i, r := range src {
		dst[i] = ConfigReference{Name: r.Name}
	}
	return dst
}

func convertSLOToV1(src *SLOSpec) *servingv1.SLOSpec {
	if src == nil {
		return nil
	}
	return &servingv1.SLOSpec{
		TargetP99LatencyMs: src.TargetP99LatencyMs,
		TargetAvailability: src.TargetAvailability,
		ErrorBudgetDays:    src.ErrorBudgetDays,
	}
}

func convertSLOFromV1(src *servingv1.SLOSpec) *SLOSpec {
	if src == nil {
		return nil
	}
	return &SLOSpec{
		TargetP99LatencyMs: src.TargetP99LatencyMs,
		TargetAvailability: src.TargetAvailability,
		ErrorBudgetDays:    src.ErrorBudgetDays,
	}
}

func convertCanaryToV1(src *CanarySpec) *servingv1.CanarySpec {
	if src == nil {
		return nil
	}
	return &servingv1.CanarySpec{Weight: src.Weight, BaseModel: src.BaseModel}
}

func convertCanaryFromV1(src *servingv1.CanarySpec) *CanarySpec {
	if src == nil {
		return nil
	}
	return &CanarySpec{Weight: src.Weight, BaseModel: src.BaseModel}
}

func convertStatePlanesToV1(src StatePlanes) servingv1.StatePlanes {
	return servingv1.StatePlanes{
		Lifecycle: src.Lifecycle, Trust: src.Trust, Binding: src.Binding,
		Composition: src.Composition, Risk: src.Risk,
	}
}

func convertStatePlanesFromV1(src servingv1.StatePlanes) StatePlanes {
	return StatePlanes{
		Lifecycle: src.Lifecycle, Trust: src.Trust, Binding: src.Binding,
		Composition: src.Composition, Risk: src.Risk,
	}
}

func convertAdaptiveMetricsToV1(src *AdaptiveMetrics) *servingv1.AdaptiveMetrics {
	if src == nil {
		return nil
	}
	return &servingv1.AdaptiveMetrics{
		P50Latency: src.P50Latency, P95Latency: src.P95Latency, P99Latency: src.P99Latency,
		QueueDepth: src.QueueDepth, LoadLevel: src.LoadLevel,
	}
}

func convertAdaptiveMetricsFromV1(src *servingv1.AdaptiveMetrics) *AdaptiveMetrics {
	if src == nil {
		return nil
	}
	return &AdaptiveMetrics{
		P50Latency: src.P50Latency, P95Latency: src.P95Latency, P99Latency: src.P99Latency,
		QueueDepth: src.QueueDepth, LoadLevel: src.LoadLevel,
	}
}
