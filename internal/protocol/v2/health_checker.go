/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v2

import (
	"context"
	"fmt"
	"time"
)

// HealthChecker uses the V2 protocol to check inference server health.
// Used by the operator to determine pod readiness via V2 endpoints
// instead of raw TCP/HTTP probes.
type HealthChecker struct {
	client *Client
}

// NewHealthChecker creates a new V2 protocol health checker.
func NewHealthChecker(baseURL string, opts ...ClientOption) *HealthChecker {
	defaultOpts := []ClientOption{
		WithTimeout(5 * time.Second),
	}
	opts = append(defaultOpts, opts...)
	return &HealthChecker{
		client: NewClient(baseURL, opts...),
	}
}

// CheckServerLive checks if the server process is alive.
// Maps to Kubernetes liveness probe.
func (h *HealthChecker) CheckServerLive(ctx context.Context) error {
	live, err := h.client.ServerLive(ctx)
	if err != nil {
		return fmt.Errorf("server liveness check failed: %w", err)
	}
	if !live {
		return fmt.Errorf("server is not live")
	}
	return nil
}

// CheckServerReady checks if the server is ready to accept requests.
// Maps to Kubernetes readiness probe.
func (h *HealthChecker) CheckServerReady(ctx context.Context) error {
	ready, err := h.client.ServerReady(ctx)
	if err != nil {
		return fmt.Errorf("server readiness check failed: %w", err)
	}
	if !ready {
		return fmt.Errorf("server is not ready")
	}
	return nil
}

// CheckModelReady checks if a specific model is loaded and ready.
// Used for readiness gates on LLMInferenceService status.
func (h *HealthChecker) CheckModelReady(ctx context.Context, modelName string) error {
	ready, err := h.client.ModelReady(ctx, modelName)
	if err != nil {
		return fmt.Errorf("model readiness check failed for %q: %w", modelName, err)
	}
	if !ready {
		return fmt.Errorf("model %q is not ready", modelName)
	}
	return nil
}

// IsHealthy performs a full health check: server live + server ready.
func (h *HealthChecker) IsHealthy(ctx context.Context) bool {
	if err := h.CheckServerLive(ctx); err != nil {
		return false
	}
	if err := h.CheckServerReady(ctx); err != nil {
		return false
	}
	return true
}
