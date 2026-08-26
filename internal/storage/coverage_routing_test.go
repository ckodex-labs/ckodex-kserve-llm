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

// TestS3Client_Pull_InvalidURI verifies error on malformed URI.
func TestGetClient_UnknownScheme_Error(t *testing.T) {
	_, err := GetClient("unknownscheme")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no storage client registered for scheme")
}

func TestGetClient_KnownScheme_OK(t *testing.T) {
	// "oci" is registered by the OCI client's init() function.
	c, err := GetClient("oci")
	require.NoError(t, err)
	assert.NotNil(t, c)
}

// ============================================================================
// ComputeSHA256Stream — error reader
// ============================================================================

// errReader always returns an error on Read.
type errReader struct{ msg string }
