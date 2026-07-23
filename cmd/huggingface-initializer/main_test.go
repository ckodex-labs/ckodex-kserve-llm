/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeArgsKServeContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uri  string
		want []string
	}{
		{
			name: "default revision",
			uri:  "hf://org/model",
			want: []string{"download", "org/model", "--revision", "main", "--local-dir", "/mnt/models"},
		},
		{
			name: "explicit revision",
			uri:  "hf://org/model@release",
			want: []string{"download", "org/model", "--revision", "release", "--local-dir", "/mnt/models"},
		},
		{
			name: "empty revision",
			uri:  "hf://org/model@",
			want: []string{"download", "org/model", "--revision", "main", "--local-dir", "/mnt/models"},
		},
		{
			name: "mirror scheme",
			uri:  "hf-mirror://org/model@v2",
			want: []string{"download", "org/model", "--revision", "v2", "--local-dir", "/mnt/models"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeArgs([]string{tt.uri, "/mnt/models"})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeArgsPassesThroughHFCLIArguments(t *testing.T) {
	t.Parallel()

	args := []string{"download", "org/model", "--local-dir", "/models"}
	got, err := normalizeArgs(args)
	require.NoError(t, err)
	assert.Equal(t, args, got)
}

func TestNormalizeArgsRejectsIncompleteKServeContract(t *testing.T) {
	t.Parallel()

	_, err := normalizeArgs([]string{"hf://", "/mnt/models"})
	require.ErrorContains(t, err, "must include a repository")

	_, err = normalizeArgs([]string{"hf://org/model", ""})
	require.ErrorContains(t, err, "must not be empty")

	_, err = normalizeArgs([]string{"huggingface://org/model", "/mnt/models"})
	require.ErrorContains(t, err, "unsupported source URI")
	require.ErrorContains(t, err, "hf:// or hf-mirror://")
}
