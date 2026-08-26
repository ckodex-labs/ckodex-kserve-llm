/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinimizedModelReferenceDoesNotDiscloseURI(t *testing.T) {
	tests := []struct {
		name   string
		uri    string
		scheme string
		secret []string
	}{
		{
			name:   "https signed URL",
			uri:    "https://audit-user:secret@example.com/private/models/llama?X-Amz-Credential=key&X-Amz-Signature=signature",
			scheme: "https",
			secret: []string{"audit-user", "secret", "example.com", "/private/models/llama", "X-Amz-Signature"},
		},
		{
			name:   "s3 path",
			uri:    "s3://access-key:secret@bucket/private/models/llama?token=secret-token",
			scheme: "s3",
			secret: []string{"access-key", "secret", "bucket", "/private/models/llama", "secret-token"},
		},
		{
			name:   "hugging face reference",
			uri:    "hf://org/private-model",
			scheme: "hf",
			secret: []string{"org", "/private-model"},
		},
		{
			name:   "OCI reference",
			uri:    "oci://registry.example.com/private/model:latest",
			scheme: "oci",
			secret: []string{"registry.example.com", "/private/model:latest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, scheme := minimizedModelReference(tt.uri)
			require.Equal(t, tt.scheme, scheme)
			require.Regexp(t, regexp.MustCompile(`^sha256:[0-9a-f]{64}$`), ref)
			require.NotEqual(t, tt.uri, ref)
			for _, disclosed := range tt.secret {
				require.NotContains(t, ref, disclosed)
			}
		})
	}
}

func TestLLMInferenceAuditDetailsPreserveOperationalFields(t *testing.T) {
	service := makeLLMInferenceService("llama", "prod")
	service.Spec.Engine = "vllm"
	service.Spec.Model.URI = "https://user:secret@example.com/model?signature=secret"
	service.Status.Replicas = 3
	service.Status.ModelReady = true
	service.Status.DetectedHardware = "NVIDIA H100"

	details := llmInferenceAuditDetails(service, "enforced")
	encoded, err := json.Marshal(details)
	require.NoError(t, err)
	serialized := string(encoded)
	require.NotContains(t, serialized, "user")
	require.NotContains(t, serialized, "secret")
	require.NotContains(t, serialized, "example.com")
	require.NotContains(t, serialized, "/model")
	require.NotContains(t, serialized, "signature")
	require.Equal(t, "https", details["model_scheme"])
	require.Regexp(t, regexp.MustCompile(`^sha256:[0-9a-f]{64}$`), details["model_ref"])
	require.Equal(t, "vllm", details["engine"])
	require.Equal(t, "true", details["ready"])
	require.Equal(t, "NVIDIA H100", details["detected_hw"])
	require.Equal(t, "enforced", details["exec.mode"])
	require.Equal(t, "3", details["replicas"])
	require.False(t, strings.Contains(serialized, "model_uri"))
}
