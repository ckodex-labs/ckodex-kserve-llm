/*
Copyright 2026 CKodex Authors.
*/

package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// LMCDownloadDuration measures how long it takes to warm a model cache on a node.
	LMCDownloadDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ckodex_lmc_download_duration_seconds",
			Help:    "Duration of model cache warming jobs in seconds.",
			Buckets: []float64{60, 300, 600, 1800, 3600}, // 1m to 1h
		},
		[]string{"model_uri", "node"},
	)

	// LMCCacheSize measures the total size of local model caches currently in 'Ready' state.
	LMCCacheSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ckodex_lmc_pvc_size_bytes",
			Help: "Total storage used by LocalModelCache PVCs in bytes.",
		},
		[]string{"model_uri", "node"},
	)

	LMCWarmingAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ckodex_lmc_warming_attempts_total",
			Help: "Total number of model cache warming attempts.",
		},
		[]string{"model_uri", "node", "status"},
	)

	// GovernanceState tracks the distribution of adapters across the composite state planes.
	GovernanceState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ckodex_governance_adapter_state",
			Help: "Current count of adapters in each lifecycle state.",
		},
		[]string{"lifecycle", "trust"},
	)

	// QuarantineIncidents tracks every time an adapter is forcibly blocked.
	QuarantineIncidents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ckodex_governance_quarantine_total",
			Help: "Total number of times an adapter has been moved to quarantine.",
		},
		[]string{"adapter_name", "reason"},
	)

	// ContextUtilization tracks the percentage of the model's context window being used.
	ContextUtilization = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ckodex_inference_context_utilization",
			Help:    "Percentage of context window utilized per request.",
			Buckets: []float64{0.1, 0.25, 0.5, 0.75, 0.9, 1.0},
		},
		[]string{"model_name", "engine"},
	)

	// KVCachePressure tracks the memory pressure on the KV cache.
	KVCachePressure = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ckodex_inference_kv_cache_pressure",
			Help: "Current memory pressure on the KV cache (0-1).",
		},
		[]string{"model_name", "engine"},
	)
)

var (
	// ResilienceCircuitBreakerState tracks the current state of a circuit breaker (0=Closed, 1=HalfOpen, 2=Open).
	ResilienceCircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ckodex_resilience_circuit_breaker_state",
			Help: "Current state of the circuit breaker (0=Closed, 1=HalfOpen, 2=Open).",
		},
		[]string{"name"},
	)

	// ResilienceCircuitBreakerTripped tracks the total number of times a circuit breaker has tripped (moved to Open).
	ResilienceCircuitBreakerTripped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ckodex_resilience_circuit_breaker_tripped_total",
			Help: "Total number of times the circuit breaker has tripped (entered Open state).",
		},
		[]string{"name"},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		LMCDownloadDuration,
		LMCCacheSize,
		LMCWarmingAttempts,
		GovernanceState,
		QuarantineIncidents,
		ContextUtilization,
		KVCachePressure,
		ResilienceCircuitBreakerState,
		ResilienceCircuitBreakerTripped,
	)
}
