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
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3Client_Pull_InvalidURI verifies error on malformed URI.
func TestComputeSHA256Stream_ReadError(t *testing.T) {
	_, err := ComputeSHA256Stream(&errReader{msg: "io failure"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "io failure")
}

// ============================================================================
// downloadSimple — bad URL (network error)
// ============================================================================

func TestDownloadSimple_NetworkError(t *testing.T) {
	d := &ParallelDownloader{HTTPClient: &http.Client{}}
	err := d.downloadSimple(context.Background(), "http://127.0.0.1:19994/file", filepath.Join(t.TempDir(), "out.bin"))
	require.Error(t, err)
}

// ============================================================================
// downloadChunk — extra coverage
// ============================================================================

func TestDownloadChunk_PartialContent_OK(t *testing.T) {
	const body = "0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := chunkDescriptor{Index: 0, Start: 0, End: 9, Path: filepath.Join(dir, "chunk_0")}
	var counter atomic.Int64
	d := &ParallelDownloader{HTTPClient: srv.Client()}
	err := d.downloadChunk(context.Background(), srv.URL+"/file", c, &counter)
	require.NoError(t, err)
	assert.Equal(t, int64(10), counter.Load())
}

func TestDownloadChunk_UnexpectedStatus_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := chunkDescriptor{Index: 0, Start: 0, End: 9, Path: filepath.Join(dir, "chunk_0")}
	var counter atomic.Int64
	d := &ParallelDownloader{HTTPClient: srv.Client()}
	err := d.downloadChunk(context.Background(), srv.URL+"/file", c, &counter)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status")
}

// ============================================================================
// isInsecureRegistry — env flag branch
// ============================================================================

func TestDownloadFile_ValidSHA256_Success(t *testing.T) {
	const content = "verified-content"

	// Pre-compute the correct sha256 for the content.
	expectedHash := sha256Hex([]byte(content))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "verified.bin")
	d := &ParallelDownloader{
		Workers:    1,
		ChunkSize:  DefaultChunkSize,
		HTTPClient: srv.Client(),
	}
	require.NoError(t, d.DownloadFile(context.Background(), srv.URL+"/file", dest, expectedHash))

	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

// ============================================================================
// GitHub pullRecursive — directory with sub-directory recursion
// ============================================================================

// TestGitHubClient_PullRecursive_DirectoryWithFile exercises the directory branch
// where the API returns a list entry with type "file" (not "dir") and downloads it.
func TestComputeSHA256_FileNotFound_Error(t *testing.T) {
	_, err := ComputeSHA256("/nonexistent/path/to/file.bin")
	require.Error(t, err)
}

func TestComputeSHA256_ValidFile_OK(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.bin")
	require.NoError(t, os.WriteFile(p, []byte("test"), 0644))
	h, err := ComputeSHA256(p)
	require.NoError(t, err)
	assert.Equal(t, sha256Hex([]byte("test")), h)
}

// ============================================================================
// VerifyFile — success path
// ============================================================================

func TestVerifyFile_Match_OK(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.bin")
	require.NoError(t, os.WriteFile(p, []byte("check"), 0644))
	require.NoError(t, VerifyFile(p, sha256Hex([]byte("check"))))
}

// ============================================================================
// S3 — additional Pull branches
// ============================================================================

// TestS3Client_Pull_InvalidURI_NoBucket covers the case when ref has only 1 part.
