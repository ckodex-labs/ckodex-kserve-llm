/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNoTCP skips the test if TCP binding is unavailable (sandbox restriction).
func skipIfNoTCP(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("TCP binding unavailable in this environment: %v", err)
	}
	_ = ln.Close()
}

// ---- helpers ---------------------------------------------------------------

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "checksum-test-*.bin")
	require.NoError(t, err)
	_, err = f.Write(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ---- NewChecksumVerifier ---------------------------------------------------

func TestNewChecksumVerifier_DefaultBaseURL(t *testing.T) {
	v := NewChecksumVerifier("tok")
	assert.Equal(t, "https://huggingface.co", v.BaseURL)
	assert.Equal(t, "tok", v.Token)
}

func TestNewChecksumVerifierWithMirror_CustomURL(t *testing.T) {
	v := NewChecksumVerifierWithMirror("tok", "https://mirror.example.com/")
	// trailing slash stripped
	assert.Equal(t, "https://mirror.example.com", v.BaseURL)
}

func TestNewChecksumVerifierWithMirror_EmptyMirror_UsesDefault(t *testing.T) {
	v := NewChecksumVerifierWithMirror("", "")
	assert.Equal(t, "https://huggingface.co", v.BaseURL)
}

// ---- VerifyFile ------------------------------------------------------------

func TestVerifyFile_Match(t *testing.T) {
	content := []byte("hello model weights")
	path := writeTempFile(t, content)
	expected := sha256Hex(content)

	require.NoError(t, VerifyFile(path, expected))
}

func TestVerifyFile_Mismatch_Error(t *testing.T) {
	content := []byte("real content")
	path := writeTempFile(t, content)

	err := VerifyFile(path, "000000000000000000000000000000000000000000000000000000000000dead")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestVerifyFile_NotFound_Error(t *testing.T) {
	err := VerifyFile("/nonexistent/file.bin", "abc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot open")
}

func TestVerifyFile_EmptyFile(t *testing.T) {
	path := writeTempFile(t, []byte{})
	// SHA256 of empty byte slice
	expected := sha256Hex([]byte{})
	require.NoError(t, VerifyFile(path, expected))
}

func TestVerifyFile_CaseInsensitive(t *testing.T) {
	content := []byte("data")
	path := writeTempFile(t, content)
	// Uppercase expected — should still match
	upperExpected := strings.ToUpper(sha256Hex(content))
	require.NoError(t, VerifyFile(path, upperExpected))
}

// ---- ComputeSHA256 ---------------------------------------------------------

func TestComputeSHA256_KnownValue(t *testing.T) {
	content := []byte("deterministic content")
	path := writeTempFile(t, content)

	got, err := ComputeSHA256(path)
	require.NoError(t, err)
	assert.Equal(t, sha256Hex(content), got)
}

func TestComputeSHA256_NotFound_Error(t *testing.T) {
	_, err := ComputeSHA256("/does/not/exist.bin")
	require.Error(t, err)
}

// ---- VerifyDirectory -------------------------------------------------------

func TestVerifyDirectory_AllMatch(t *testing.T) {
	dir := t.TempDir()
	content := []byte("model shard 0")
	path := filepath.Join(dir, "shard-0.bin")
	require.NoError(t, os.WriteFile(path, content, 0600))

	checksums := map[string]string{
		"shard-0.bin": sha256Hex(content),
	}

	require.NoError(t, VerifyDirectory(dir, checksums))
}

func TestVerifyDirectory_OneMismatch_Error(t *testing.T) {
	dir := t.TempDir()
	content := []byte("real data")
	path := filepath.Join(dir, "shard-0.bin")
	require.NoError(t, os.WriteFile(path, content, 0600))

	checksums := map[string]string{
		"shard-0.bin": "0000000000000000000000000000000000000000000000000000000000000000",
	}

	err := VerifyDirectory(dir, checksums)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum verification failed")
}

func TestVerifyDirectory_MissingFile_Skipped(t *testing.T) {
	dir := t.TempDir()
	// The file listed in checksums does not exist — should be silently skipped
	checksums := map[string]string{
		"nonexistent.bin": sha256Hex([]byte("x")),
	}
	require.NoError(t, VerifyDirectory(dir, checksums))
}

func TestVerifyDirectory_EmptyChecksums_NoError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, VerifyDirectory(dir, map[string]string{}))
}

func TestVerifyDirectory_MultipleFiles_AllMatch(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"a.bin": []byte("content-a"),
		"b.bin": []byte("content-b"),
		"c.bin": []byte("content-c"),
	}
	checksums := make(map[string]string)
	for name, data := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0600))
		checksums[name] = sha256Hex(data)
	}
	require.NoError(t, VerifyDirectory(dir, checksums))
}

// ---- FetchFileChecksums (httptest) ----------------------------------------

func TestFetchFileChecksums_OK(t *testing.T) {
	skipIfNoTCP(t)
	info := HFModelInfo{
		Siblings: []HFFileInfo{
			{Filename: "model.safetensors", LFS: &HFLFSInfo{SHA256: "ABCDEF123456", Size: 1000}},
			{Filename: "config.json", LFS: nil}, // no LFS — should be omitted
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	}))
	defer srv.Close()

	v := &ChecksumVerifier{
		Token:      "",
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL,
	}

	checksums, err := v.FetchFileChecksums(context.Background(), "org/model", "main")
	require.NoError(t, err)
	assert.Equal(t, "abcdef123456", checksums["model.safetensors"]) // lowercased
	assert.NotContains(t, checksums, "config.json")
}

func TestFetchFileChecksums_NonOKStatus_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	v := &ChecksumVerifier{HTTPClient: srv.Client(), BaseURL: srv.URL}
	_, err := v.FetchFileChecksums(context.Background(), "org/repo", "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HF API returned")
}

func TestFetchFileChecksums_InvalidJSON_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	v := &ChecksumVerifier{HTTPClient: srv.Client(), BaseURL: srv.URL}
	_, err := v.FetchFileChecksums(context.Background(), "org/repo", "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode")
}

func TestFetchFileChecksums_Unreachable_Error(t *testing.T) {
	v := &ChecksumVerifier{
		HTTPClient: &http.Client{},
		BaseURL:    "http://127.0.0.1:19996",
	}
	_, err := v.FetchFileChecksums(context.Background(), "org/repo", "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch model info")
}

func TestFetchFileChecksums_WithToken_SetsHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"siblings":[]}`))
	}))
	defer srv.Close()

	v := &ChecksumVerifier{
		Token:      "hf-token-xyz",
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL,
	}

	_, err := v.FetchFileChecksums(context.Background(), "org/model", "main")
	require.NoError(t, err)
	assert.Equal(t, "Bearer hf-token-xyz", gotAuth)
}
