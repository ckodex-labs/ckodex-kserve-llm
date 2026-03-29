/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- NewParallelDownloader -------------------------------------------------

func TestNewParallelDownloader_Defaults(t *testing.T) {
	d := NewParallelDownloader("tok")
	require.NotNil(t, d)
	assert.Equal(t, DefaultDownloadWorkers, d.Workers)
	assert.Equal(t, DefaultChunkSize, d.ChunkSize)
	assert.Equal(t, "tok", d.Token)
}

func TestNewParallelDownloader_EnvWorkers(t *testing.T) {
	t.Setenv("STORAGE_DOWNLOAD_WORKERS", "8")
	d := NewParallelDownloader("")
	assert.Equal(t, 8, d.Workers)
}

func TestNewParallelDownloader_EnvWorkersInvalid_UsesDefault(t *testing.T) {
	t.Setenv("STORAGE_DOWNLOAD_WORKERS", "not-a-number")
	d := NewParallelDownloader("")
	assert.Equal(t, DefaultDownloadWorkers, d.Workers)
}

func TestNewParallelDownloader_EnvWorkersZero_UsesDefault(t *testing.T) {
	t.Setenv("STORAGE_DOWNLOAD_WORKERS", "0")
	d := NewParallelDownloader("")
	assert.Equal(t, DefaultDownloadWorkers, d.Workers)
}

func TestNewParallelDownloader_EnvWorkersTooHigh_UsesDefault(t *testing.T) {
	t.Setenv("STORAGE_DOWNLOAD_WORKERS", "100")
	d := NewParallelDownloader("")
	assert.Equal(t, DefaultDownloadWorkers, d.Workers)
}

func TestNewParallelDownloader_EnvChunkSize(t *testing.T) {
	t.Setenv("STORAGE_CHUNK_SIZE_MB", "128")
	d := NewParallelDownloader("")
	assert.Equal(t, int64(128*1024*1024), d.ChunkSize)
}

func TestNewParallelDownloader_EnvChunkSizeInvalid_UsesDefault(t *testing.T) {
	t.Setenv("STORAGE_CHUNK_SIZE_MB", "bad")
	d := NewParallelDownloader("")
	assert.Equal(t, DefaultChunkSize, d.ChunkSize)
}

// ---- ComputeSHA256Stream ---------------------------------------------------

func TestComputeSHA256Stream_KnownValue(t *testing.T) {
	content := []byte("streaming checksum test")
	got, err := ComputeSHA256Stream(bytes.NewReader(content))
	require.NoError(t, err)
	assert.Equal(t, sha256Hex(content), got)
}

func TestComputeSHA256Stream_EmptyReader(t *testing.T) {
	got, err := ComputeSHA256Stream(bytes.NewReader(nil))
	require.NoError(t, err)
	assert.Equal(t, sha256Hex([]byte{}), got)
}

// ---- DownloadFile — small file (no range) -----------------------------------

func TestDownloadFile_SimpleDownload_NoSHA(t *testing.T) {
	content := []byte("small model config")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			// No Content-Length → unknown size → simple download
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "config.json")
	d := &ParallelDownloader{
		Workers:    1,
		ChunkSize:  DefaultChunkSize,
		HTTPClient: srv.Client(),
	}

	require.NoError(t, d.DownloadFile(context.Background(), srv.URL+"/file", dest, ""))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestDownloadFile_SimpleDownload_SHA256Mismatch_Error(t *testing.T) {
	content := []byte("data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	d := &ParallelDownloader{
		Workers:    1,
		ChunkSize:  DefaultChunkSize,
		HTTPClient: srv.Client(),
	}

	err := d.DownloadFile(context.Background(), srv.URL+"/file", dest, "deadbeef")
	require.Error(t, err)
}

func TestDownloadFile_SmallFile_WithContentLength(t *testing.T) {
	content := []byte("tiny")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "tiny.bin")
	d := &ParallelDownloader{
		Workers:    2,
		ChunkSize:  DefaultChunkSize,
		HTTPClient: srv.Client(),
	}

	require.NoError(t, d.DownloadFile(context.Background(), srv.URL+"/file", dest, ""))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestDownloadFile_WithToken_SetsAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "f.bin")
	d := &ParallelDownloader{
		Workers:    1,
		ChunkSize:  DefaultChunkSize,
		HTTPClient: srv.Client(),
		Token:      "secret-token",
	}

	require.NoError(t, d.DownloadFile(context.Background(), srv.URL+"/file", dest, ""))
	assert.Contains(t, gotAuth, "Bearer secret-token")
}

func TestDownloadFile_ServerError_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "f.bin")
	d := &ParallelDownloader{
		Workers:    1,
		ChunkSize:  DefaultChunkSize,
		HTTPClient: srv.Client(),
	}

	err := d.DownloadFile(context.Background(), srv.URL+"/file", dest, "")
	require.Error(t, err)
}

func TestDownloadFile_Unreachable_Error(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "f.bin")
	d := &ParallelDownloader{
		Workers:    1,
		ChunkSize:  DefaultChunkSize,
		HTTPClient: &http.Client{},
	}

	err := d.DownloadFile(context.Background(), "http://127.0.0.1:19995/file", dest, "")
	require.Error(t, err)
}
