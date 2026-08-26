/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-github/v62/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ParseOCIURI
// ============================================================================

func TestParseModelpackURI_TagReference(t *testing.T) {
	a, err := ParseModelpackURI("modelpack://registry.acme.com/ckodex/llama3:v2")
	require.NoError(t, err)
	assert.Equal(t, "registry.acme.com", a.Registry)
	assert.Equal(t, "ckodex/llama3", a.Repository)
	assert.Equal(t, "v2", a.Reference)
	assert.Empty(t, a.Digest)
}

func TestParseModelpackURI_DigestReference(t *testing.T) {
	a, err := ParseModelpackURI("modelpack://registry.acme.com/ckodex/llama3@sha256:abc")
	require.NoError(t, err)
	assert.Equal(t, "sha256:abc", a.Digest)
	assert.Equal(t, "sha256:abc", a.Reference)
}

func TestParseModelpackURI_NoTag_DefaultsToLatest(t *testing.T) {
	a, err := ParseModelpackURI("modelpack://registry.acme.com/ckodex/llama3")
	require.NoError(t, err)
	assert.Equal(t, "latest", a.Reference)
}

func TestParseModelpackURI_NotModelpack_Error(t *testing.T) {
	_, err := ParseModelpackURI("oci://registry/repo:tag")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a Modelpack URI")
}

func TestParseModelpackURI_MissingRepo_Error(t *testing.T) {
	_, err := ParseModelpackURI("modelpack://registry-only")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Modelpack URI")
}

// ============================================================================
// GitHubClient — constructor and URI-error path
// ============================================================================

func TestNewGitHubClient_NotNil(t *testing.T) {
	c := NewGitHubClient(context.Background(), "")
	require.NotNil(t, c)
}

func TestGitHubClient_Pull_InvalidURI_Error(t *testing.T) {
	c := NewGitHubClient(context.Background(), "")
	// Less than 3 path segments → error before any network call.
	err := c.Pull(context.Background(), "github://owner/repo", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GitHub URI format")
}

func TestGitHubClient_Schemes(t *testing.T) {
	c := NewGitHubClient(context.Background(), "")
	assert.Contains(t, c.Schemes(), "github")
}

// ============================================================================
// GitLabClient — constructor and URI-error path
// ============================================================================

func TestNewGitLabClient_OK(t *testing.T) {
	c, err := NewGitLabClient("")
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestGitLabClient_Pull_InvalidURI_Error(t *testing.T) {
	c, err := NewGitLabClient("")
	require.NoError(t, err)
	// Less than 2 path segments → error before any network call.
	err = c.Pull(context.Background(), "gitlab://project-only", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GitLab URI format")
}

func TestGitLabClient_Schemes(t *testing.T) {
	c, err := NewGitLabClient("")
	require.NoError(t, err)
	assert.Contains(t, c.Schemes(), "gitlab")
}

// ============================================================================
// ArtifactoryClient — URI-error path (no network)
// ============================================================================

func TestArtifactoryClient_Pull_InvalidURI_Error(t *testing.T) {
	c := &ArtifactoryClient{} // nil manager triggers env-based init, but URI fails first
	err := c.Pull(context.Background(), "artifactory://host/repo", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Artifactory URI format")
}

func TestArtifactoryClient_Schemes(t *testing.T) {
	c := &ArtifactoryClient{}
	assert.Contains(t, c.Schemes(), "artifactory")
}

func TestArtifactoryClient_DownloadFile_NoOp(t *testing.T) {
	c := &ArtifactoryClient{}
	require.NoError(t, c.downloadFile(context.Background(), "http://unused", t.TempDir()))
}

// ============================================================================
// ModelpackClient — URI-error path (no network)
// ============================================================================

func TestModelpackClient_Pull_InvalidURI_Error(t *testing.T) {
	c := &ModelpackClient{}
	err := c.Pull(context.Background(), "not-modelpack://foo", t.TempDir())
	require.Error(t, err)
}

func TestModelpackClient_Schemes(t *testing.T) {
	c := &ModelpackClient{}
	assert.Contains(t, c.Schemes(), "modelpack")
}

// ============================================================================
// isInsecureRegistry (OCI)
// ============================================================================

func TestGitHubClient_PullRecursive_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	baseURL, _ := url.Parse(srv.URL + "/")
	ghClient := github.NewClient(srv.Client())
	ghClient.BaseURL = baseURL

	c := &GitHubClient{}
	err := c.pullRecursive(context.Background(), ghClient, "owner", "repo", "model.bin", "main", t.TempDir())
	require.Error(t, err)
}

func TestGitHubClient_PullRecursive_SingleFile_OK(t *testing.T) {
	content := []byte("model file content")

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/contents/") {
			// GitHub API: return a single-file content object.
			downloadURL := srv.URL + "/files/model.bin"
			resp := map[string]interface{}{
				"type":         "file",
				"name":         "model.bin",
				"path":         "model.bin",
				"sha":          "deadbeef",
				"size":         len(content),
				"download_url": downloadURL,
				"url":          srv.URL + "/repos/owner/repo/contents/model.bin",
				"html_url":     srv.URL + "/owner/repo/blob/main/model.bin",
				"git_url":      srv.URL + "/repos/owner/repo/git/blobs/deadbeef",
				"_links":       map[string]string{},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		// File download endpoint.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	// Replace DefaultTransport so downloadFile (uses http.DefaultClient) reaches srv.
	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	baseURL, _ := url.Parse(srv.URL + "/")
	ghClient := github.NewClient(srv.Client())
	ghClient.BaseURL = baseURL

	c := &GitHubClient{}
	dest := t.TempDir()
	err := c.pullRecursive(context.Background(), ghClient, "owner", "repo", "model.bin", "main", dest)
	require.NoError(t, err)
}

func TestGitHubClient_PullRecursive_Directory_OK(t *testing.T) {
	content := []byte("file-in-dir content")

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/contents/") {
			// Return a directory listing (array).
			downloadURL := srv.URL + "/files/weights.bin"
			listing := []map[string]interface{}{
				{
					"type":         "file",
					"name":         "weights.bin",
					"path":         "models/weights.bin",
					"sha":          "abc",
					"size":         len(content),
					"download_url": downloadURL,
					"url":          srv.URL + "/repos/owner/repo/contents/models/weights.bin",
					"html_url":     "",
					"git_url":      "",
					"_links":       map[string]string{},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(listing)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	baseURL, _ := url.Parse(srv.URL + "/")
	ghClient := github.NewClient(srv.Client())
	ghClient.BaseURL = baseURL

	c := &GitHubClient{}
	err := c.pullRecursive(context.Background(), ghClient, "owner", "repo", "models", "main", t.TempDir())
	require.NoError(t, err)
}

// ============================================================================
// assembleChunks — error path: missing chunk file
// ============================================================================

func TestNewArtifactoryClient_OK(t *testing.T) {
	// NewArtifactoryClient only builds the config; it doesn't make network calls.
	client, err := NewArtifactoryClient("https://art.example.com/artifactory/", "user", "pass")
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.manager)
}

func TestArtifactoryClient_Pull_InvalidURI_TooFewParts(t *testing.T) {
	c := &ArtifactoryClient{}
	// URI with only 2 parts after stripping scheme — needs host/repo/path (3 parts).
	err := c.Pull(context.Background(), "artifactory://host/repo", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Artifactory URI format")
}

func TestArtifactoryClient_Pull_NilManager_InvalidURI(t *testing.T) {
	c := &ArtifactoryClient{manager: nil}
	// Valid URI format but nil manager will try to create one from env and then attempt download.
	// With a nil manager and a fake host the env build should succeed, but DownloadFiles will fail.
	// We just verify no panic occurs and an error is returned.
	err := c.Pull(context.Background(), "artifactory://host.example.com/libs-release/path/to/model.bin", t.TempDir())
	// Either succeeds or returns error — we mainly guard against panics.
	require.Error(t, err)
}

// ============================================================================
// GSClient — Pull with invalid URI
// ============================================================================

func TestGSClient_Pull_InvalidURI(t *testing.T) {
	c := &GSClient{} // nil inner client
	err := c.Pull(context.Background(), "gs://nobucket", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GS URI format")
}

// TestNewGSClient_WithEmulator_OK covers NewGSClient constructor using the
// STORAGE_EMULATOR_HOST env var to bypass ADC requirements.
func TestNewGSClient_WithEmulator_OK(t *testing.T) {
	t.Setenv("STORAGE_EMULATOR_HOST", "localhost:19997")
	c, err := NewGSClient(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, c)
}

// TestGSClient_Pull_NilClient_CancelledContext covers the lazy-init path in Pull
// when c.client is nil. A pre-cancelled context exits immediately without retrying.
func TestGSClient_Pull_NilClient_CancelledContext(t *testing.T) {
	t.Setenv("STORAGE_EMULATOR_HOST", "127.0.0.1:19997")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()         // cancel before the call so GCS SDK exits without sleeping
	c := &GSClient{} // nil inner client — triggers lazy-init code path
	err := c.Pull(ctx, "gs://mybucket/models/", t.TempDir())
	// Any result is acceptable — we only need to exercise the lazy-init code path.
	assert.Error(t, err)
}

func TestModelpackClient_PullInternal_EmptyRef_Error(t *testing.T) {
	c := &ModelpackClient{}
	artifact := &ModelArtifact{
		Registry:   "registry.example.com",
		Repository: "mymodel",
		Reference:  "",
	}
	err := c.pullInternal(context.Background(), artifact, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Modelpack reference")
}

func TestParseModelpackURI_ValidTagURI(t *testing.T) {
	a, err := ParseModelpackURI("modelpack://registry.example.com/org/mymodel:v1.0")
	require.NoError(t, err)
	assert.Equal(t, "registry.example.com", a.Registry)
	assert.Equal(t, "org/mymodel", a.Repository)
	assert.Equal(t, "v1.0", a.Reference)
}

func TestParseModelpackURI_NotModelpackURI(t *testing.T) {
	_, err := ParseModelpackURI("oci://registry.example.com/repo:tag")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a Modelpack URI")
}

func TestParseModelpackURI_TooFewParts(t *testing.T) {
	_, err := ParseModelpackURI("modelpack://onlyregistry")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Modelpack URI")
}

// TestModelpackClient_Push_NetworkError covers the Push function's error path
// when the target registry is unreachable.
