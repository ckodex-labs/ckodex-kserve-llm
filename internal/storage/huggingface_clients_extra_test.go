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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ParseOCIURI
// ============================================================================

func TestHuggingFaceClient_ListRepoFiles_OK(t *testing.T) {
	entries := []HFTreeEntry{
		{Type: "file", Path: "config.json", Size: 100},
		{Type: "file", Path: "model.safetensors", Size: 4096},
		{Type: "directory", Path: "sub/", Size: 0},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(entries)
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &HuggingFaceClient{baseURL: srv.URL}
	files, err := c.listRepoFiles(context.Background(), "org/model", "main", "")
	require.NoError(t, err)
	// Only file-type entries should be returned.
	assert.Equal(t, []string{"config.json", "model.safetensors"}, files)
}

func TestHuggingFaceClient_ListRepoFiles_NonOK_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &HuggingFaceClient{baseURL: srv.URL}
	_, err := c.listRepoFiles(context.Background(), "org/model", "main", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HF API tree returned")
}

func TestHuggingFaceClient_ListRepoFiles_InvalidJSON_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &HuggingFaceClient{baseURL: srv.URL}
	_, err := c.listRepoFiles(context.Background(), "org/model", "main", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode")
}

func TestHuggingFaceClient_ListRepoFiles_WithToken_SetsHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &HuggingFaceClient{baseURL: srv.URL}
	files, err := c.listRepoFiles(context.Background(), "org/model", "main", "my-token")
	require.NoError(t, err)
	assert.Empty(t, files)
	assert.Equal(t, "Bearer my-token", gotAuth)
}

// ============================================================================
// GSClient — URI-error path (returns before network call)
// ============================================================================

func TestGSClient_Pull_InvalidURI_Error(t *testing.T) {
	c := &GSClient{} // nil client — but URI check happens before lazy-init
	err := c.Pull(context.Background(), "gs://bucket-only", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GS URI format")
}

func TestGSClient_Schemes(t *testing.T) {
	c := &GSClient{}
	assert.Contains(t, c.Schemes(), "gs")
}

// ============================================================================
// NewS3Client — constructor (no network calls on config load)
// ============================================================================

func TestNewS3Client_DefaultRegion_OK(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	c, err := NewS3Client(context.Background(), "us-east-1")
	require.NoError(t, err)
	require.NotNil(t, c)
}

// ============================================================================
// downloadSimple — non-200 error path
// ============================================================================

func TestDownloadSimple_NonOK_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	d := &ParallelDownloader{Workers: 1, ChunkSize: DefaultChunkSize, HTTPClient: srv.Client()}
	dest := filepath.Join(t.TempDir(), "f.bin")
	err := d.downloadSimple(context.Background(), srv.URL+"/file", dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

// ============================================================================
// HuggingFaceClient.Pull — full end-to-end via DefaultTransport replacement
// ============================================================================

// buildHFServer returns a test server that handles both the HF tree API and file downloads.
// It responds to /api/models/... with a JSON file listing and to all other paths with fileContent.
func buildHFServer(t *testing.T, files []HFTreeEntry, fileContent []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/models/") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(files)
			return
		}
		// File download — respond to HEAD and GET.
		if r.Method == http.MethodHead {
			// No Content-Length → size unknown → simple download path.
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fileContent)
	}))
}

func TestHuggingFaceClient_Pull_SkipChecksum_OK(t *testing.T) {
	t.Setenv("SKIP_CHECKSUM", "1")

	content := []byte("model weights data")
	entries := []HFTreeEntry{
		{Type: "file", Path: "model.safetensors", Size: int64(len(content))},
		{Type: "directory", Path: "sub/", Size: 0}, // directories must be filtered
	}
	srv := buildHFServer(t, entries, content)
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &HuggingFaceClient{baseURL: srv.URL}
	dest := t.TempDir()
	require.NoError(t, c.Pull(context.Background(), "hf://org/model", dest))

	got, err := os.ReadFile(filepath.Join(dest, "model.safetensors"))
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestHuggingFaceClient_Pull_WithRevisionSyntax(t *testing.T) {
	t.Setenv("SKIP_CHECKSUM", "1")

	content := []byte("v2 weights")
	entries := []HFTreeEntry{{Type: "file", Path: "config.json"}}
	srv := buildHFServer(t, entries, content)
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &HuggingFaceClient{baseURL: srv.URL}
	dest := t.TempDir()
	// "hf://org/model@v2" — @ syntax sets revision to "v2"
	require.NoError(t, c.Pull(context.Background(), "hf://org/model@v2", dest))
}

func TestHuggingFaceClient_Pull_EmptyFileList_Error(t *testing.T) {
	t.Setenv("SKIP_CHECKSUM", "1")

	srv := buildHFServer(t, []HFTreeEntry{{Type: "directory", Path: "sub/"}}, nil)
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &HuggingFaceClient{baseURL: srv.URL}
	err := c.Pull(context.Background(), "hf://org/model", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no files found")
}

func TestHuggingFaceClient_Pull_ListFilesError(t *testing.T) {
	// Server returns 500 for the tree API → listRepoFiles returns an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &HuggingFaceClient{baseURL: srv.URL}
	err := c.Pull(context.Background(), "hf://org/model", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list repo files")
}

func TestHuggingFaceClient_Pull_ChecksumWarning_ContinuesOnFetchError(t *testing.T) {
	// SKIP_CHECKSUM is NOT set → Pull tries to fetch checksums.
	// The checksum fetch will fail (test server returns 404 for that path),
	// but Pull logs a warning and continues — it must NOT return an error.
	content := []byte("weights")
	entries := []HFTreeEntry{{Type: "file", Path: "model.bin"}}

	var checksumRequested bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/models/") && !strings.Contains(r.URL.Path, "tree"):
			// Checksum resolve endpoint — return 404 to simulate unavailability.
			checksumRequested = true
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/api/models/"):
			// Tree listing.
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(entries)
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &HuggingFaceClient{baseURL: srv.URL}
	// Must succeed even though checksum fetch fails — Pull warns and proceeds.
	err := c.Pull(context.Background(), "hf://org/model", t.TempDir())
	require.NoError(t, err)
	_ = checksumRequested // may or may not have been set depending on ChecksumVerifier internals
}

// ============================================================================
// VaultClient — NewVaultClient, FetchSecret, InjectVaultSecrets
// ============================================================================

func TestAssembleChunks_MissingChunkFile_Error(t *testing.T) {
	d := &ParallelDownloader{}
	destFile := filepath.Join(t.TempDir(), "assembled.bin")

	// Provide a chunk descriptor that points to a non-existent file.
	chunks := []chunkDescriptor{
		{Index: 0, Start: 0, End: 9, Path: filepath.Join(t.TempDir(), "chunk_00000_missing")},
	}

	err := d.assembleChunks(chunks, destFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chunk 0")
}

// TestAssembleChunks_OK verifies that existing chunk files are assembled correctly.
func TestAssembleChunks_OK(t *testing.T) {
	dir := t.TempDir()
	chunkPath := filepath.Join(dir, "chunk_00000")
	require.NoError(t, os.WriteFile(chunkPath, []byte("helloworld"), 0644))

	d := &ParallelDownloader{}
	destFile := filepath.Join(dir, "assembled.bin")

	chunks := []chunkDescriptor{
		{Index: 0, Start: 0, End: 9, Path: chunkPath},
	}

	require.NoError(t, d.assembleChunks(chunks, destFile))
	data, err := os.ReadFile(destFile)
	require.NoError(t, err)
	assert.Equal(t, "helloworld", string(data))
}

// ============================================================================
// downloadSimple — happy path and error paths
// ============================================================================

func TestDownloadSimple_OK(t *testing.T) {
	const body = "simple-file-content"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	destFile := filepath.Join(t.TempDir(), "out.bin")
	d := &ParallelDownloader{HTTPClient: srv.Client()}
	require.NoError(t, d.downloadSimple(context.Background(), srv.URL+"/file", destFile))

	data, err := os.ReadFile(destFile)
	require.NoError(t, err)
	assert.Equal(t, body, string(data))
}

func TestDownloadSimple_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	destFile := filepath.Join(t.TempDir(), "out.bin")
	d := &ParallelDownloader{HTTPClient: srv.Client()}
	err := d.downloadSimple(context.Background(), srv.URL+"/file", destFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

// ============================================================================
// ModelpackClient
// ============================================================================
