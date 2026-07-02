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

	assert.Equal(t, "vllm/vllm-openai:v0.24.0", controller.DefaultVLLMImage)
	assert.Equal(t, controller.DefaultVLLMImage, controllerapi.VLLMImage)
	assert.Equal(t, "vllm/vllm-openai-cpu:v0.24.0", cfg.Defaults.RuntimeImage)
	assert.Equal(t, "kserve/storage-initializer:v0.19.0", controller.StorageInitializerImage)
	assert.Equal(t, controller.StorageInitializerImage, controllerapi.StorageInitializerImage)
	assert.Equal(t, controller.StorageInitializerImage, cfg.Defaults.StorageInitializerImage)
	assert.Equal(t, scheduler.EPPImage, cfg.Defaults.SchedulerImage)
	assert.Equal(t, scheduler.EPPImage, cfg.Scheduler.Image)
	assert.Equal(t, "ghcr.io/spiffe/spire-agent:1.15.1", security.SPIREAgentImage)
	assert.Equal(t, "ghcr.io/spiffe/spire-server:1.15.1", security.SPIREServerImage)
	assert.Equal(t, "ghcr.io/spiffe/spiffe-helper:0.11.0", security.SPIFFEHelperImage)
}
