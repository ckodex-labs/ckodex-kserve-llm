package deployment

import (
	"fmt"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	runtimeregistry "github.com/ckodex-labs/kserve-llm-operator/internal/runtime/registry"
	"github.com/ckodex-labs/kserve-llm-operator/internal/storage"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func (b *Builder) applyLoraAdapters(loras []servingv1alpha2.LLMLoraAdapter, podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	if !hasArg(c.Args, "--enable-lora") {
		c.Args = append(c.Args, "--enable-lora")
	}
	for _, lora := range loras {
		b.addLoraMount(lora, c, podSpec)
	}
}

func (b *Builder) addLoraMount(lora servingv1alpha2.LLMLoraAdapter, c *corev1.Container, podSpec *corev1.PodSpec) {
	name := fmt.Sprintf("lora-%s", lora.Name)
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
			Path: fmt.Sprintf("/tmp/ckodex/models/%s", name), Type: ptr.To(corev1.HostPathDirectoryOrCreate),
		}},
	})
	c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
		Name: name, MountPath: fmt.Sprintf("%s/lora-%s", api.ModelMountPath, lora.Name), ReadOnly: true,
	})
}

func (b *Builder) applyEngineSelection(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec, _ HardwareType) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	adapter, ok := runtimeregistry.Resolve(llmSvc.Spec.Engine)
	if !ok || len(adapter.Validate(llmSvc)) > 0 || !adapter.Image().Valid() {
		c.Image = ""
		c.Command = nil
		c.Args = nil
		return
	}
	if adapter.Name() != runtimeregistry.DefaultEngine {
		// Non-default engines are admitted only against their verified manifest;
		// a vLLM operator default must not leak across the engine boundary.
		c.Image = adapter.Image().Reference()
	} else if c.Image == "" {
		c.Image = b.RuntimeImage
		if c.Image == "" {
			c.Image = api.VLLMImage
		}
	}
	if b.AirGappedMode && b.LocalRegistry != "" {
		c.Image = b.rewriteImage(c.Image)
	}
	rendered := adapter.Render(inferenceruntime.RenderRequest{Service: llmSvc, ExistingArgs: c.Args, ModelPath: api.ModelMountPath})
	c.Args = rendered.Args
}

func (b *Builder) rewriteImage(image string) string {
	if b.LocalRegistry == "" {
		return image
	}
	return fmt.Sprintf("%s/%s", b.LocalRegistry, strings.Join(strings.Split(image, "/"), "/"))
}

func (b *Builder) storageResolveAirGap(uri string) string {
	return storage.ResolveAirGappedURI(uri, b.LocalRegistry)
}
