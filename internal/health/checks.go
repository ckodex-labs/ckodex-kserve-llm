/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package health provides readiness health checks for optional operator subsystems.
// Each check is registered as a readiness gate so the operator pod only enters
// Ready state when its dependencies are reachable. This prevents the operator from
// appearing healthy while silently failing to reconcile security or secret resources.
package health

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// VaultHealthCheck verifies that the Vault agent sidecar (or injector) is reachable.
// Vault availability is required when EnableSecurity=true because the operator fetches
// dynamic secrets (registry credentials, TLS certs) from Vault at reconcile time.
//
// The check probes the Vault agent's local listener at 127.0.0.1:8200/v1/sys/health.
// Vault returns 200 (active), 429 (standby), or 473 (perf standby) — all of these
// indicate a reachable and functioning cluster. Only network errors and 5xx responses
// are treated as unhealthy.
type VaultHealthCheck struct {
	// AgentAddr is the Vault agent address. Defaults to "http://127.0.0.1:8200".
	AgentAddr  string
	httpClient *http.Client
}

// NewVaultHealthCheck creates a Vault check pointing at the local agent sidecar.
func NewVaultHealthCheck() *VaultHealthCheck {
	return &VaultHealthCheck{
		AgentAddr: "http://127.0.0.1:8200",
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// Check implements healthz.Checker. Returns nil when Vault agent is reachable.
func (v *VaultHealthCheck) Check(_ *http.Request) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	url := v.AgentAddr + "/v1/sys/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("vault health: failed to build request: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vault health: agent unreachable at %s: %w", v.AgentAddr, err)
	}
	defer resp.Body.Close()

	// 200 = active, 429 = standby, 473 = performance standby — all acceptable.
	switch resp.StatusCode {
	case http.StatusOK, http.StatusTooManyRequests, 473:
		return nil
	default:
		return fmt.Errorf("vault health: agent returned unexpected status %d", resp.StatusCode)
	}
}

// GatekeeperHealthCheck verifies that Gatekeeper's webhook service is available.
// When OPA enforcement is enabled, Gatekeeper must be running before the operator
// creates ConstraintTemplates and Constraints — otherwise OPA reconcile will silently
// install policies that are never enforced because the admission webhook isn't registered.
//
// The check probes the Gatekeeper webhook service in the gatekeeper-system namespace
// via the in-cluster DNS name. A connection refused or timeout indicates Gatekeeper
// is not yet running or has not passed its own readiness gate.
type GatekeeperHealthCheck struct {
	// WebhookURL is the Gatekeeper webhook health endpoint.
	// Defaults to "http://gatekeeper-webhook-service.gatekeeper-system.svc:8888/readyz".
	WebhookURL string
	httpClient *http.Client
}

// NewGatekeeperHealthCheck creates a Gatekeeper check using in-cluster DNS.
func NewGatekeeperHealthCheck() *GatekeeperHealthCheck {
	return &GatekeeperHealthCheck{
		WebhookURL: "http://gatekeeper-webhook-service.gatekeeper-system.svc:8888/readyz",
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// Check implements healthz.Checker. Returns nil when Gatekeeper webhook is reachable.
func (g *GatekeeperHealthCheck) Check(_ *http.Request) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.WebhookURL, nil)
	if err != nil {
		return fmt.Errorf("gatekeeper health: failed to build request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gatekeeper health: webhook service unreachable at %s: %w", g.WebhookURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gatekeeper health: webhook service returned status %d", resp.StatusCode)
	}

	return nil
}

// SPIREHealthCheck verifies that the SPIRE Agent socket is present and accepting connections.
// The SPIRE Agent exposes the SPIFFE Workload API over a Unix domain socket at
// /run/spiffe/workload/spiffe-workload.sock. When this socket is absent or not accepting
// connections, pods requesting SVIDs will hang indefinitely — making SPIRE availability a
// hard readiness dependency when EnableSecurity=true.
//
// The check connects to the SPIRE Agent health endpoint (HTTP over TCP, not the gRPC socket)
// which is exposed at 0.0.0.0:8080/live on the DaemonSet pods. Accessed via in-cluster DNS.
type SPIREHealthCheck struct {
	// AgentHealthURL is the SPIRE agent health endpoint.
	// Defaults to "http://spire-agent.spire.svc:8080/live".
	AgentHealthURL string
	httpClient     *http.Client
}

// NewSPIREHealthCheck creates a SPIRE check using in-cluster DNS.
func NewSPIREHealthCheck() *SPIREHealthCheck {
	return &SPIREHealthCheck{
		AgentHealthURL: "http://spire-agent.spire.svc:8080/live",
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// Check implements healthz.Checker. Returns nil when SPIRE agent is live.
func (s *SPIREHealthCheck) Check(_ *http.Request) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.AgentHealthURL, nil)
	if err != nil {
		return fmt.Errorf("spire health: failed to build request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spire health: agent unreachable at %s: %w", s.AgentHealthURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spire health: agent returned status %d", resp.StatusCode)
	}

	return nil
}
