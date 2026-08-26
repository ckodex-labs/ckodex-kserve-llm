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
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ParseOCIURI
// ============================================================================

func TestNewHuggingFaceClient_DefaultBaseURL(t *testing.T) {
	c := NewHuggingFaceClient("tok")
	assert.Equal(t, defaultHFBaseURL, c.baseURL)
}

func TestNewHuggingFaceClientWithMirror_CustomURL(t *testing.T) {
	c := NewHuggingFaceClientWithMirror("tok", "https://hf-mirror.internal/")
	assert.Equal(t, "https://hf-mirror.internal", c.baseURL, "trailing slash must be stripped")
}

func TestNewHuggingFaceClientWithMirror_EmptyMirror_UsesDefault(t *testing.T) {
	c := NewHuggingFaceClientWithMirror("tok", "")
	assert.Equal(t, defaultHFBaseURL, c.baseURL)
}

func TestHuggingFaceClient_Schemes(t *testing.T) {
	c := NewHuggingFaceClient("")
	schemes := c.Schemes()
	assert.Contains(t, schemes, "hf")
	assert.Contains(t, schemes, "huggingface")
}

// ============================================================================
// downloadChunked (white-box: same package)
// ============================================================================

// buildRangeServer creates an httptest server that serves content with byte-range support.
func buildRangeServer(t *testing.T, content []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)
			return
		}
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
		// Parse "bytes=start-end".
		rangeHeader = strings.TrimPrefix(rangeHeader, "bytes=")
		parts := strings.SplitN(rangeHeader, "-", 2)
		start, _ := strconv.ParseInt(parts[0], 10, 64)
		end, _ := strconv.ParseInt(parts[1], 10, 64)
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
}

func TestDownloadChunked_SmallChunks_AssemblesCorrectly(t *testing.T) {
	content := []byte("0123456789ABCDEFGHIJ") // 20 bytes → 4 chunks of 5
	srv := buildRangeServer(t, content)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	d := &ParallelDownloader{
		Workers:    2,
		ChunkSize:  5,
		HTTPClient: srv.Client(),
	}

	require.NoError(t, d.downloadChunked(context.Background(), srv.URL+"/file", dest, int64(len(content))))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestDownloadChunked_SingleChunk_OK(t *testing.T) {
	content := []byte("hello chunked")
	srv := buildRangeServer(t, content)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "single.bin")
	d := &ParallelDownloader{
		Workers:    1,
		ChunkSize:  int64(len(content)) + 100, // larger than file → single chunk
		HTTPClient: srv.Client(),
	}

	require.NoError(t, d.downloadChunked(context.Background(), srv.URL+"/file", dest, int64(len(content))))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestDownloadChunked_ChunkServerError_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "err.bin")
	d := &ParallelDownloader{Workers: 1, ChunkSize: 5, HTTPClient: srv.Client()}
	err := d.downloadChunked(context.Background(), srv.URL+"/file", dest, 20)
	require.Error(t, err)
}

func TestDownloadChunked_ResumesExistingChunk(t *testing.T) {
	content := []byte("resume-me-content!!") // 19 bytes
	srv := buildRangeServer(t, content)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "resume.bin")
	chunksDir := dest + ".chunks"
	require.NoError(t, os.MkdirAll(chunksDir, 0o755))

	// Pre-write chunk_00000 (bytes 0-9, exactly 10 bytes) so it's skipped.
	chunk0Path := filepath.Join(chunksDir, "chunk_00000")
	require.NoError(t, os.WriteFile(chunk0Path, content[0:10], 0o600))

	d := &ParallelDownloader{Workers: 1, ChunkSize: 10, HTTPClient: srv.Client()}
	require.NoError(t, d.downloadChunked(context.Background(), srv.URL+"/file", dest, int64(len(content))))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// ============================================================================
// countingReader
// ============================================================================

func TestCountingReader_TracksBytes(t *testing.T) {
	var counter atomic.Int64
	data := []byte("hello counting")
	cr := &countingReader{r: strings.NewReader(string(data)), counter: &counter}

	buf := make([]byte, len(data))
	n, err := cr.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, int64(len(data)), counter.Load())
}

// ============================================================================
// InjectVaultSecrets — empty path is a no-op
// ============================================================================
