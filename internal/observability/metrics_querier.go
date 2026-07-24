/*
Copyright 2026 CKodex Authors.
*/

package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// MetricsQuerier defines the interface for fetching real-time model performance metrics.
type MetricsQuerier interface {
	GetAdaptiveMetrics(ctx context.Context, namespace, name string) *servingv1alpha2.AdaptiveMetrics
}

// PrometheusMetricsQuerier would connect to a real Prometheus/OTel backend.
type PrometheusMetricsQuerier struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (p *PrometheusMetricsQuerier) GetAdaptiveMetrics(ctx context.Context, namespace, name string) *servingv1alpha2.AdaptiveMetrics {
	if p == nil || strings.TrimSpace(p.BaseURL) == "" || strings.TrimSpace(name) == "" {
		return nil
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	// Recording rules aggregate by model. Namespace is retained in the API so
	// callers can evolve to namespace-scoped rules without changing the contract.
	_ = namespace
	metrics := &servingv1alpha2.AdaptiveMetrics{}
	values := map[string]*string{
		"p50": &metrics.P50Latency,
		"p95": &metrics.P95Latency,
		"p99": &metrics.P99Latency,
	}
	found := false
	for quantile, target := range values {
		v, ok := p.query(ctx, client, fmt.Sprintf("ckodex:inference_latency_%s{model=%q}", quantile, name))
		if ok {
			*target = fmt.Sprintf("%gms", v*1000)
			found = true
		}
	}
	if queue, ok := p.query(ctx, client, fmt.Sprintf("sum(ckodex_scheduler_queue_depth{model=%q})", name)); ok {
		metrics.QueueDepth = int64(queue)
		metrics.LoadLevel = loadLevel(metrics.QueueDepth)
		found = true
	}
	if !found {
		return nil
	}
	return metrics
}

type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Value [2]any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (p *PrometheusMetricsQuerier) query(ctx context.Context, client *http.Client, promQL string) (float64, bool) {
	u, err := url.Parse(strings.TrimRight(p.BaseURL, "/") + "/api/v1/query")
	if err != nil {
		return 0, false
	}
	q := u.Query()
	q.Set("query", promQL)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, false
	}
	var decoded prometheusResponse
	if json.Unmarshal(body, &decoded) != nil || decoded.Status != "success" || len(decoded.Data.Result) == 0 {
		return 0, false
	}
	raw, ok := decoded.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	return v, err == nil
}

func loadLevel(queueDepth int64) string {
	switch {
	case queueDepth >= 20:
		return "Severe"
	case queueDepth >= 10:
		return "Moderate"
	case queueDepth >= 5:
		return "Light"
	default:
		return "None"
	}
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
