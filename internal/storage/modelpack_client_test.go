/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"reflect"
	"testing"
)

func TestParseModelpackURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    *ModelArtifact
		wantErr bool
	}{
		{
			name: "valid URI with tag",
			uri:  "modelpack://registry.example.com/models/qwen3:v1.0",
			want: &ModelArtifact{
				RawURI:     "modelpack://registry.example.com/models/qwen3:v1.0",
				Registry:   "registry.example.com",
				Repository: "models/qwen3",
				Reference:  "v1.0",
			},
			wantErr: false,
		},
		{
			name: "valid URI with digest",
			uri:  "modelpack://ghcr.io/modelpack/qwen3@sha256:1234567890abcdef",
			want: &ModelArtifact{
				RawURI:     "modelpack://ghcr.io/modelpack/qwen3@sha256:1234567890abcdef",
				Registry:   "ghcr.io",
				Repository: "modelpack/qwen3",
				Reference:  "sha256:1234567890abcdef",
				Digest:     "sha256:1234567890abcdef",
			},
			wantErr: false,
		},
		{
			name: "valid URI without tag (default to latest)",
			uri:  "modelpack://localhost:5000/my-model",
			want: &ModelArtifact{
				RawURI:     "modelpack://localhost:5000/my-model",
				Registry:   "localhost:5000",
				Repository: "my-model",
				Reference:  "latest",
			},
			wantErr: false,
		},
		{
			name:    "invalid scheme",
			uri:     "oci://ghcr.io/models/qwen3:latest",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid URI format",
			uri:     "modelpack://only-registry",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseModelpackURI(tt.uri)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseModelpackURI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseModelpackURI() = %v, want %v", got, tt.want)
			}
		})
	}
}
