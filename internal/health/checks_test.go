/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package health

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- helpers ---------------------------------------------------------------

// testServer starts an httptest server that always returns the given status code.
func testServer(statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(statusCode)
	}))
}

// ---- VaultHealthCheck ------------------------------------------------------

func TestVaultHealthCheck_New_NotNil(t *testing.T) {
	v := NewVaultHealthCheck()
	require.NotNil(t, v)
	assert.Equal(t, "http://127.0.0.1:8200", v.AgentAddr)
}

func TestVaultHealthCheck_Active_200(t *testing.T) {
	srv := testServer(http.StatusOK)
	defer srv.Close()

	v := &VaultHealthCheck{AgentAddr: srv.URL, httpClient: srv.Client()}
	assert.NoError(t, v.Check(nil))
}

func TestVaultHealthCheck_Standby_429(t *testing.T) {
	srv := testServer(http.StatusTooManyRequests) // 429 = standby — acceptable
	defer srv.Close()

	v := &VaultHealthCheck{AgentAddr: srv.URL, httpClient: srv.Client()}
	assert.NoError(t, v.Check(nil))
}

func TestVaultHealthCheck_PerfStandby_473(t *testing.T) {
	srv := testServer(473)
	defer srv.Close()

	v := &VaultHealthCheck{AgentAddr: srv.URL, httpClient: srv.Client()}
	assert.NoError(t, v.Check(nil))
}

func TestVaultHealthCheck_ServerError_500(t *testing.T) {
	srv := testServer(http.StatusInternalServerError)
	defer srv.Close()

	v := &VaultHealthCheck{AgentAddr: srv.URL, httpClient: srv.Client()}
	err := v.Check(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestVaultHealthCheck_NotFound_404(t *testing.T) {
	srv := testServer(http.StatusNotFound)
	defer srv.Close()

	v := &VaultHealthCheck{AgentAddr: srv.URL, httpClient: srv.Client()}
	err := v.Check(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 404")
}

func TestVaultHealthCheck_Unreachable(t *testing.T) {
	// Point at a port where nothing is listening.
	v := &VaultHealthCheck{
		AgentAddr:  "http://127.0.0.1:19999",
		httpClient: &http.Client{},
	}
	err := v.Check(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent unreachable")
}

// ---- GatekeeperHealthCheck -------------------------------------------------

func TestGatekeeperHealthCheck_New_NotNil(t *testing.T) {
	g := NewGatekeeperHealthCheck()
	require.NotNil(t, g)
	assert.Contains(t, g.WebhookURL, "gatekeeper-system")
}

func TestGatekeeperHealthCheck_OK_200(t *testing.T) {
	srv := testServer(http.StatusOK)
	defer srv.Close()

	g := &GatekeeperHealthCheck{WebhookURL: srv.URL + "/readyz", httpClient: srv.Client()}
	assert.NoError(t, g.Check(nil))
}

func TestGatekeeperHealthCheck_NotReady_503(t *testing.T) {
	srv := testServer(http.StatusServiceUnavailable)
	defer srv.Close()

	g := &GatekeeperHealthCheck{WebhookURL: srv.URL + "/readyz", httpClient: srv.Client()}
	err := g.Check(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
}

func TestGatekeeperHealthCheck_Unauthorized_401(t *testing.T) {
	srv := testServer(http.StatusUnauthorized)
	defer srv.Close()

	g := &GatekeeperHealthCheck{WebhookURL: srv.URL + "/readyz", httpClient: srv.Client()}
	err := g.Check(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

func TestGatekeeperHealthCheck_Unreachable(t *testing.T) {
	g := &GatekeeperHealthCheck{
		WebhookURL: "http://127.0.0.1:19998/readyz",
		httpClient: &http.Client{},
	}
	err := g.Check(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webhook service unreachable")
}

// ---- SPIREHealthCheck ------------------------------------------------------

func TestSPIREHealthCheck_New_NotNil(t *testing.T) {
	s := NewSPIREHealthCheck()
	require.NotNil(t, s)
	assert.Contains(t, s.AgentHealthURL, "spire-agent")
}

func TestSPIREHealthCheck_Live_200(t *testing.T) {
	srv := testServer(http.StatusOK)
	defer srv.Close()

	s := &SPIREHealthCheck{AgentHealthURL: srv.URL + "/live", httpClient: srv.Client()}
	assert.NoError(t, s.Check(nil))
}

func TestSPIREHealthCheck_NotLive_503(t *testing.T) {
	srv := testServer(http.StatusServiceUnavailable)
	defer srv.Close()

	s := &SPIREHealthCheck{AgentHealthURL: srv.URL + "/live", httpClient: srv.Client()}
	err := s.Check(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
}

func TestSPIREHealthCheck_ServerError_500(t *testing.T) {
	srv := testServer(http.StatusInternalServerError)
	defer srv.Close()

	s := &SPIREHealthCheck{AgentHealthURL: srv.URL + "/live", httpClient: srv.Client()}
	err := s.Check(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestSPIREHealthCheck_Unreachable(t *testing.T) {
	s := &SPIREHealthCheck{
		AgentHealthURL: "http://127.0.0.1:19997/live",
		httpClient:     &http.Client{},
	}
	err := s.Check(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent unreachable")
}
