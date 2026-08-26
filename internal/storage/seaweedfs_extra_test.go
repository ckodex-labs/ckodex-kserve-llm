/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ParseOCIURI
// ============================================================================

func newTestSwFS(t *testing.T, srvURL string, c *http.Client) *SeaweedFSClient {
	t.Helper()
	return &SeaweedFSClient{
		Config: SeaweedFSConfig{
			FilerURL: srvURL,
			BasePath: "/models",
		},
		client: c,
	}
}

func TestSeaweedFSClient_Download_OK(t *testing.T) {
	content := []byte("weights data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	cl := newTestSwFS(t, srv.URL, srv.Client())
	require.NoError(t, cl.Download(context.Background(), "/llama3/model.bin", dest))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestSeaweedFSClient_Download_NonOK_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	cl := newTestSwFS(t, srv.URL, srv.Client())
	err := cl.Download(context.Background(), "/nope.bin", filepath.Join(t.TempDir(), "out.bin"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download failed with status 404")
}

func TestSeaweedFSClient_Upload_OK(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Write a temp local file to upload.
	local := filepath.Join(t.TempDir(), "model.bin")
	require.NoError(t, os.WriteFile(local, []byte("data"), 0600))

	cl := newTestSwFS(t, srv.URL, srv.Client())
	require.NoError(t, cl.Upload(context.Background(), local, "/llama3/model.bin"))
	assert.Equal(t, http.MethodPost, gotMethod)
}

func TestSeaweedFSClient_Upload_NonOK_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	local := filepath.Join(t.TempDir(), "model.bin")
	require.NoError(t, os.WriteFile(local, []byte("data"), 0600))

	cl := newTestSwFS(t, srv.URL, srv.Client())
	err := cl.Upload(context.Background(), local, "/llama3/model.bin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload failed with status 500")
}

func TestSeaweedFSClient_Upload_MissingLocalFile_Error(t *testing.T) {
	cl := newTestSwFS(t, "http://unused", &http.Client{})
	err := cl.Upload(context.Background(), "/nonexistent/file.bin", "/remote/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open local file")
}

func TestSeaweedFSClient_Delete_OK(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusNoContent, http.StatusAccepted} {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			}))
			defer srv.Close()
			cl := newTestSwFS(t, srv.URL, srv.Client())
			require.NoError(t, cl.Delete(context.Background(), "/llama3/model.bin"))
		})
	}
}

func TestSeaweedFSClient_Delete_Error_Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()
	cl := newTestSwFS(t, srv.URL, srv.Client())
	err := cl.Delete(context.Background(), "/llama3/model.bin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed with status 403")
}

func TestSeaweedFSClient_Exists_True(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cl := newTestSwFS(t, srv.URL, srv.Client())
	exists, err := cl.Exists(context.Background(), "/llama3/model.bin")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSeaweedFSClient_Exists_False(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	cl := newTestSwFS(t, srv.URL, srv.Client())
	exists, err := cl.Exists(context.Background(), "/llama3/nope.bin")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestSeaweedFSClient_Pull_OK(t *testing.T) {
	content := []byte("model data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	// Pull expects a seaweedfs:// URI, but internally calls Download.
	// We point the FilerURL to the test server and use a relative path URI.
	cl := &SeaweedFSClient{
		Config: SeaweedFSConfig{FilerURL: srv.URL, BasePath: ""},
		client: srv.Client(),
	}

	// Construct a URI that ParseSeaweedFSURI can parse:
	// seaweedfs://host:port/path — host:port must match srv's address.
	host := strings.TrimPrefix(srv.URL, "http://")
	uri := "seaweedfs://" + host + "/model.bin"
	cl.Config.FilerURL = srv.URL

	require.NoError(t, cl.Pull(context.Background(), uri, dest))
}

func TestSeaweedFSClient_Pull_InvalidURI_Error(t *testing.T) {
	cl := &SeaweedFSClient{Config: DefaultSeaweedFSConfig(), client: &http.Client{}}
	err := cl.Pull(context.Background(), "not-a-seaweedfs-uri", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a SeaweedFS URI")
}

// ============================================================================
// HuggingFaceClient constructors
// ============================================================================

func TestSeaweedFSClient_Download_RequestError(t *testing.T) {
	cl := &SeaweedFSClient{
		Config: SeaweedFSConfig{FilerURL: "://bad-url", BasePath: ""},
		client: &http.Client{},
	}
	err := cl.Download(context.Background(), "/path", filepath.Join(t.TempDir(), "out.bin"))
	require.Error(t, err)
}

// TestSeaweedFSClient_Upload_RequestError exercises the upload request-creation
// error branch with an invalid URL.
func TestSeaweedFSClient_Upload_RequestError(t *testing.T) {
	dir := t.TempDir()
	localFile := filepath.Join(dir, "model.bin")
	require.NoError(t, os.WriteFile(localFile, []byte("data"), 0644))

	cl := &SeaweedFSClient{
		Config: SeaweedFSConfig{FilerURL: "://bad-url", BasePath: ""},
		client: &http.Client{},
	}
	err := cl.Upload(context.Background(), localFile, "/path")
	require.Error(t, err)
}

// TestSeaweedFSClient_Delete_RequestError exercises the delete request-creation
// error branch with an invalid URL.
func TestSeaweedFSClient_Delete_RequestError(t *testing.T) {
	cl := &SeaweedFSClient{
		Config: SeaweedFSConfig{FilerURL: "://bad-url", BasePath: ""},
		client: &http.Client{},
	}
	err := cl.Delete(context.Background(), "/path")
	require.Error(t, err)
}

// TestSeaweedFSClient_Exists_RequestError exercises the exists request-creation
// error branch with an invalid URL.
func TestSeaweedFSClient_Exists_RequestError(t *testing.T) {
	cl := &SeaweedFSClient{
		Config: SeaweedFSConfig{FilerURL: "://bad-url", BasePath: ""},
		client: &http.Client{},
	}
	_, err := cl.Exists(context.Background(), "/path")
	require.Error(t, err)
}

// ============================================================================
// InjectVaultSecrets — NewVaultClient error branch
// ============================================================================

// TestInjectVaultSecrets_NonEmptyPath_AttemptsFetch exercises the code path
// where InjectVaultSecrets proceeds past the empty-path guard and creates a
// VaultClient, then attempts to fetch a secret from an unreachable Vault.
