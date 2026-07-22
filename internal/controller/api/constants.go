package api

const (
	FinalizerName = "serving.ckodex.com/finalizer"

	VLLMImage                     = "vllm/vllm-openai:v0.25.1"
	QuantCppImage                 = "ckodex/quant-cpp:v0.1.0"
	StorageInitializerImage       = "kserve/storage-initializer:v0.19.0"
	HuggingFaceInitializerImage   = "python:3.12.11-slim-bookworm@sha256:519591d6871b7bc437060736b9f7456b8731f1499a57e22e6c285135ae657bf7"
	HuggingFaceHubVersion         = "1.8.0"
	HuggingFaceXetVersion         = "1.5.2"
	HFCSIPVStorageSize            = "500Gi" // Nominal; hf-csi-driver does not enforce capacity limits
	CKodexStorageInitializerImage = "ckodex/storage-initializer:v0.1.0"
	HFMountCSIDriver              = "hf.csi.huggingface.co"

	ModelVolumeName = "model-store"
	ModelMountPath  = "/mnt/models"

	DefaultTerminationGracePeriod = 60
	DefaultVLLMCPURequest         = "2"
	DefaultVLLMMemoryRequest      = "4Gi"
)
