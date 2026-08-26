/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/
package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3Client_Pull_InvalidURI verifies error on malformed URI.
func TestIsInsecureRegistry_EnvFlag(t *testing.T) {
	t.Setenv("OCI_INSECURE", "1")
	assert.True(t, isInsecureRegistry("some.remote.registry.io"))
}

func TestIsInsecureRegistry_TrueValue(t *testing.T) {
	t.Setenv("OCI_INSECURE", "true")
	assert.True(t, isInsecureRegistry("some.registry.io"))
}

func TestIsInsecureRegistry_LocalhostPort(t *testing.T) {
	assert.True(t, isInsecureRegistry("localhost:5000"))
}

// ============================================================================
// OCIClient resolveCredentials — env vars branch
// ============================================================================

func TestOCIClient_ResolveCredentials_EnvVars(t *testing.T) {
	t.Setenv("OCI_REGISTRY_USERNAME", "myuser")
	t.Setenv("OCI_REGISTRY_PASSWORD", "mypass")
	c := &OCIClient{}
	cred := c.resolveCredentials("some.registry.io")
	assert.Equal(t, "myuser", cred.Username)
	assert.Equal(t, "mypass", cred.Password)
}

// ============================================================================
// loadDockerCredential — various branches
// ============================================================================

func TestLoadDockerCredential_ValidUserPass(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]interface{}{
		"auths": map[string]interface{}{
			"my.registry.io": map[string]string{
				"username": "dockeruser",
				"password": "dockerpass",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(cfgPath, data, 0600))

	cred, err := loadDockerCredential(cfgPath, "my.registry.io")
	require.NoError(t, err)
	assert.Equal(t, "dockeruser", cred.Username)
}

func TestLoadDockerCredential_HTTPSPrefixFallback(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]interface{}{
		"auths": map[string]interface{}{
			"https://my.registry.io": map[string]string{
				"username": "dockeruser",
				"password": "dockerpass",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(cfgPath, data, 0600))

	cred, err := loadDockerCredential(cfgPath, "my.registry.io")
	require.NoError(t, err)
	assert.Equal(t, "dockeruser", cred.Username)
}

func TestLoadDockerCredential_Base64AuthOnly_Error(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]interface{}{
		"auths": map[string]interface{}{
			"my.registry.io": map[string]string{
				"auth": "dXNlcjpwYXNz", // base64("user:pass")
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(cfgPath, data, 0600))

	_, err := loadDockerCredential(cfgPath, "my.registry.io")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base64 auth not decoded")
}

func TestLoadDockerCredential_NoRegistry_Error(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]interface{}{"auths": map[string]interface{}{}}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(cfgPath, data, 0600))

	_, err := loadDockerCredential(cfgPath, "unknown.registry.io")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no docker auth")
}

func TestLoadDockerCredential_BadJSON_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte("not-json"), 0600))

	_, err := loadDockerCredential(cfgPath, "my.registry.io")
	require.Error(t, err)
}

// ============================================================================
// DownloadFile — checksum-verified success path
// ============================================================================

// TestDownloadFile_ValidSHA256_Success exercises the checksum-verified happy path
// (VerifyFile succeeds) covering line 104: "Checksum verified: ...".
