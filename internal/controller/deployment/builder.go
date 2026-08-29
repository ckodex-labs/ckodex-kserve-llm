package deployment

import (
	"context"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

// SPIREInjector defines the interface for SPIRE sidecar injection.
type SPIREInjector interface {
	InjectSidecar(podSpec *corev1.PodSpec, llmSvc *servingv1alpha2.LLMInferenceService)
}

// Builder constructs Deployment objects for LLM inference.
type Builder struct {
	Client                  client.Client
	Recorder                record.EventRecorder
	SPIRE                   SPIREInjector
	EnableHardwareSelection bool
	OTEL_Endpoint           string
	AirGappedMode           bool
	LocalRegistry           string
	LocalCosignKeyPath      string
	RuntimeImage            string
	HFInitializerImage      string
	HFMirrorURL             string
	Defaults                operatorconfig.DefaultsConfig
}

// Build constructs the desired Deployment spec.
func (b *Builder) Build(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32, hwType HardwareType, loras []servingv1alpha2.LLMLoraAdapter) *appsv1.Deployment {
	return b.BuildWithRole(ctx, llmSvc, replicas, hwType, loras, "")
}

// BuildWithRole builds a model Deployment and assigns its distributed KV role.
func (b *Builder) BuildWithRole(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32, hwType HardwareType, loras []servingv1alpha2.LLMLoraAdapter, kvRole string) *appsv1.Deployment {
	selectorLabels := deploymentSelectorLabels(llmSvc)
	labels := b.deploymentLabels(llmSvc, selectorLabels)
	podSpec := b.preparePod(ctx, llmSvc, hwType)
	b.applyStorage(ctx, llmSvc, hwType, podSpec)
	b.applyPodPolicies(podSpec, selectorLabels)
	b.applyRuntime(llmSvc, hwType, loras, kvRole, podSpec)
	annotations := b.buildAnnotations(llmSvc, podSpec, kvRole)
	podAnnotations := b.podAnnotations(llmSvc)
	return newDeployment(llmSvc, replicas, selectorLabels, labels, annotations, podAnnotations, podSpec)
}

func deploymentSelectorLabels(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
	}
}

func (b *Builder) deploymentLabels(llmSvc *servingv1alpha2.LLMInferenceService, selectors map[string]string) map[string]string {
	labels := copyStringMap(llmSvc.Spec.Template.Labels)
	labels["serving.ckodex.com/model"] = strings.ReplaceAll(llmSvc.Spec.Model.Name, "/", ".")
	for key, value := range selectors {
		labels[key] = value
	}
	for key, value := range llmSvc.Spec.CostAllocationTags {
		labels["ckodex.cost/"+strings.ReplaceAll(key, ".", "-")] = value
	}
	if isMultiprocessLMCache(llmSvc) {
		labels["serving.ckodex.com/lmcache-mode"] = "multiprocess"
	}
	return labels
}

func (b *Builder) preparePod(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, hwType HardwareType) *corev1.PodSpec {
	podSpec := llmSvc.Spec.Template.Spec.DeepCopy()
	if len(podSpec.Containers) > 0 && podSpec.Containers[0].Image == "" && b.RuntimeImage != "" && b.RuntimeImage != api.VLLMImage {
		// A configured non-default runtime image is an operator-level override.
		// Seed it before hardware selection so it is not replaced by a built-in
		// CPU default.
		podSpec.Containers[0].Image = b.RuntimeImage
	}
	// Select a hardware-specific default before applying the operator fallback.
	// An image explicitly supplied by the workload remains authoritative because
	// ApplyHardwareOptimizations only changes empty/default CUDA images. The
	// standard operator image is intentionally left empty until after this step.
	ApplyHardwareOptimizations(ctx, hwType, podSpec)
	if len(podSpec.Containers) > 0 && podSpec.Containers[0].Image == "" && b.RuntimeImage != "" {
		podSpec.Containers[0].Image = b.RuntimeImage
	}
	if podSpec.TerminationGracePeriodSeconds == nil {
		grace := b.Defaults.TerminationGracePeriodSeconds
		if grace == 0 {
			grace = api.DefaultTerminationGracePeriod
		}
		podSpec.TerminationGracePeriodSeconds = ptr.To(grace)
	}
	if len(podSpec.Containers) > 0 {
		b.ensureResources(&podSpec.Containers[0])
		b.injectPreStop(&podSpec.Containers[0])
	}
	return podSpec
}

func (b *Builder) applyStorage(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, hwType HardwareType, podSpec *corev1.PodSpec) {
	if !b.applyLocalModelCache(ctx, llmSvc, podSpec) {
		if initializer := b.BuildStorageInitializer(ctx, llmSvc, hwType, nil); initializer != nil {
			podSpec.InitContainers = append([]corev1.Container{*initializer}, podSpec.InitContainers...)
		}
	}
	b.ensureModelVolume(llmSvc, podSpec)
	b.ensureModelVolumeMount(llmSvc, podSpec)
}

func (b *Builder) applyPodPolicies(podSpec *corev1.PodSpec, selectors map[string]string) {
	b.ensureHealthProbes(podSpec)
	b.ensureSecurityContext(podSpec)
	b.ensureTopologySpreadConstraints(podSpec, selectors)
}

func (b *Builder) applyRuntime(llmSvc *servingv1alpha2.LLMInferenceService, hwType HardwareType, loras []servingv1alpha2.LLMLoraAdapter, kvRole string, podSpec *corev1.PodSpec) {
	if len(loras) > 0 {
		b.applyLoraAdapters(loras, podSpec)
	}
	b.applyEngineSelection(llmSvc, podSpec, hwType)
	b.applyKVTransfer(llmSvc, podSpec, kvRole)
	b.ensureVLLMEnv(llmSvc, podSpec)
	b.applyGPUDeviceSelection(llmSvc, podSpec)
	if !isNilSPIREInjector(b.SPIRE) {
		b.SPIRE.InjectSidecar(podSpec, llmSvc)
	}
	b.injectVector(llmSvc, podSpec)
}

func (b *Builder) podAnnotations(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	annotations := copyStringMap(llmSvc.Spec.Template.Annotations)
	annotations["prometheus.io/scrape"] = "true"
	annotations["prometheus.io/port"] = "8000"
	if isMultiprocessLMCache(llmSvc) {
		annotations["serving.ckodex.com/lmcache-engine"] = llmSvc.Spec.KVCache.Transfer.LMCache.EngineRef.Name
	}
	return annotations
}

func newDeployment(llmSvc *servingv1alpha2.LLMInferenceService, replicas int32, selectors, labels, annotations, podAnnotations map[string]string, podSpec *corev1.PodSpec) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: llmSvc.Name, Namespace: llmSvc.Namespace, Labels: labels, Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas, Strategy: deploymentStrategyForReplicas(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: selectors},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: podAnnotations}, Spec: *podSpec,
			},
		},
	}
}

// BuildPrefill builds the dedicated prefill side of a PD deployment.
func (b *Builder) BuildPrefill(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, hwType HardwareType) *appsv1.Deployment {
	if llmSvc.Spec.Prefill == nil {
		return nil
	}
	clone := llmSvc.DeepCopy()
	clone.Spec.Template = *llmSvc.Spec.Prefill.Template.DeepCopy()
	clone.Spec.Replicas = llmSvc.Spec.Prefill.Replicas
	clone.Spec.Prefill = nil
	d := b.BuildWithRole(ctx, clone, replicaCount(clone.Spec.Replicas), hwType, nil, "kv_producer")
	d.Name = llmSvc.Name + "-prefill"
	if d.Annotations == nil {
		d.Annotations = map[string]string{}
	}
	d.Annotations["serving.ckodex.com/pd-disaggregation"] = "true"
	d.Spec.Selector.MatchLabels["serving.ckodex.com/role"] = "prefill"
	d.Spec.Template.Labels["serving.ckodex.com/role"] = "prefill"
	return d
}

func replicaCount(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}
