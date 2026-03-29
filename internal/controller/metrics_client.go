/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MetricsQuerier abstracts Prometheus metric queries used for promotion gate evaluation.
// Keeping this as an interface lets tests inject deterministic stubs without a running
// Prometheus, while production wires in a PrometheusMetricsQuerier.
type MetricsQuerier interface {
	// QuerySuccessRate returns the request success rate (0–100) for the given model
	// over the trailing 5-minute window.
	QuerySuccessRate(ctx context.Context, modelName, namespace string) (float64, error)

	// QueryP99LatencyMS returns the P99 end-to-end request latency in milliseconds
	// for the given model over the trailing 5-minute window.
	QueryP99LatencyMS(ctx context.Context, modelName, namespace string) (int64, error)
}

// PrometheusMetricsQuerier queries the Prometheus HTTP API for promotion gate metrics.
// It uses the standard /api/v1/query endpoint (no external library dependency).
//
// PromQL expressions target the labels emitted by the CKodex vLLM sidecar exporter:
//
//	model    = LLMInferenceService.Spec.Model.Name  (set by the request pipeline)
//	namespace = Kubernetes namespace
type PrometheusMetricsQuerier struct {
	// BaseURL is the Prometheus base URL, e.g. "http://prometheus.monitoring:9090".
	BaseURL string
	// HTTPClient is the HTTP client used for queries. Defaults to a 10 s timeout client.
	HTTPClient *http.Client
}

// NewPrometheusMetricsQuerier creates a PrometheusMetricsQuerier with a 10 s timeout.
func NewPrometheusMetricsQuerier(baseURL string) *PrometheusMetricsQuerier {
	return &PrometheusMetricsQuerier{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// QuerySuccessRate queries Prometheus for the 5-minute success rate of the given model.
// Returns a value in [0, 100]. Returns 0 when the time-series is empty (model has no
// recorded traffic yet — callers must treat this as a gate failure, not a pass).
func (p *PrometheusMetricsQuerier) QuerySuccessRate(ctx context.Context, modelName, namespace string) (float64, error) {
	// Rate of successful requests / total requests × 100.
	// Labels follow the CKodex vLLM sidecar convention:
	//   model     = LLMInferenceService model name
	//   namespace = Kubernetes namespace
	promQL := fmt.Sprintf(
		`sum(rate(ckodex_inference_requests_total{model=%q,namespace=%q,status="success"}[5m]))`+
			` / sum(rate(ckodex_inference_requests_total{model=%q,namespace=%q}[5m])) * 100`,
		modelName, namespace, modelName, namespace,
	)
	v, err := p.queryScalar(ctx, promQL)
	if err != nil {
		return 0, fmt.Errorf("success rate query for model %q: %w", modelName, err)
	}
	return v, nil
}

// QueryP99LatencyMS queries Prometheus for the 5-minute P99 latency in milliseconds.
// Returns 0 when the histogram is empty (no traffic). Callers must treat 0 as a gate
// failure — a model with no traffic has not yet proved P99 compliance.
func (p *PrometheusMetricsQuerier) QueryP99LatencyMS(ctx context.Context, modelName, namespace string) (int64, error) {
	// histogram_quantile over the CKodex latency histogram (recorded in seconds).
	// Multiply by 1000 to convert to milliseconds for comparison with GateCriteria.MaxLatencyP99.
	promQL := fmt.Sprintf(
		`histogram_quantile(0.99, sum(rate(ckodex_inference_request_duration_seconds_bucket{model=%q,namespace=%q}[5m])) by (le)) * 1000`,
		modelName, namespace,
	)
	v, err := p.queryScalar(ctx, promQL)
	if err != nil {
		return 0, fmt.Errorf("P99 latency query for model %q: %w", modelName, err)
	}
	return int64(v), nil
}

// prometheusQueryResponse is the JSON envelope returned by /api/v1/query.
type prometheusQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]any            `json:"value"` // [timestamp, stringValue]
		} `json:"result"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

// queryScalar executes an instant PromQL query and extracts the scalar result.
// Returns (0, nil) when the result set is empty (no time-series match).
// Returns an error when the HTTP request fails or the result is not a scalar vector.
func (p *PrometheusMetricsQuerier) queryScalar(ctx context.Context, promQL string) (float64, error) {
	queryURL, err := url.Parse(p.BaseURL + "/api/v1/query")
	if err != nil {
		return 0, fmt.Errorf("parse prometheus URL: %w", err)
	}
	q := queryURL.Query()
	q.Set("query", promQL)
	queryURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("build prometheus request: %w", err)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("execute prometheus request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read prometheus response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result prometheusQueryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("decode prometheus response: %w", err)
	}
	if result.Status != "success" {
		return 0, fmt.Errorf("prometheus query failed (%s): %s", result.ErrorType, result.Error)
	}

	// Empty result set — metric has no data (new model with no traffic yet).
	if len(result.Data.Result) == 0 {
		return 0, nil
	}

	// Extract the string-encoded float from Value[1].
	raw, ok := result.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected prometheus value type: %T", result.Data.Result[0].Value[1])
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse prometheus scalar %q: %w", raw, err)
	}
	return v, nil
}

// noopMetricsQuerier is used when no Prometheus URL is configured.
// It always passes gates by returning values that satisfy any threshold —
// this preserves backward-compatibility for operators that have not yet
// wired up a Prometheus endpoint.
//
// Operators should set PrometheusURL in OperatorConfig to enable real gate enforcement.
type noopMetricsQuerier struct{}

func (noopMetricsQuerier) QuerySuccessRate(_ context.Context, _, _ string) (float64, error) {
	return 100, nil // assume 100 % success — no data
}

func (noopMetricsQuerier) QueryP99LatencyMS(_ context.Context, _, _ string) (int64, error) {
	return 0, nil // assume 0 ms — no data
}
