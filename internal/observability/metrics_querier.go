/*
Copyright 2026 CKodex Authors.
*/

package observability

import (
	"context"
	"fmt"
	"math/rand"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// MetricsQuerier defines the interface for fetching real-time model performance metrics.
type MetricsQuerier interface {
	GetAdaptiveMetrics(ctx context.Context, namespace, name string) *servingv1alpha2.AdaptiveMetrics
}

// PrometheusMetricsQuerier would connect to a real Prometheus/OTel backend.
type PrometheusMetricsQuerier struct {
	BaseURL string
}

func (p *PrometheusMetricsQuerier) GetAdaptiveMetrics(ctx context.Context, namespace, name string) *servingv1alpha2.AdaptiveMetrics {
	// TODO: Implement actual Prometheus range queries here.
	// For now, this is a placeholder for the M3 Vision.
	return nil
}

// MockMetricsQuerier provides simulated metrics for testing the M3 vision.
type MockMetricsQuerier struct{}

func (m *MockMetricsQuerier) GetAdaptiveMetrics(ctx context.Context, namespace, name string) *servingv1alpha2.AdaptiveMetrics {
	// Simulate semi-realistic metrics
	p50 := 20 + rand.Intn(30)
	p95 := 80 + rand.Intn(100)
	p99 := 150 + rand.Intn(200)
	queueDepth := int64(rand.Intn(10))

	level := "None"
	if queueDepth > 5 {
		level = "Light"
	}

	return &servingv1alpha2.AdaptiveMetrics{
		P50Latency: fmt.Sprintf("%dms", p50),
		P95Latency: fmt.Sprintf("%dms", p95),
		P99Latency: fmt.Sprintf("%dms", p99),
		QueueDepth: queueDepth,
		LoadLevel:  level,
	}
}
