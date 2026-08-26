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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ParseOCIURI
// ============================================================================

func TestModelpackClient_Push_NetworkError(t *testing.T) {
	c := &ModelpackClient{}
	artifact := &ModelArtifact{
		Registry:   "localhost:19998", // unreachable
		Repository: "mymodel",
		Reference:  "v1.0",
	}
	err := c.Push(context.Background(), artifact, t.TempDir())
	// Push will fail at b.Push due to unreachable registry.
	require.Error(t, err)
}

// ============================================================================
// OCIClient.pullInternal — covers path beyond empty-ref guard
// ============================================================================

// TestOCIClient_PullInternal_NetworkError covers the pullInternal code path
// beyond the empty-reference guard, through MkdirAll, NewRepository, and
// auth setup, failing at oras.Copy on an unreachable registry.
func TestOCIClient_PullInternal_NetworkError(t *testing.T) {
	c := &OCIClient{}
	artifact := &ModelArtifact{
		Registry:   "localhost:19999", // unreachable plain-HTTP insecure registry
		Repository: "mymodel",
		Reference:  "v1.0",
		RawURI:     "oci://localhost:19999/mymodel:v1.0",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := c.pullInternal(ctx, artifact, t.TempDir())
	require.Error(t, err)
}

// TestOCIClient_Push_NetworkError covers OCIClient.Push beyond the empty-ref guard.
func TestOCIClient_Push_NetworkError(t *testing.T) {
	c := &OCIClient{}
	artifact := &ModelArtifact{
		Registry:   "localhost:19999", // unreachable
		Repository: "mymodel",
		Reference:  "v1.0",
		RawURI:     "oci://localhost:19999/mymodel:v1.0",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := c.Push(ctx, artifact, t.TempDir())
	require.Error(t, err)
}

// ============================================================================
// downloadChunk — short write path
// ============================================================================

// TestDownloadChunk_ShortWrite_Error exercises the short-write detection branch.
// The server returns fewer bytes than the requested range.
func TestDownloadChunk_ShortWrite_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond with partial content but only 3 bytes — range requests 0-9 (10 bytes).
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("abc")) // only 3 bytes, expected 10
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := chunkDescriptor{Index: 0, Start: 0, End: 9, Path: filepath.Join(dir, "chunk_0")}
	var counter atomic.Int64
	d := &ParallelDownloader{HTTPClient: srv.Client()}
	err := d.downloadChunk(context.Background(), srv.URL+"/file", c, &counter)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "short write")
}

// ============================================================================
// DownloadFile — probe failure path
// ============================================================================

// TestDownloadFile_ProbeFailure exercises the probe-failed error branch.
func TestDownloadFile_ProbeFailure(t *testing.T) {
	d := &ParallelDownloader{HTTPClient: &http.Client{}}
	err := d.DownloadFile(context.Background(), "http://127.0.0.1:19994/file", filepath.Join(t.TempDir(), "out.bin"), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe failed")
}

// TestDownloadFile_ChecksumMismatch_Error exercises the checksum mismatch branch
// which removes the downloaded file on hash failure.
func TestDownloadFile_ChecksumMismatch_Error(t *testing.T) {
	const content = "some-model-data"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	d := &ParallelDownloader{
		Workers:    1,
		ChunkSize:  DefaultChunkSize,
		HTTPClient: srv.Client(),
	}
	err := d.DownloadFile(context.Background(), srv.URL+"/file", dest, "0000000000000000000000000000000000000000000000000000000000000000")
	require.Error(t, err)
	// File should have been removed on checksum failure.
	_, statErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr))
}

// ============================================================================
// seaweedfs_client — Download request-creation error
// ============================================================================

// TestSeaweedFSClient_Download_RequestError exercises the http.NewRequestWithContext
// error branch by passing an invalid URL.
