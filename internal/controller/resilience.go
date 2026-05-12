/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"fmt"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/sony/gobreaker"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// CircuitBreakerSettings defines the thresholds for the circuit breaker.
type CircuitBreakerSettings struct {
	Name        string
	MaxRequests uint32
	Interval    time.Duration
	Timeout     time.Duration
}

// NewDefaultCircuitBreaker creates a gobreaker.CircuitBreaker with production defaults.
func NewDefaultCircuitBreaker(settings CircuitBreakerSettings, recorder record.EventRecorder, affectedObject metav1.Object) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        settings.Name,
		MaxRequests: settings.MaxRequests, // Number of requests allowed in half-open state
		Interval:    settings.Interval,    // How long to track failures before resetting
		Timeout:     settings.Timeout,     // How long to stay in Open state before moving to Half-Open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip if 5 consecutive failures occur.
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// 1. Update Metrics
			stateVal := 0.0
			switch to {
			case gobreaker.StateClosed:
				stateVal = 0.0
			case gobreaker.StateHalfOpen:
				stateVal = 1.0
			case gobreaker.StateOpen:
				stateVal = 2.0
				observability.ResilienceCircuitBreakerTripped.WithLabelValues(name).Inc()
			}
			observability.ResilienceCircuitBreakerState.WithLabelValues(name).Set(stateVal)

			// 2. Emit Kubernetes Event
			if recorder != nil && affectedObject != nil {
				if robj, ok := affectedObject.(runtime.Object); ok {
					msg := fmt.Sprintf("Circuit Breaker %s changed state: %s -> %s", name, from, to)
					eventType := corev1.EventTypeNormal
					if to == gobreaker.StateOpen {
						eventType = corev1.EventTypeWarning
					}
					recorder.Event(robj, eventType, "CircuitBreakerStateChange", msg)
				}
			}
		},
	})
}
