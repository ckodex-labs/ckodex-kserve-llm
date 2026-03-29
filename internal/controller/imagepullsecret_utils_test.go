/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- indexOf -------------------------------------------------------------

func TestIndexOf_Found(t *testing.T) {
	assert.Equal(t, 0, indexOf("abc", "a"))
	assert.Equal(t, 1, indexOf("abc", "b"))
	assert.Equal(t, 2, indexOf("abc", "c"))
}

func TestIndexOf_MultiChar(t *testing.T) {
	assert.Equal(t, 1, indexOf("abc", "bc"))
	assert.Equal(t, 0, indexOf("abc", "abc"))
}

func TestIndexOf_NotFound(t *testing.T) {
	assert.Equal(t, -1, indexOf("abc", "d"))
	assert.Equal(t, -1, indexOf("", "a"))
}

func TestIndexOf_Separator(t *testing.T) {
	assert.Equal(t, 9, indexOf("namespace/name", "/"))
}

// ---- splitN --------------------------------------------------------------

func TestSplitN_TwoParts(t *testing.T) {
	parts := splitN("namespace/name", "/", 2)
	assert.Equal(t, []string{"namespace", "name"}, parts)
}

func TestSplitN_NoSeparator(t *testing.T) {
	parts := splitN("justname", "/", 2)
	// No separator found — returns whole string as single element.
	assert.Equal(t, []string{"justname"}, parts)
}

func TestSplitN_MultipleSlashes_OnlySplitsOnce(t *testing.T) {
	// n=2 means at most one split.
	parts := splitN("ns/sub/name", "/", 2)
	assert.Equal(t, []string{"ns", "sub/name"}, parts)
}

func TestSplitN_EmptyString(t *testing.T) {
	parts := splitN("", "/", 2)
	assert.Equal(t, []string{""}, parts)
}

// ---- parseSecretRef ------------------------------------------------------

func TestParseSecretRef_Valid(t *testing.T) {
	ns, name, err := parseSecretRef("kube-system/my-pull-secret")
	require.NoError(t, err)
	assert.Equal(t, "kube-system", ns)
	assert.Equal(t, "my-pull-secret", name)
}

func TestParseSecretRef_DefaultNamespace(t *testing.T) {
	ns, name, err := parseSecretRef("default/regcred")
	require.NoError(t, err)
	assert.Equal(t, "default", ns)
	assert.Equal(t, "regcred", name)
}

func TestParseSecretRef_NoSlash_Error(t *testing.T) {
	_, _, err := parseSecretRef("justname")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "<namespace>/<name>")
}

func TestParseSecretRef_EmptyNamespace_Error(t *testing.T) {
	_, _, err := parseSecretRef("/name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "<namespace>/<name>")
}

func TestParseSecretRef_EmptyName_Error(t *testing.T) {
	_, _, err := parseSecretRef("namespace/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "<namespace>/<name>")
}

func TestParseSecretRef_EmptyString_Error(t *testing.T) {
	_, _, err := parseSecretRef("")
	require.Error(t, err)
}

func TestParseSecretRef_MultipleSlashes_SecondBecomesName(t *testing.T) {
	// splitN(n=2) keeps everything after the first slash as "name".
	ns, name, err := parseSecretRef("ns/sub/secret")
	require.NoError(t, err)
	assert.Equal(t, "ns", ns)
	assert.Equal(t, "sub/secret", name)
}
