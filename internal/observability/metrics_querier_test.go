package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrometheusMetricsQuerier_GetAdaptiveMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		value := "0.125"
		if q == "sum(ckodex_scheduler_queue_depth{model=\"gemma\"})" {
			value = "12"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":["1","` + value + `"]}]}}`))
	}))
	defer server.Close()

	metrics := (&PrometheusMetricsQuerier{BaseURL: server.URL}).GetAdaptiveMetrics(context.Background(), "default", "gemma")
	require.NotNil(t, metrics)
	require.Equal(t, "125ms", metrics.P50Latency)
	require.Equal(t, "125ms", metrics.P95Latency)
	require.Equal(t, "125ms", metrics.P99Latency)
	require.Equal(t, int64(12), metrics.QueueDepth)
	require.Equal(t, "Moderate", metrics.LoadLevel)
}

func TestPrometheusMetricsQuerier_EmptyOrUnavailableReturnsNil(t *testing.T) {
	require.Nil(t, (&PrometheusMetricsQuerier{}).GetAdaptiveMetrics(context.Background(), "default", "gemma"))
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	require.Nil(t, (&PrometheusMetricsQuerier{BaseURL: server.URL}).GetAdaptiveMetrics(context.Background(), "default", "gemma"))
}
