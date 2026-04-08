/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"time"

	"github.com/sony/gobreaker"
)

// CircuitBreakerSettings defines the thresholds for the circuit breaker.
type CircuitBreakerSettings struct {
	Name        string
	MaxRequests uint32
	Interval    time.Duration
	Timeout     time.Duration
}

// NewDefaultCircuitBreaker creates a gobreaker.CircuitBreaker with production defaults.
func NewDefaultCircuitBreaker(settings CircuitBreakerSettings) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        settings.Name,
		MaxRequests: settings.MaxRequests, // Number of requests allowed in half-open state
		Interval:    settings.Interval,    // How long to track failures before resetting
		Timeout:     settings.Timeout,     // How long to stay in Open state before moving to Half-Open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.ConsecutiveFailures)
			// Trip if 5 consecutive failures occur.
			return failureRatio >= 5
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// In a real implementation we would log this and possibly emit a K8s Event.
			// Since we don't have the context here, we just use a generic log if possible.
		},
	})
}
