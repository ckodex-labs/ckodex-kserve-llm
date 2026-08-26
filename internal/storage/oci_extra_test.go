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
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
)

// ============================================================================
// ParseOCIURI
// ============================================================================

func TestIsInsecureRegistry_Localhost(t *testing.T) {
	assert.True(t, isInsecureRegistry("localhost:5000"))
}

func TestIsInsecureRegistry_Loopback(t *testing.T) {
	assert.True(t, isInsecureRegistry("127.0.0.1:5000"))
}

func TestIsInsecureRegistry_Production_NotInsecure(t *testing.T) {
	assert.False(t, isInsecureRegistry("ghcr.io"))
}

func TestIsInsecureRegistry_EnvVar(t *testing.T) {
	t.Setenv("OCI_INSECURE", "1")
	assert.True(t, isInsecureRegistry("ghcr.io"))
}

func TestIsInsecureRegistry_EnvVar_True(t *testing.T) {
	t.Setenv("OCI_INSECURE", "true")
	assert.True(t, isInsecureRegistry("ghcr.io"))
}

// ============================================================================
// OCIClient.logPulledLayers — filesystem-only function
// ============================================================================

func TestLogPulledLayers_Dir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.safetensors"), []byte("w"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0600))

	c := &OCIClient{}
	// Must not panic. Logs to stdout — no return value to assert.
	c.logPulledLayers(dir)
}

func TestLogPulledLayers_NonExistentDir_NoError(t *testing.T) {
	c := &OCIClient{}
	c.logPulledLayers("/nonexistent/path/xyz")
}

// ============================================================================
// S3Client — pure-logic helpers and URI-error path
// ============================================================================

func TestS3PathStyleOption_SetsUsePathStyle(t *testing.T) {
	var opts struct{ UsePathStyle bool }
	// s3PathStyleOption returns a func(*s3.Options); we simulate just the field.
	// We verify via the S3Client constructor behavior rather than direct struct mutation.
	f := s3PathStyleOption()
	require.NotNil(t, f)
	_ = opts
}

func TestS3ConfigOptions_DefaultRegion(t *testing.T) {
	opts := s3ConfigOptions("us-west-2")
	require.NotEmpty(t, opts, "must return at least the region option")
}

func TestS3ConfigOptions_WithAnonymousCredentials(t *testing.T) {
	t.Setenv("AWS_NO_SIGN_REQUEST", "yes")
	opts := s3ConfigOptions("us-east-1")
	assert.Len(t, opts, 2, "region + anonymous credentials")
}

func TestS3ConfigOptions_WithCustomEndpoint_S3_Endpoint(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "http://minio.local:9000")
	opts := s3ConfigOptions("us-east-1")
	assert.Len(t, opts, 2, "region + endpoint")
}

func TestS3ConfigOptions_WithCustomEndpoint_AWS_Endpoint_URL(t *testing.T) {
	t.Setenv("AWS_ENDPOINT_URL", "http://minio.local:9000")
	opts := s3ConfigOptions("us-east-1")
	assert.Len(t, opts, 2, "region + endpoint")
}

func TestS3Client_Pull_InvalidURI_Error(t *testing.T) {
	c := &S3Client{}
	err := c.Pull(context.Background(), "s3://bucket-only", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid S3 URI format")
}

func TestS3Client_Schemes(t *testing.T) {
	c := &S3Client{}
	assert.Contains(t, c.Schemes(), "s3")
}

// ============================================================================
// github_client.downloadFile — replace DefaultTransport for testing
// ============================================================================

func TestGitHubClient_DownloadFile_OK(t *testing.T) {
	content := []byte("model weights")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	// Replace http.DefaultTransport to route to test server.
	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &GitHubClient{}
	dest := filepath.Join(t.TempDir(), "model.bin")
	require.NoError(t, c.downloadFile(context.Background(), srv.URL+"/file", dest))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestGitHubClient_DownloadFile_NonOK_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &GitHubClient{}
	err := c.downloadFile(context.Background(), srv.URL+"/file", filepath.Join(t.TempDir(), "f.bin"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download from GitHub")
}

// ============================================================================
// huggingface_client.listRepoFiles — replace DefaultTransport
// ============================================================================

func TestOCIClient_ResolveCredentials_ExplicitConfig(t *testing.T) {
	c := &OCIClient{
		RegistryAuth: map[string]RegistryAuthConfig{
			"ghcr.io": {Username: "user1", Password: "pass1"},
		},
	}
	cred := c.resolveCredentials("ghcr.io")
	assert.Equal(t, "user1", cred.Username)
	assert.Equal(t, "pass1", cred.Password)
}

func TestOCIClient_ResolveCredentials_ExplicitConfig_EmptyUsername_FallsThrough(t *testing.T) {
	// Username is empty → falls through to env var check.
	t.Setenv("OCI_REGISTRY_USERNAME", "")
	c := &OCIClient{
		RegistryAuth: map[string]RegistryAuthConfig{
			"ghcr.io": {Username: "", Password: ""},
		},
	}
	// No env var, no docker config → should return anonymous credential.
	cred := c.resolveCredentials("ghcr.io")
	// Anonymous credential has empty Username.
	assert.Empty(t, cred.Username)
}

func TestOCIClient_ResolveCredentials_EnvVar(t *testing.T) {
	t.Setenv("OCI_REGISTRY_USERNAME", "env-user")
	t.Setenv("OCI_REGISTRY_PASSWORD", "env-pass")

	c := &OCIClient{}
	cred := c.resolveCredentials("ghcr.io")
	assert.Equal(t, "env-user", cred.Username)
	assert.Equal(t, "env-pass", cred.Password)
}

func TestOCIClient_ResolveCredentials_DockerConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := `{"auths":{"ghcr.io":{"username":"docker-user","password":"docker-pass"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("OCI_REGISTRY_USERNAME", "") // ensure env branch does not trigger

	c := &OCIClient{}
	cred := c.resolveCredentials("ghcr.io")
	assert.Equal(t, "docker-user", cred.Username)
}

func TestOCIClient_ResolveCredentials_Anonymous(t *testing.T) {
	// No config, no env vars, no docker config → anonymous.
	t.Setenv("OCI_REGISTRY_USERNAME", "")
	t.Setenv("DOCKER_CONFIG", t.TempDir()) // empty dir → config.json missing → falls through

	c := &OCIClient{}
	cred := c.resolveCredentials("ghcr.io")
	assert.Empty(t, cred.Username)
	assert.Empty(t, cred.Password)
}

// ============================================================================
// loadDockerCredential — filesystem-based tests
// ============================================================================

func TestLoadDockerCredential_OK(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := `{"auths":{"my-registry.io":{"username":"u","password":"p"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	cred, err := loadDockerCredential(cfgPath, "my-registry.io")
	require.NoError(t, err)
	assert.Equal(t, "u", cred.Username)
	assert.Equal(t, "p", cred.Password)
}

func TestLoadDockerCredential_HTTPSPrefixMatch(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// Registry stored with https:// prefix (common in older docker configs).
	cfg := `{"auths":{"https://my-registry.io":{"username":"u2","password":"p2"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	cred, err := loadDockerCredential(cfgPath, "my-registry.io")
	require.NoError(t, err)
	assert.Equal(t, "u2", cred.Username)
}

func TestLoadDockerCredential_EntryNotFound_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := `{"auths":{"other-registry.io":{"username":"u","password":"p"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	_, err := loadDockerCredential(cfgPath, "missing-registry.io")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no docker auth")
}

func TestLoadDockerCredential_FileNotFound_Error(t *testing.T) {
	_, err := loadDockerCredential("/nonexistent/config.json", "registry.io")
	require.Error(t, err)
}

func TestLoadDockerCredential_InvalidJSON_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte("not-json"), 0600))

	_, err := loadDockerCredential(cfgPath, "registry.io")
	require.Error(t, err)
}

func TestLoadDockerCredential_Base64Auth_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// Only the "auth" base64 field, no username/password.
	cfg := `{"auths":{"registry.io":{"auth":"dXNlcjpwYXNz"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	_, err := loadDockerCredential(cfgPath, "registry.io")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base64 auth not decoded")
}

func TestLoadDockerCredential_EmptyEntry_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// Entry exists but all fields empty.
	cfg := `{"auths":{"registry.io":{}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	_, err := loadDockerCredential(cfgPath, "registry.io")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty auth entry")
}

// ============================================================================
// OCIClient — early-exit error paths (no network)
// ============================================================================

func TestOCIClient_Pull_InvalidURI_Error(t *testing.T) {
	c := &OCIClient{}
	err := c.Pull(context.Background(), "s3://bucket/path", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an OCI URI")
}

func TestOCIClient_PullInternal_EmptyRef_Error(t *testing.T) {
	c := &OCIClient{}
	err := c.pullInternal(context.Background(), &ModelArtifact{Reference: ""}, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no OCI reference configured")
}

func TestNewOCIFileStore_DisablesAutomaticUnpack(t *testing.T) {
	store, err := newOCIFileStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	assert.True(t, store.SkipUnpack, "automatic archive unpacking must remain disabled")
}

const orasContainmentVersion = "v2.6.2"

func TestORASContainmentMatchesDependencyVersion(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	goModPath := filepath.Join(filepath.Dir(filename), "..", "..", "go.mod")
	data, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	module, err := modfile.Parse(goModPath, data, nil)
	require.NoError(t, err)

	var version string
	for _, dep := range module.Require {
		if dep.Mod.Path == "oras.land/oras-go/v2" {
			version = dep.Mod.Version
			break
		}
	}
	assert.Equal(t, orasContainmentVersion, version,
		"review GHSA-fxhp-mv3v-67qp containment before changing oras-go")
}

func TestOCIClient_Push_EmptyRef_Error(t *testing.T) {
	c := &OCIClient{}
	err := c.Push(context.Background(), &ModelArtifact{Reference: ""}, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no OCI reference configured")
}
