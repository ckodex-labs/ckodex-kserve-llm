/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"testing"
)

func TestRegistry(t *testing.T) {
	schemes := []string{"hf", "huggingface", "github", "gitlab", "artifactory", "oci", "ocis", "seaweedfs", "modelpack"}

	for _, s := range schemes {
		client, err := GetClient(s)
		if err != nil {
			t.Errorf("expected client for scheme %s, got error: %v", s, err)
		}
		if client == nil {
			t.Errorf("expected non-nil client for scheme %s", s)
		}
	}
}

func TestURIHandling(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		scheme   string
		expected string
	}{
		{"HuggingFace", "hf://org/repo@branch", "hf", "hf"},
		{"GitHub", "github://owner/repo/path@ref", "github", "github"},
		{"GitLab", "gitlab://project/path@ref", "gitlab", "gitlab"},
		{"Artifactory", "artifactory://host/repo/path", "artifactory", "artifactory"},
		{"S3", "s3://bucket/path", "s3", "s3"},
		{"GS", "gs://bucket/path", "gs", "gs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := GetClient(tt.scheme)
			if err != nil {
				t.Fatalf("failed to get client: %v", err)
			}

			// We don't actually pull in the test, just check if the client is registered
			if client == nil {
				t.Errorf("client for %s is nil", tt.scheme)
			}
		})
	}
}
