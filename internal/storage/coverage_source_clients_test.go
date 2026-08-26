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
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v62/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3Client_Pull_InvalidURI verifies error on malformed URI.
func TestGitHubClient_Pull_InvalidURI(t *testing.T) {
	c := &GitHubClient{client: github.NewClient(nil)}
	err := c.Pull(context.Background(), "github://owner/repo", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GitHub URI format")
}

// TestGitHubClient_Pull_WithToken_InvalidURI exercises the branch where GITHUB_TOKEN
// is set (switches to an authenticated client) but then the URI is invalid.
func TestGitHubClient_Pull_WithToken_InvalidURI(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_testtoken123")
	c := &GitHubClient{client: github.NewClient(nil)}
	err := c.Pull(context.Background(), "github://owner/repo", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GitHub URI format")
}

// TestGitHubClient_DownloadFile_ServerError exercises the non-200 error branch.
func TestGitHubClient_DownloadFile_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &GitHubClient{}
	err := c.downloadFile(context.Background(), srv.URL+"/model.bin", filepath.Join(t.TempDir(), "out.bin"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

// TestGitHubClient_PullRecursive_SingleFile_OK_WithDownload covers the single-file
// path including a real downloadFile call to the httptest server.
func TestGitHubClient_PullRecursive_SingleFile_OK_WithDownload(t *testing.T) {
	srv, ghClient := newGitHubSingleFileServer(t, "model.bin", "model-data-v2")
	defer srv.Close()
	destDir := t.TempDir()
	err := (&GitHubClient{}).pullRecursive(context.Background(), ghClient, "owner", "repo", "model.bin", "main", destDir)
	require.NoError(t, err)
	data, readErr := os.ReadFile(filepath.Join(destDir, "model.bin"))
	require.NoError(t, readErr)
	assert.Equal(t, "model-data-v2", string(data))
}

// ============================================================================
// GitLab Pull — exercises the "with revision" branch
// ============================================================================

// TestGitLabClient_Pull_WithRevision_InvalidFormat verifies that Pull parses the
// @revision suffix correctly before hitting the URI format check.
func TestGitLabClient_Pull_WithRevision_InvalidFormat(t *testing.T) {
	c := &GitLabClient{}
	err := c.Pull(context.Background(), "gitlab://onlyonepart@v1.0", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GitLab URI format")
}

// ============================================================================
// VaultClient — exercises InjectVaultSecrets empty-path early return
// ============================================================================

func TestInjectVaultSecrets_EmptyPath_ReturnsNil(t *testing.T) {
	err := InjectVaultSecrets(context.Background(), "")
	require.NoError(t, err)
}

func newGitHubSingleFileServer(t *testing.T, name, content string) (*httptest.Server, *github.Client) {
	t.Helper()
	var downloadURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/repos/owner/repo/contents/"+name {
			entry := map[string]interface{}{"type": "file", "name": name, "path": name, "download_url": downloadURL, "size": len(content)}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(entry)
			return
		}
		if r.URL.Path == "/download/"+name {
			_, _ = w.Write([]byte(content))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	downloadURL = srv.URL + "/download/" + name
	baseURL, _ := url.Parse(srv.URL + "/api/v3/")
	client := github.NewClient(srv.Client())
	client.BaseURL = baseURL
	return srv, client
}

func newGitHubDirectoryServer(t *testing.T) (*httptest.Server, *github.Client) {
	t.Helper()
	const content = "directory-file-content"
	var downloadURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/repos/owner/repo/contents/models" {
			entry := []map[string]interface{}{{"type": "file", "name": "weights.bin", "path": "models/weights.bin", "download_url": downloadURL, "size": len(content)}}
			_ = json.NewEncoder(w).Encode(entry)
			return
		}
		if r.URL.Path == "/download/weights.bin" {
			_, _ = w.Write([]byte(content))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	downloadURL = srv.URL + "/download/weights.bin"
	baseURL, _ := url.Parse(srv.URL + "/api/v3/")
	client := github.NewClient(srv.Client())
	client.BaseURL = baseURL
	return srv, client
}

// ============================================================================
// interface.go — GetClient unknown scheme
// ============================================================================

func TestGitHubClient_PullRecursive_DirectoryWithFile(t *testing.T) {
	srv, ghClient := newGitHubDirectoryServer(t)
	defer srv.Close()
	destDir := t.TempDir()
	err := (&GitHubClient{}).pullRecursive(context.Background(), ghClient, "owner", "repo", "models", "main", destDir)
	require.NoError(t, err)
	data, readErr := os.ReadFile(filepath.Join(destDir, "models", "weights.bin"))
	require.NoError(t, readErr)
	assert.Equal(t, "directory-file-content", string(data))
}

// ============================================================================
// NewGitLabClient
// ============================================================================

// TestNewGitLabClient_EmptyToken_OK verifies that NewGitLabClient succeeds with an empty token.
