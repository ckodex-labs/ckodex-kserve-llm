/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"testing"

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
