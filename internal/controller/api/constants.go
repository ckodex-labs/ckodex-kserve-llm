package api

const (
	FinalizerName = "serving.ckodex.com/finalizer"

	VLLMImage                     = "vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e"
	StorageInitializerImage       = "kserve/storage-initializer:v0.21.0@sha256:3a3a6a8053478c8d8d90f96fc5e54c1892c07784102dda70104374d4f89a3672"
	HuggingFaceInitializerImage   = "ghcr.io/ckodex-labs/ckodex-kserve-llm-huggingface-initializer:v0.20.0"
	HFCSIPVStorageSize            = "500Gi" // Nominal; hf-csi-driver does not enforce capacity limits
	CKodexStorageInitializerImage = "ckodex/storage-initializer:v0.1.0"
	HFMountCSIDriver              = "hf.csi.huggingface.co"

	ModelVolumeName = "model-store"
	ModelMountPath  = "/mnt/models"

	DefaultTerminationGracePeriod = 60
	DefaultVLLMCPURequest         = "2"
	DefaultVLLMMemoryRequest      = "4Gi"
)
