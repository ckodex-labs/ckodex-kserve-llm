package compatibility_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller"
	controllerapi "github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/ckodex-labs/kserve-llm-operator/internal/scheduler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/security"
)

func TestReleaseDefaultsStayAligned(t *testing.T) {
	cfg := config.DefaultOperatorConfig()

	assert.Equal(t, "vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e", controller.DefaultVLLMImage)
	assert.Equal(t, controller.DefaultVLLMImage, controllerapi.VLLMImage)
	assert.Equal(t, controller.DefaultVLLMImage, cfg.Defaults.RuntimeImage)
	assert.Equal(t, "kserve/storage-initializer:v0.21.0@sha256:3a3a6a8053478c8d8d90f96fc5e54c1892c07784102dda70104374d4f89a3672", controller.StorageInitializerImage)
	assert.Equal(t, controller.StorageInitializerImage, controllerapi.StorageInitializerImage)
	assert.Equal(t, controller.StorageInitializerImage, cfg.Defaults.StorageInitializerImage)
	assert.Equal(t, controllerapi.HuggingFaceInitializerImage, cfg.Defaults.HuggingFaceInitializerImage)
	assert.Equal(t, scheduler.EPPImage, cfg.Defaults.SchedulerImage)
	assert.Equal(t, scheduler.EPPImage, cfg.Scheduler.Image)
	assert.Equal(t, "ghcr.io/spiffe/spire-agent:1.15.1", security.SPIREAgentImage)
	assert.Equal(t, "ghcr.io/spiffe/spire-server:1.15.1", security.SPIREServerImage)
	assert.Equal(t, "ghcr.io/spiffe/spiffe-helper:0.11.0", security.SPIFFEHelperImage)
}
