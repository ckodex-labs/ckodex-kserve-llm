package api

const (
	FinalizerName = "serving.ckodex.com/finalizer"

	VLLMImage                     = "vllm/vllm-openai:v0.24.0"
	QuantCppImage                 = "ckodex/quant-cpp:v0.1.0"
	StorageInitializerImage       = "kserve/storage-initializer:v0.19.0"
	HFCSIPVStorageSize            = "500Gi" // Nominal; hf-csi-driver does not enforce capacity limits
	CKodexStorageInitializerImage = "ckodex/storage-initializer:v0.1.0"
	HFMountCSIDriver              = "hf.csi.huggingface.co"

	ModelVolumeName = "model-store"
	ModelMountPath  = "/mnt/models"

	DefaultTerminationGracePeriod = 60
	DefaultVLLMCPURequest         = "2"
	DefaultVLLMMemoryRequest      = "4Gi"

	VLLMGemma4Image = "vllm/vllm-openai:gemma4"
)
