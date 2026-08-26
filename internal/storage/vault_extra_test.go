/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ParseOCIURI
// ============================================================================

func TestInjectVaultSecrets_EmptyPath_NoOp(t *testing.T) {
	err := InjectVaultSecrets(context.Background(), "")
	require.NoError(t, err)
}

// ============================================================================
// ParseModelpackURI
// ============================================================================

func TestNewVaultClient_OK(t *testing.T) {
	// vault.NewClient only configures the client; it does NOT connect to Vault.
	t.Setenv("VAULT_ADDR", "http://127.0.0.1:19996") // unreachable but valid URL
	c, err := NewVaultClient()
	require.NoError(t, err)
	require.NotNil(t, c)
}

// buildVaultServer returns a fake Vault HTTP server that serves a fixed secret payload.
func buildVaultServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func TestVaultClient_FetchSecret_OK(t *testing.T) {
	srv := buildVaultServer(t, http.StatusOK,
		`{"request_id":"r1","lease_id":"","renewable":false,"lease_duration":0,"data":{"HF_TOKEN":"my-hf-secret"},"wrap_info":null,"warnings":null,"auth":null}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-vault-token")

	c, err := NewVaultClient()
	require.NoError(t, err)

	data, err := c.FetchSecret(context.Background(), "secret/data/hf")
	require.NoError(t, err)
	assert.Equal(t, "my-hf-secret", data["HF_TOKEN"])
}

func TestVaultClient_FetchSecret_NilData_Error(t *testing.T) {
	// Vault returns 200 but with null data (path exists, no secrets stored).
	srv := buildVaultServer(t, http.StatusOK,
		`{"request_id":"r2","lease_id":"","renewable":false,"lease_duration":0,"data":null,"wrap_info":null,"warnings":null,"auth":null}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")

	c, err := NewVaultClient()
	require.NoError(t, err)

	_, err = c.FetchSecret(context.Background(), "secret/data/empty")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data found at Vault path")
}

func TestVaultClient_FetchSecret_APIError(t *testing.T) {
	// Vault returns 403 Forbidden → SDK wraps as error.
	srv := buildVaultServer(t, http.StatusForbidden,
		`{"errors":["1 error occurred: * permission denied"]}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "bad-token")

	c, err := NewVaultClient()
	require.NoError(t, err)

	_, err = c.FetchSecret(context.Background(), "secret/data/protected")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read from Vault")
}

func TestInjectVaultSecrets_SetsEnvFromVault(t *testing.T) {
	srv := buildVaultServer(t, http.StatusOK,
		`{"request_id":"r3","lease_id":"","renewable":false,"lease_duration":0,"data":{"TEST_VAULT_KEY":"injected-value"},"wrap_info":null,"warnings":null,"auth":null}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")

	// Clean up injected env var after the test.
	t.Cleanup(func() { _ = os.Unsetenv("TEST_VAULT_KEY") })

	err := InjectVaultSecrets(context.Background(), "secret/data/inject")
	require.NoError(t, err)
	assert.Equal(t, "injected-value", os.Getenv("TEST_VAULT_KEY"))
}

// ============================================================================
// OCIClient — resolveCredentials (pure logic, no registry connection)
// ============================================================================

func TestInjectVaultSecrets_NonEmptyPath_AttemptsFetch(t *testing.T) {
	srv := buildVaultServer(t, http.StatusForbidden, `{"errors":["forbidden"]}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-vault-token")
	err := InjectVaultSecrets(context.Background(), "secret/myapp")
	require.Error(t, err)
}
