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
func (e *errReader) Read(_ []byte) (int, error) {
	return 0, &testReadError{e.msg}
}

type testReadError struct{ msg string }

func (e *testReadError) Error() string { return e.msg }

func TestNewGitLabClient_EmptyToken_OK(t *testing.T) {
	c, err := NewGitLabClient("")
	require.NoError(t, err)
	assert.NotNil(t, c)
}

// Note: GitLab pullRecursive branch tests (ListTree error, GetRawFile error) are in gitlab_extra_test.go.

// ============================================================================
// ComputeSHA256 — file not found path
// ============================================================================
