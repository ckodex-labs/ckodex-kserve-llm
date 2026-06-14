/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

const (
	// DefaultVLLMImage is the default vLLM container image.
	// Pinned to a specific version — never use :latest (supply chain risk, air-gapped incompatible).
	// Update: change the tag, rebuild, then update the digest comment below.
	// Verify: cosign verify vllm/vllm-openai:v0.23.0 --certificate-oidc-issuer=https://token.actions.githubusercontent.com
	DefaultVLLMImage = "vllm/vllm-openai:v0.23.0"

	// StorageInitializerImage is the default KServe init container image for standard storage (s3, gs, etc).
	// Pinned — :latest is an air-gapped deployment blocker and violates supply chain security policy.
	StorageInitializerImage = "kserve/storage-initializer:v0.19.0-rc0"
	// CKodexStorageInitializerImage is our custom Go image supporting s3://, hf://, GitHub, GitLab, etc.
	// Build with: make storage-initializer-load   (builds + loads into KIND)
	// Image source: Dockerfile target `storage-initializer`
	CKodexStorageInitializerImage = "ckodex/storage-initializer:v0.1.0"

	// HFMountCSIDriver is the CSI driver name registered by the hf-csi-driver DaemonSet.
	// Install via: helm install hf-csi oci://ghcr.io/huggingface/charts/hf-csi-driver
	HFMountCSIDriver = "hf.csi.huggingface.co"

	// ModelVolumeName is the shared volume for model artifacts.
	ModelVolumeName = "model-store"

	// ModelMountPath is where models are mounted in the container.
	ModelMountPath = "/mnt/models"

	// SeaweedFSFilerS3Endpoint is the full http:// URL for the SeaweedFS S3 API.
	// Must include scheme — boto3 and aws-sdk-go-v2 both require a complete URL for endpoint_url.
	SeaweedFSFilerS3Endpoint = "http://seaweedfs.storage:8333"
)

// Resource Management Defaults (Phase 5 Hardening)
const (
	// DefaultVLLMCPURequest ensures vLLM has enough compute for the scheduler to place it reliably.
	DefaultVLLMCPURequest = "2"
	// DefaultVLLMMemoryRequest provides enough overhead for 8B models in FP16 (disk-to-memory overhead).
	DefaultVLLMMemoryRequest = "4Gi"

	// DefaultASRCPURequest for Whisper-family models.
	DefaultASRCPURequest = "1"
	// DefaultASRMemoryRequest — enough for medium/large-v3 models.
	DefaultASRMemoryRequest = "2Gi"

	// DefaultCacheCPURequest for storage-initializer jobs.
	DefaultCacheCPURequest = "1"
	// DefaultCacheMemoryRequest for storage-initializer jobs.
	DefaultCacheMemoryRequest = "1Gi"

	// DefaultTerminationGracePeriod gives vLLM enough time to flush metrics and drain connections.
	DefaultTerminationGracePeriod = 60
	// ASRTerminationGracePeriod — ASR is usually stateless and can shut down faster.
	ASRTerminationGracePeriod = 30
)
