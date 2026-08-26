package deployment

import (
	"fmt"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	vllmruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime/vllm"
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

func (b *Builder) applyEngineSelection(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec, hwType HardwareType) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	if llmSvc.Spec.Quantization != nil && llmSvc.Spec.Quantization.Method == "gguf" {
		b.selectQuantCpp(llmSvc, c, hwType)
		return
	}
	switch llmSvc.Spec.Engine {
	case "quant-cpp":
		b.selectQuantCpp(llmSvc, c, hwType)
	case "", "vllm":
		b.selectVLLM(llmSvc, c)
	default:
		c.Image = ""
		c.Command = nil
		c.Args = nil
	}
}

func (b *Builder) selectQuantCpp(llmSvc *servingv1alpha2.LLMInferenceService, c *corev1.Container, hwType HardwareType) {
	c.Image = b.quantCppImage()
	if b.AirGappedMode && b.LocalRegistry != "" {
		c.Image = b.rewriteImage(c.Image)
	}
	b.ensureQuantCppArgs(llmSvc, c, hwType)
}

func (b *Builder) quantCppImage() string {
	if b.Defaults.QuantCppImage != "" {
		return b.Defaults.QuantCppImage
	}
	return api.QuantCppImage
}

func (b *Builder) selectVLLM(llmSvc *servingv1alpha2.LLMInferenceService, c *corev1.Container) {
	if c.Image == "" {
		c.Image = b.RuntimeImage
		if c.Image == "" {
			c.Image = api.VLLMImage
		}
	}
	if b.AirGappedMode && b.LocalRegistry != "" {
		c.Image = b.rewriteImage(c.Image)
	}
	rendered := (vllmruntime.Adapter{}).Render(inferenceruntime.RenderRequest{Service: llmSvc, ExistingArgs: c.Args, ModelPath: api.ModelMountPath})
	c.Args = rendered.Args
}

func (b *Builder) ensureQuantCppArgs(llmSvc *servingv1alpha2.LLMInferenceService, c *corev1.Container, hwType HardwareType) {
	if !hasArg(c.Args, "-m") && !hasArg(c.Args, "--model") {
		c.Args = append(c.Args, "-m", api.ModelMountPath)
	}
	if ctxSize, ok := llmSvc.Annotations["ckodex.com/ctx-size"]; ok {
		c.Args = append(c.Args, "--ctx-size", ctxSize)
	}
	if hwType == HardwareAppleSilicon && !hasArg(c.Args, "-ngl") && !hasArg(c.Args, "--n-gpu-layers") {
		c.Args = append(c.Args, "--n-gpu-layers", "99")
	}
	if !hasArg(c.Args, "--host") {
		c.Args = append(c.Args, "--host", "0.0.0.0", "--port", "8000")
	}
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
