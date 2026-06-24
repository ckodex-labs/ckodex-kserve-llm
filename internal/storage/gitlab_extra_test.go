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
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// GitLabClient — pullRecursive
// ============================================================================

// TestGitLabClient_PullRecursive_APIError exercises the error branch when
// GetFile returns a non-404 client error.
func TestGitLabClient_PullRecursive_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad request"}`))
	}))
	defer srv.Close()

	client, err := gitlab.NewClient("", gitlab.WithBaseURL(srv.URL+"/"), gitlab.WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	c := &GitLabClient{client: client}
	err = c.pullRecursive(context.Background(), client, "42", "models/file.bin", "main", t.TempDir())
	assert.Error(t, err)
}

// TestGitLabClient_PullRecursive_SingleFile_OK exercises the happy path where
// GetFile returns success and GetRawFile returns the file bytes.
func TestGitLabClient_PullRecursive_SingleFile_OK(t *testing.T) {
	const fileContent = "model-weights-data"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// GetFile: GET /api/v4/projects/{id}/repository/files/{encoded_path}
		// GetRawFile: same path + "/raw"
		if strings.HasSuffix(path, "/raw") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fileContent))
			return
		}
		// GetFile response
		resp := map[string]interface{}{
			"file_name":      "file.bin",
			"file_path":      "models/file.bin",
			"size":           len(fileContent),
			"encoding":       "base64",
			"content":        "bW9kZWwtd2VpZ2h0cy1kYXRh",
			"content_sha256": "abc",
			"ref":            "main",
			"blob_id":        "deadbeef",
			"commit_id":      "cafecafe",
			"last_commit_id": "cafecafe",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := gitlab.NewClient("", gitlab.WithBaseURL(srv.URL+"/"), gitlab.WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	destDir := t.TempDir()
	c := &GitLabClient{client: client}
	err = c.pullRecursive(context.Background(), client, "42", "models/file.bin", "main", destDir)
	require.NoError(t, err)

	// Verify the file was written.
	data, readErr := os.ReadFile(filepath.Join(destDir, "file.bin"))
	require.NoError(t, readErr)
	assert.Equal(t, fileContent, string(data))
}

// TestGitLabClient_PullRecursive_Directory_OK exercises the directory branch
// where GetFile returns 404 and ListTree returns a tree of blobs.
func TestGitLabClient_PullRecursive_Directory_OK(t *testing.T) {
	const blobContent = "blob-data"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GetRawFile for a specific blob path
		if strings.Contains(path, "/repository/files/") && strings.HasSuffix(path, "/raw") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(blobContent))
			return
		}

		// GetFile → 404 to trigger directory listing
		if strings.Contains(path, "/repository/files/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 File Not Found"}`))
			return
		}

		// ListTree → return one blob
		if strings.Contains(path, "/repository/tree") {
			tree := []map[string]interface{}{
				{
					"id":   "abc123",
					"name": "weights.bin",
					"type": "blob",
					"path": "models/weights.bin",
					"mode": "100644",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Total-Pages", "1")
			w.Header().Set("X-Page", "1")
			w.Header().Set("X-Per-Page", "20")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(tree)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client, err := gitlab.NewClient("", gitlab.WithBaseURL(srv.URL+"/"), gitlab.WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	destDir := t.TempDir()
	c := &GitLabClient{client: client}
	err = c.pullRecursive(context.Background(), client, "42", "models", "main", destDir)
	require.NoError(t, err)

	// Verify the blob was written.
	data, readErr := os.ReadFile(filepath.Join(destDir, "models", "weights.bin"))
	require.NoError(t, readErr)
	assert.Equal(t, blobContent, string(data))
}

// TestGitLabClient_Pull_InvalidURI_TooFewParts verifies that Pull returns an
// error when the URI has fewer than 2 path segments.
func TestGitLabClient_Pull_InvalidURI_TooFewParts(t *testing.T) {
	c := &GitLabClient{}
	// Set GITLAB_TOKEN to empty to avoid env side-effects; NewClient("") succeeds.
	err := c.Pull(context.Background(), "gitlab://onlyonepart", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GitLab URI format")
}
