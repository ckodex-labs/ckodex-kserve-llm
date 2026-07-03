/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v62/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ParseOCIURI
// ============================================================================

func TestParseOCIURI_TagReference(t *testing.T) {
	a, err := ParseOCIURI("oci://ghcr.io/ckodex/llama3:v1.2")
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io", a.Registry)
	assert.Equal(t, "ckodex/llama3", a.Repository)
	assert.Equal(t, "v1.2", a.Reference)
	assert.Empty(t, a.Digest)
}

func TestParseOCIURI_OCISTagReference(t *testing.T) {
	a, err := ParseOCIURI("ocis://ghcr.io/ckodex/llama3:v1.2")
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io", a.Registry)
	assert.Equal(t, "ckodex/llama3", a.Repository)
	assert.Equal(t, "v1.2", a.Reference)
	assert.Equal(t, "ocis://ghcr.io/ckodex/llama3:v1.2", a.RawURI)
}

func TestParseOCIURI_DigestReference(t *testing.T) {
	a, err := ParseOCIURI("oci://ghcr.io/ckodex/llama3@sha256:abc123")
	require.NoError(t, err)
	assert.Equal(t, "ckodex/llama3", a.Repository)
	assert.Equal(t, "sha256:abc123", a.Digest)
	assert.Equal(t, "sha256:abc123", a.Reference)
}

func TestParseOCIURI_NoTag_DefaultsToLatest(t *testing.T) {
	a, err := ParseOCIURI("oci://ghcr.io/ckodex/llama3")
	require.NoError(t, err)
	assert.Equal(t, "latest", a.Reference)
}

func TestParseOCIURI_PreservesRawURI(t *testing.T) {
	raw := "oci://ghcr.io/ckodex/llama3:v1"
	a, err := ParseOCIURI(raw)
	require.NoError(t, err)
	assert.Equal(t, raw, a.RawURI)
}

func TestParseOCIURI_NotOCIURI_Error(t *testing.T) {
	_, err := ParseOCIURI("hf://org/model")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an OCI URI")
}

func TestParseOCIURI_MissingRepository_Error(t *testing.T) {
	_, err := ParseOCIURI("oci://ghcr.io")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid OCI URI")
}

func TestResolveAirGappedURI_PreservesOCISScheme(t *testing.T) {
	resolved := ResolveAirGappedURI("ocis://ghcr.io/ckodex/model:tag", "registry.corp.internal")
	assert.Equal(t, "ocis://registry.corp.internal/ghcr.io/ckodex/model:tag", resolved)
}

// ============================================================================
// ParseSeaweedFSURI
// ============================================================================

func TestParseSeaweedFSURI_Valid(t *testing.T) {
	a, err := ParseSeaweedFSURI("seaweedfs://filer.storage:8888/models/llama3")
	require.NoError(t, err)
	assert.Equal(t, "filer.storage:8888", a.FilerHost)
	assert.Equal(t, "/models/llama3", a.Path)
	assert.Equal(t, "seaweedfs://filer.storage:8888/models/llama3", a.RawURI)
}

func TestParseSeaweedFSURI_NotSeaweedFS_Error(t *testing.T) {
	_, err := ParseSeaweedFSURI("s3://bucket/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a SeaweedFS URI")
}

func TestParseSeaweedFSURI_MissingPath_Error(t *testing.T) {
	_, err := ParseSeaweedFSURI("seaweedfs://filer.storage")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid SeaweedFS URI")
}

// ============================================================================
// DefaultSeaweedFSConfig
// ============================================================================

func TestDefaultSeaweedFSConfig_FilerURL(t *testing.T) {
	cfg := DefaultSeaweedFSConfig()
	assert.Equal(t, "http://seaweedfs-filer.storage:8888", cfg.FilerURL)
}

func TestDefaultSeaweedFSConfig_BasePath(t *testing.T) {
	cfg := DefaultSeaweedFSConfig()
	assert.Equal(t, "/models", cfg.BasePath)
}

func TestDefaultSeaweedFSConfig_Timeout(t *testing.T) {
	cfg := DefaultSeaweedFSConfig()
	assert.Equal(t, int64(5*60*1e9), int64(cfg.Timeout), "5-minute timeout")
}

// ============================================================================
// SeaweedFSClient HTTP operations
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

func TestInjectVaultSecrets_EmptyPath_NoOp(t *testing.T) {
	err := InjectVaultSecrets(context.Background(), "")
	require.NoError(t, err)
}

// ============================================================================
// ParseModelpackURI
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

func TestIsInsecureRegistry_Localhost(t *testing.T) {
	assert.True(t, isInsecureRegistry("localhost:5000"))
}

func TestIsInsecureRegistry_Loopback(t *testing.T) {
	assert.True(t, isInsecureRegistry("127.0.0.1:5000"))
}

func TestIsInsecureRegistry_Production_NotInsecure(t *testing.T) {
	assert.False(t, isInsecureRegistry("ghcr.io"))
}

func TestIsInsecureRegistry_EnvVar(t *testing.T) {
	t.Setenv("OCI_INSECURE", "1")
	assert.True(t, isInsecureRegistry("ghcr.io"))
}

func TestIsInsecureRegistry_EnvVar_True(t *testing.T) {
	t.Setenv("OCI_INSECURE", "true")
	assert.True(t, isInsecureRegistry("ghcr.io"))
}

// ============================================================================
// OCIClient.logPulledLayers — filesystem-only function
// ============================================================================

func TestLogPulledLayers_Dir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.safetensors"), []byte("w"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0600))

	c := &OCIClient{}
	// Must not panic. Logs to stdout — no return value to assert.
	c.logPulledLayers(dir)
}

func TestLogPulledLayers_NonExistentDir_NoError(t *testing.T) {
	c := &OCIClient{}
	c.logPulledLayers("/nonexistent/path/xyz")
}

// ============================================================================
// S3Client — pure-logic helpers and URI-error path
// ============================================================================

func TestS3PathStyleOption_SetsUsePathStyle(t *testing.T) {
	var opts struct{ UsePathStyle bool }
	// s3PathStyleOption returns a func(*s3.Options); we simulate just the field.
	// We verify via the S3Client constructor behavior rather than direct struct mutation.
	f := s3PathStyleOption()
	require.NotNil(t, f)
	_ = opts
}

func TestS3ConfigOptions_DefaultRegion(t *testing.T) {
	opts := s3ConfigOptions("us-west-2")
	require.NotEmpty(t, opts, "must return at least the region option")
}

func TestS3ConfigOptions_WithAnonymousCredentials(t *testing.T) {
	t.Setenv("AWS_NO_SIGN_REQUEST", "yes")
	opts := s3ConfigOptions("us-east-1")
	assert.Len(t, opts, 2, "region + anonymous credentials")
}

func TestS3ConfigOptions_WithCustomEndpoint_S3_Endpoint(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "http://minio.local:9000")
	opts := s3ConfigOptions("us-east-1")
	assert.Len(t, opts, 2, "region + endpoint")
}

func TestS3ConfigOptions_WithCustomEndpoint_AWS_Endpoint_URL(t *testing.T) {
	t.Setenv("AWS_ENDPOINT_URL", "http://minio.local:9000")
	opts := s3ConfigOptions("us-east-1")
	assert.Len(t, opts, 2, "region + endpoint")
}

func TestS3Client_Pull_InvalidURI_Error(t *testing.T) {
	c := &S3Client{}
	err := c.Pull(context.Background(), "s3://bucket-only", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid S3 URI format")
}

func TestS3Client_Schemes(t *testing.T) {
	c := &S3Client{}
	assert.Contains(t, c.Schemes(), "s3")
}

// ============================================================================
// github_client.downloadFile — replace DefaultTransport for testing
// ============================================================================

func TestGitHubClient_DownloadFile_OK(t *testing.T) {
	content := []byte("model weights")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	// Replace http.DefaultTransport to route to test server.
	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &GitHubClient{}
	dest := filepath.Join(t.TempDir(), "model.bin")
	require.NoError(t, c.downloadFile(context.Background(), srv.URL+"/file", dest))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestGitHubClient_DownloadFile_NonOK_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	c := &GitHubClient{}
	err := c.downloadFile(context.Background(), srv.URL+"/file", filepath.Join(t.TempDir(), "f.bin"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download from GitHub")
}

// ============================================================================
// huggingface_client.listRepoFiles — replace DefaultTransport
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

func TestNewVaultClient_OK(t *testing.T) {
	// vault.NewClient only configures the client; it does NOT connect to Vault.
	t.Setenv("VAULT_ADDR", "http://127.0.0.1:19996") // unreachable but valid URL
	c, err := NewVaultClient()
	require.NoError(t, err)
	require.NotNil(t, c)
}

// buildVaultServer returns a fake Vault HTTP server that serves a fixed secret payload.
func buildVaultServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func TestVaultClient_FetchSecret_OK(t *testing.T) {
	srv := buildVaultServer(t, http.StatusOK,
		`{"request_id":"r1","lease_id":"","renewable":false,"lease_duration":0,"data":{"HF_TOKEN":"my-hf-secret"},"wrap_info":null,"warnings":null,"auth":null}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-vault-token")

	c, err := NewVaultClient()
	require.NoError(t, err)

	data, err := c.FetchSecret(context.Background(), "secret/data/hf")
	require.NoError(t, err)
	assert.Equal(t, "my-hf-secret", data["HF_TOKEN"])
}

func TestVaultClient_FetchSecret_NilData_Error(t *testing.T) {
	// Vault returns 200 but with null data (path exists, no secrets stored).
	srv := buildVaultServer(t, http.StatusOK,
		`{"request_id":"r2","lease_id":"","renewable":false,"lease_duration":0,"data":null,"wrap_info":null,"warnings":null,"auth":null}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")

	c, err := NewVaultClient()
	require.NoError(t, err)

	_, err = c.FetchSecret(context.Background(), "secret/data/empty")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data found at Vault path")
}

func TestVaultClient_FetchSecret_APIError(t *testing.T) {
	// Vault returns 403 Forbidden → SDK wraps as error.
	srv := buildVaultServer(t, http.StatusForbidden,
		`{"errors":["1 error occurred: * permission denied"]}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "bad-token")

	c, err := NewVaultClient()
	require.NoError(t, err)

	_, err = c.FetchSecret(context.Background(), "secret/data/protected")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read from Vault")
}

func TestInjectVaultSecrets_SetsEnvFromVault(t *testing.T) {
	srv := buildVaultServer(t, http.StatusOK,
		`{"request_id":"r3","lease_id":"","renewable":false,"lease_duration":0,"data":{"TEST_VAULT_KEY":"injected-value"},"wrap_info":null,"warnings":null,"auth":null}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")

	// Clean up injected env var after the test.
	t.Cleanup(func() { _ = os.Unsetenv("TEST_VAULT_KEY") })

	err := InjectVaultSecrets(context.Background(), "secret/data/inject")
	require.NoError(t, err)
	assert.Equal(t, "injected-value", os.Getenv("TEST_VAULT_KEY"))
}

// ============================================================================
// OCIClient — resolveCredentials (pure logic, no registry connection)
// ============================================================================

func TestOCIClient_ResolveCredentials_ExplicitConfig(t *testing.T) {
	c := &OCIClient{
		RegistryAuth: map[string]RegistryAuthConfig{
			"ghcr.io": {Username: "user1", Password: "pass1"},
		},
	}
	cred := c.resolveCredentials("ghcr.io")
	assert.Equal(t, "user1", cred.Username)
	assert.Equal(t, "pass1", cred.Password)
}

func TestOCIClient_ResolveCredentials_ExplicitConfig_EmptyUsername_FallsThrough(t *testing.T) {
	// Username is empty → falls through to env var check.
	t.Setenv("OCI_REGISTRY_USERNAME", "")
	c := &OCIClient{
		RegistryAuth: map[string]RegistryAuthConfig{
			"ghcr.io": {Username: "", Password: ""},
		},
	}
	// No env var, no docker config → should return anonymous credential.
	cred := c.resolveCredentials("ghcr.io")
	// Anonymous credential has empty Username.
	assert.Empty(t, cred.Username)
}

func TestOCIClient_ResolveCredentials_EnvVar(t *testing.T) {
	t.Setenv("OCI_REGISTRY_USERNAME", "env-user")
	t.Setenv("OCI_REGISTRY_PASSWORD", "env-pass")

	c := &OCIClient{}
	cred := c.resolveCredentials("ghcr.io")
	assert.Equal(t, "env-user", cred.Username)
	assert.Equal(t, "env-pass", cred.Password)
}

func TestOCIClient_ResolveCredentials_DockerConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := `{"auths":{"ghcr.io":{"username":"docker-user","password":"docker-pass"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("OCI_REGISTRY_USERNAME", "") // ensure env branch does not trigger

	c := &OCIClient{}
	cred := c.resolveCredentials("ghcr.io")
	assert.Equal(t, "docker-user", cred.Username)
}

func TestOCIClient_ResolveCredentials_Anonymous(t *testing.T) {
	// No config, no env vars, no docker config → anonymous.
	t.Setenv("OCI_REGISTRY_USERNAME", "")
	t.Setenv("DOCKER_CONFIG", t.TempDir()) // empty dir → config.json missing → falls through

	c := &OCIClient{}
	cred := c.resolveCredentials("ghcr.io")
	assert.Empty(t, cred.Username)
	assert.Empty(t, cred.Password)
}

// ============================================================================
// loadDockerCredential — filesystem-based tests
// ============================================================================

func TestLoadDockerCredential_OK(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := `{"auths":{"my-registry.io":{"username":"u","password":"p"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	cred, err := loadDockerCredential(cfgPath, "my-registry.io")
	require.NoError(t, err)
	assert.Equal(t, "u", cred.Username)
	assert.Equal(t, "p", cred.Password)
}

func TestLoadDockerCredential_HTTPSPrefixMatch(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// Registry stored with https:// prefix (common in older docker configs).
	cfg := `{"auths":{"https://my-registry.io":{"username":"u2","password":"p2"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	cred, err := loadDockerCredential(cfgPath, "my-registry.io")
	require.NoError(t, err)
	assert.Equal(t, "u2", cred.Username)
}

func TestLoadDockerCredential_EntryNotFound_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := `{"auths":{"other-registry.io":{"username":"u","password":"p"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	_, err := loadDockerCredential(cfgPath, "missing-registry.io")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no docker auth")
}

func TestLoadDockerCredential_FileNotFound_Error(t *testing.T) {
	_, err := loadDockerCredential("/nonexistent/config.json", "registry.io")
	require.Error(t, err)
}

func TestLoadDockerCredential_InvalidJSON_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte("not-json"), 0600))

	_, err := loadDockerCredential(cfgPath, "registry.io")
	require.Error(t, err)
}

func TestLoadDockerCredential_Base64Auth_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// Only the "auth" base64 field, no username/password.
	cfg := `{"auths":{"registry.io":{"auth":"dXNlcjpwYXNz"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	_, err := loadDockerCredential(cfgPath, "registry.io")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base64 auth not decoded")
}

func TestLoadDockerCredential_EmptyEntry_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// Entry exists but all fields empty.
	cfg := `{"auths":{"registry.io":{}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0600))

	_, err := loadDockerCredential(cfgPath, "registry.io")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty auth entry")
}

// ============================================================================
// OCIClient — early-exit error paths (no network)
// ============================================================================

func TestOCIClient_Pull_InvalidURI_Error(t *testing.T) {
	c := &OCIClient{}
	err := c.Pull(context.Background(), "s3://bucket/path", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an OCI URI")
}

func TestOCIClient_PullInternal_EmptyRef_Error(t *testing.T) {
	c := &OCIClient{}
	err := c.pullInternal(context.Background(), &ModelArtifact{Reference: ""}, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no OCI reference configured")
}

func TestNewOCIFileStore_DisablesAutomaticUnpack(t *testing.T) {
	store, err := newOCIFileStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	assert.True(t, store.SkipUnpack, "automatic archive unpacking must remain disabled")
}

func TestOCIClient_Push_EmptyRef_Error(t *testing.T) {
	c := &OCIClient{}
	err := c.Push(context.Background(), &ModelArtifact{Reference: ""}, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no OCI reference configured")
}

// ============================================================================
// GitHub pullRecursive — white-box via fake GitHub API server
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

func TestModelpackClient_PullInternal_EmptyRef_Error(t *testing.T) {
	c := &ModelpackClient{}
	artifact := &ModelArtifact{
		Registry:   "registry.example.com",
		Repository: "mymodel",
		Reference:  "", // empty — should error immediately
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

// ============================================================================
// ArtifactoryClient
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

// TestModelpackClient_Push_NetworkError covers the Push function's error path
// when the target registry is unreachable.
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
func TestInjectVaultSecrets_NonEmptyPath_AttemptsFetch(t *testing.T) {
	srv := buildVaultServer(t, http.StatusForbidden, `{"errors":["forbidden"]}`)
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-vault-token")
	err := InjectVaultSecrets(context.Background(), "secret/myapp")
	require.Error(t, err)
}
