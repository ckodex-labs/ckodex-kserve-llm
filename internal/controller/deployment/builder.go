package deployment

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
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
	OTEL_Endpoint           string // Contract: OTEL_EXPORTER_OTLP_ENDPOINT
	
	// AirGap configuration
	AirGappedMode bool
	LocalRegistry string // e.g. "local-registry.corp.internal"
}

// Build constructs the desired Deployment spec.
func (b *Builder) Build(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32, hwType HardwareType, loras []servingv1alpha2.LLMLoraAdapter) *appsv1.Deployment {
	labels := map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
		"serving.ckodex.com/model":     strings.ReplaceAll(llmSvc.Spec.Model.Name, "/", "."),
	}

	podSpec := llmSvc.Spec.Template.Spec.DeepCopy()

	// Apply Hardware Optimizations
	ApplyHardwareOptimizations(ctx, hwType, podSpec)

	// Phase 5 Hardening: Termination Grace Period
	if podSpec.TerminationGracePeriodSeconds == nil {
		podSpec.TerminationGracePeriodSeconds = ptr.To(int64(api.DefaultTerminationGracePeriod))
	}

	// Ensure primary container resources
	if len(podSpec.Containers) > 0 {
		c := &podSpec.Containers[0]
		b.ensureResources(c)
		b.injectPreStop(c)
	}

	// LocalModelCache Logic
	skipInitializer := b.applyLocalModelCache(ctx, llmSvc, podSpec)

	if !skipInitializer {
		if initContainer := b.BuildStorageInitializer(ctx, llmSvc, hwType, nil); initContainer != nil {
			podSpec.InitContainers = append([]corev1.Container{*initContainer}, podSpec.InitContainers...)
		}
	}

	b.ensureModelVolume(llmSvc, podSpec)
	b.ensureModelVolumeMount(podSpec)
	b.ensureHealthProbes(podSpec)
	b.ensureSecurityContext(podSpec)
	b.ensureTopologySpreadConstraints(podSpec, labels)

	if len(loras) > 0 {
		b.applyLoraAdapters(loras, podSpec)
	}

	b.applyEngineSelection(llmSvc, podSpec, hwType)

	b.ensureVLLMEnv(llmSvc, podSpec)

	if b.SPIRE != nil {
		b.SPIRE.InjectSidecar(podSpec, llmSvc)
	}

	// OIS v0.1: Refined Telemetry Sinks (Vector Sidecar)
	b.injectVector(llmSvc, podSpec)

	for k, v := range llmSvc.Spec.CostAllocationTags {
		labels["ckodex.cost/"+strings.ReplaceAll(k, ".", "-")] = v
	}

	annotations := b.buildAnnotations(llmSvc)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        llmSvc.Name,
			Namespace:   llmSvc.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "8000",
					},
				},
				Spec: *podSpec,
			},
		},
	}
}

// BuildStorageInitializer creates an init container for model download.
// Returns nil if a ready LocalModelCache is found (enabling zero-copy bypass).
// activeLMC is optional; if provided, it take precedence over listing from the client.
func (b *Builder) BuildStorageInitializer(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, hwType HardwareType, activeLMC *servingv1alpha2.LocalModelCache) *corev1.Container {
	uri := llmSvc.Spec.Model.URI
	if uri == "" || strings.HasPrefix(uri, "modelpack://") || strings.HasPrefix(uri, "hf-mount://") || strings.HasPrefix(uri, "pvc://") {
		return nil
	}

	// Zero-copy bypass: If an active LMC is provided and is ready, or if one is found in the cluster.
	if activeLMC != nil {
		isReady := false
		for _, ns := range activeLMC.Status.NodeStatuses {
			if ns.Phase == "Ready" {
				isReady = true
				break
			}
		}
		if isReady {
			return nil
		}
	} else if b.isLocalModelCacheReady(ctx, uri) {
		return nil
	}

	// Dynamic hardware-aware model selection (Experimental)
	if b.EnableHardwareSelection && llmSvc.Spec.Model.HardwareAware {
		uri = b.transformModelURI(uri, hwType)
	}

	parts := strings.SplitN(uri, "://", 2)
	scheme := ""
	if len(parts) > 1 {
		scheme = parts[0]
	}

	initializerImage := api.StorageInitializerImage
	if scheme != "hf" && scheme != "huggingface" {
		initializerImage = api.CKodexStorageInitializerImage
	}

	if b.AirGappedMode && b.LocalRegistry != "" {
		initializerImage = b.rewriteImage(initializerImage)
		// Storage initialized in air-gap expects converted URIs (hf:// -> oci://)
		uri = b.storageResolveAirGap(uri)
	}

	if hwType == HardwareAppleSilicon {
		initializerImage = api.CKodexStorageInitializerImage
		if b.AirGappedMode && b.LocalRegistry != "" {
			initializerImage = b.rewriteImage(initializerImage)
		}
	}

	container := &corev1.Container{
		Name:  "storage-initializer",
		Image: initializerImage,
		Args:  []string{uri, api.ModelMountPath},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      api.ModelVolumeName,
				MountPath: api.ModelMountPath,
				ReadOnly:  false, // Writable for download
			},
			{
				Name:      "tmp-scratch",
				MountPath: "/tmp",
			},
		},
	}

	if llmSvc.Spec.Model.Storage != nil {
		if llmSvc.Spec.Model.Storage.VaultRef != "" {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  "VAULT_PATH",
				Value: llmSvc.Spec.Model.Storage.VaultRef,
			})
		}
		if llmSvc.Spec.Model.Storage.VaultAddr != "" {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  "VAULT_ADDR",
				Value: llmSvc.Spec.Model.Storage.VaultAddr,
			})
		}
		if llmSvc.Spec.Model.Storage.SecretRef != nil {
			container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: *llmSvc.Spec.Model.Storage.SecretRef,
				},
			})
		}
		// Managed ExternalSecret injection (M3 Phase 5)
		if llmSvc.Spec.Model.Storage.ExternalSecret != nil {
			targetName := llmSvc.Name + "-external-secret"
			container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: targetName},
				},
			})
		}
	}

	// Apply universal restricted security context
	b.applyRestrictedSecurityContext(container)

	return container
}

func (b *Builder) ensureResources(c *corev1.Container) {
	if c.Resources.Requests == nil {
		c.Resources.Requests = make(corev1.ResourceList)
	}
	if _, ok := c.Resources.Requests[corev1.ResourceCPU]; !ok {
		c.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(api.DefaultVLLMCPURequest)
	}
	if _, ok := c.Resources.Requests[corev1.ResourceMemory]; !ok {
		c.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(api.DefaultVLLMMemoryRequest)
	}
	if c.Resources.Limits == nil {
		c.Resources.Limits = make(corev1.ResourceList)
	}
	if _, ok := c.Resources.Limits[corev1.ResourceCPU]; !ok {
		c.Resources.Limits[corev1.ResourceCPU] = c.Resources.Requests[corev1.ResourceCPU]
	}
	if _, ok := c.Resources.Limits[corev1.ResourceMemory]; !ok {
		c.Resources.Limits[corev1.ResourceMemory] = c.Resources.Requests[corev1.ResourceMemory]
	}
}

func (b *Builder) injectPreStop(c *corev1.Container) {
	if c.Lifecycle == nil {
		c.Lifecycle = &corev1.Lifecycle{}
	}
	if c.Lifecycle.PreStop == nil {
		c.Lifecycle.PreStop = &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/sh", "-c", "sleep 15"},
			},
		}
	}
}

func (b *Builder) applyLocalModelCache(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) bool {
	activeLMC := b.getReadyLMC(ctx, llmSvc.Spec.Model.URI)
	if activeLMC == nil {
		return false
	}

	readyNodes := []string{}
	for _, ns := range activeLMC.Status.NodeStatuses {
		if ns.Phase == "Ready" {
			readyNodes = append(readyNodes, ns.NodeName)
		}
	}

	if len(readyNodes) == 0 {
		return false
	}

	if podSpec.Affinity == nil {
		podSpec.Affinity = &corev1.Affinity{}
	}
	if podSpec.Affinity.NodeAffinity == nil {
		podSpec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{
			{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{
						Key:      "kubernetes.io/hostname",
						Operator: corev1.NodeSelectorOpIn,
						Values:   readyNodes,
					},
				},
			},
		},
	}

	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: api.ModelVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: fmt.Sprintf("/tmp/ckodex/models/%s", activeLMC.Name),
				Type: ptr.To(corev1.HostPathDirectoryOrCreate),
			},
		},
	})
	return true
}

func (b *Builder) getReadyLMC(ctx context.Context, modelURI string) *servingv1alpha2.LocalModelCache {
	var lmcList servingv1alpha2.LocalModelCacheList
	if err := b.Client.List(ctx, &lmcList); err != nil {
		return nil
	}

	for _, lmc := range lmcList.Items {
		if lmc.Spec.SourceModelURI == modelURI {
			// Check if at least one node is ready
			for _, ns := range lmc.Status.NodeStatuses {
				if ns.Phase == "Ready" {
					return &lmc
				}
			}
		}
	}
	return nil
}

func (b *Builder) transformModelURI(uri string, hwType HardwareType) string {
	if !strings.HasPrefix(uri, "oci://") {
		return uri
	}

	suffix := "-cpu"
	switch hwType {
	case HardwareNVIDIA:
		suffix = "-nvidia"
	case HardwareAppleSiliconMPS:
		suffix = "-mps"
	case HardwareAMD:
		suffix = "-rocm"
	}

	// Append suffix to the tag or digest
	if strings.Contains(uri, "@sha256:") {
		// Digests are immutable; we can't easily suffix them without a mapping.
		// For now, only suffix tags.
		return uri
	}

	if strings.Contains(uri, ":") {
		return uri + suffix
	}
	return uri + ":latest" + suffix
}

func (b *Builder) isLocalModelCacheReady(ctx context.Context, modelURI string) bool {
	return b.getReadyLMC(ctx, modelURI) != nil
}

func (b *Builder) ensureModelVolume(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	for _, v := range podSpec.Volumes {
		if v.Name == api.ModelVolumeName {
			return
		}
	}

	uri := llmSvc.Spec.Model.URI
	switch {
	case strings.HasPrefix(uri, "modelpack://"):
		ref := strings.TrimPrefix(uri, "modelpack://")
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: api.ModelVolumeName,
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver:           "model.csi.modelpack.org",
					VolumeAttributes: map[string]string{"modelRef": ref},
				},
			},
		})
	case strings.HasPrefix(uri, "hf-mount://"):
		repoPath := strings.TrimPrefix(uri, "hf-mount://")
		repo := repoPath
		revision := ""
		if idx := strings.Index(repoPath, "@"); idx != -1 {
			repo = repoPath[:idx]
			revision = repoPath[idx+1:]
		}

		attrs := map[string]string{
			"repo": repo,
		}
		if revision != "" {
			attrs["revision"] = revision
		}
		if llmSvc.Spec.Model.Storage != nil && llmSvc.Spec.Model.Storage.SecretRef != nil {
			attrs["tokenSecret"] = llmSvc.Spec.Model.Storage.SecretRef.Name
		}

		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: api.ModelVolumeName,
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver:           api.HFMountCSIDriver,
					VolumeAttributes: attrs,
					ReadOnly:         ptr.To(true),
				},
			},
		})
	case strings.HasPrefix(uri, "pvc://"):
		pvcName := strings.TrimPrefix(uri, "pvc://")
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: api.ModelVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
					ReadOnly:  true,
				},
			},
		})
	default:
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: api.ModelVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Always add the 4Gi /tmp scratch space to support ReadOnlyRootFilesystem
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "tmp-scratch",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: ptr.To(resource.MustParse("4Gi")),
			},
		},
	})
}

func (b *Builder) ensureModelVolumeMount(podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	for _, m := range c.VolumeMounts {
		if m.Name == api.ModelVolumeName {
			return
		}
	}
	c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
		Name:      api.ModelVolumeName,
		MountPath: api.ModelMountPath,
		ReadOnly:  true,
	})

	// Inject /tmp scratch mount
	foundTmp := false
	for _, m := range c.VolumeMounts {
		if m.MountPath == "/tmp" {
			foundTmp = true
			break
		}
	}
	if !foundTmp {
		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
			Name:      "tmp-scratch",
			MountPath: "/tmp",
		})
	}
}

func (b *Builder) ensureHealthProbes(podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	if c.ReadinessProbe == nil {
		c.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/v2/health/ready", Port: intstr.FromInt32(8000)},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			SuccessThreshold:    3, // Pristine requirement: ensure stability before routing
		}
	}
	if c.LivenessProbe == nil {
		c.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(8000)},
			},
			InitialDelaySeconds: 120,
			PeriodSeconds:       15,
		}
	}
}

func (b *Builder) ensureSecurityContext(podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	// Apply to all containers in the pod
	for i := range podSpec.Containers {
		b.applyRestrictedSecurityContext(&podSpec.Containers[i])
	}
	for i := range podSpec.InitContainers {
		b.applyRestrictedSecurityContext(&podSpec.InitContainers[i])
	}

	// Add Pod-level restricted security context
	if podSpec.SecurityContext == nil {
		podSpec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup:        ptr.To(int64(65532)),
			RunAsNonRoot:   ptr.To(true),
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		}
	}
}

func (b *Builder) applyRestrictedSecurityContext(c *corev1.Container) {
	if c.SecurityContext == nil {
		c.SecurityContext = &corev1.SecurityContext{}
	}
	c.SecurityContext.RunAsUser = ptr.To(int64(65532))
	c.SecurityContext.RunAsGroup = ptr.To(int64(65532))
	c.SecurityContext.RunAsNonRoot = ptr.To(true)
	c.SecurityContext.ReadOnlyRootFilesystem = ptr.To(true)
	c.SecurityContext.AllowPrivilegeEscalation = ptr.To(false)
	c.SecurityContext.Capabilities = &corev1.Capabilities{
		Drop: []corev1.Capability{"ALL"},
	}
	c.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}
}

func (b *Builder) ensureVLLMEnv(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]

	// OIS v0.1 Identity Context
	modelID := llmSvc.Spec.Model.Name
	if modelID == "" {
		modelID = strings.ReplaceAll(llmSvc.Spec.Model.URI, "/", ".")
	}
	engineURN := observability.URN("engine", llmSvc.Spec.Engine)
	if llmSvc.Spec.Engine == "" {
		engineURN = observability.URN("engine", "vllm")
	}

	envs := map[string]string{
		"HOME":                    "/tmp",
		"TORCHINDUCTOR_CACHE_DIR": "/tmp",
		"VLLM_LOGGING_LEVEL":      "INFO",

		// OIS Core Profile (Section 26.1)
		"OIS_MODEL_ID":   modelID,
		"OIS_MODEL_URN":  observability.URN("model", modelID),
		"OIS_ENGINE_URN": engineURN,
		"OIS_ACTOR_URN":  observability.URN("actor", llmSvc.Namespace), // Default to namespace authority
	}
	for k, v := range envs {
		found := false
		for _, e := range c.Env {
			if e.Name == k {
				found = true
				break
			}
		}
		if !found {
			c.Env = append(c.Env, corev1.EnvVar{Name: k, Value: v})
		}
	}
}

func (b *Builder) injectVector(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	// 1. Determine sink configuration (Contract: User Spec > Operator Config > Default)
	sinkType := "stdout"

	if b.OTEL_Endpoint != "" {
		sinkType = "otlp"
	}

	if llmSvc.Spec.Observability != nil && llmSvc.Spec.Observability.Sink != nil {
		sinkType = llmSvc.Spec.Observability.Sink.Type
	}

	// 2. Inject sidecar if telemetry is enabled or OTLP sink is active
	if sinkType != "stdout" || b.OTEL_Endpoint != "" {
		// The ConfigMap is managed by the reconciler; here we just point to its name.
		// Convention: <name>-vector-config
		configMapName := llmSvc.Name + "-vector-config"
		observability.InjectVectorSidecar(podSpec, configMapName)
	}
}

func (b *Builder) buildAnnotations(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	ann := make(map[string]string)
	if llmSvc.Spec.Canary != nil {
		ann["ckodex.com/canary-weight"] = fmt.Sprintf("%d", llmSvc.Spec.Canary.Weight)
	}

	// Phase 5: Istio Sidecar Injection for ToolSurface DPI
	if llmSvc.Spec.ToolSurface != nil && (len(llmSvc.Spec.ToolSurface.AllowedAPIs) > 0 || len(llmSvc.Spec.ToolSurface.AllowedCIDRs) > 0) {
		ann["sidecar.istio.io/inject"] = "true"
		ann["sidecar.istio.io/rewriteAppHTTPProbers"] = "true"
		ann["sidecar.istio.io/discoveryNamespaces"] = llmSvc.Namespace
	}

	return ann
}

// applyLoraAdapters injects --enable-lora and mounts PVCs for all active adapters.
func (b *Builder) applyLoraAdapters(loras []servingv1alpha2.LLMLoraAdapter, podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]

	// 1. Ensure --enable-lora and --lora-modules (if we want to pre-load)
	// We only set --enable-lora as the hot-swap controller handles the dynamic registration.
	foundEnabledLora := false
	for _, arg := range c.Args {
		if arg == "--enable-lora" {
			foundEnabledLora = true
			break
		}
	}
	if !foundEnabledLora {
		c.Args = append(c.Args, "--enable-lora")
	}

	// 2. Add Volumes and VolumeMounts for each adapter's LocalModelCache
	for _, lora := range loras {
		volName := fmt.Sprintf("lora-%s", lora.Name)
		pvcName := fmt.Sprintf("lora-%s", lora.Name) // Matches adapter controller naming

		// Add Volume (using same HostPath bypass as base LMC for zero-copy performance)
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/tmp/ckodex/models/%s", pvcName),
					Type: ptr.To(corev1.HostPathDirectoryOrCreate),
				},
			},
		})

		// Add Mount
		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: fmt.Sprintf("%s/lora-%s", api.ModelMountPath, lora.Name),
			ReadOnly:  true,
		})
	}
}

// applyEngineSelection selects the container image and arguments based on the engine.
func (b *Builder) applyEngineSelection(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec, hwType HardwareType) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]

	engine := llmSvc.Spec.Engine
	if engine == "" {
		engine = "vllm"
	}

	switch engine {
	case "quant-cpp":
		c.Image = api.QuantCppImage
		if b.AirGappedMode && b.LocalRegistry != "" {
			c.Image = b.rewriteImage(c.Image)
		}
		b.ensureQuantCppArgs(llmSvc, c, hwType)
	default:
		// Default to vllm image if not already set by template
		if c.Image == "" {
			c.Image = api.VLLMImage
		}
		if b.AirGappedMode && b.LocalRegistry != "" {
			c.Image = b.rewriteImage(c.Image)
		}
		// vLLM args are typically handled by applying WellKnown config or user spec.
		// If no args provided, we add safe defaults.
		if len(c.Args) == 0 {
			c.Args = []string{
				"--model", api.ModelMountPath,
				"--host", "0.0.0.0",
				"--port", "8000",
			}
		}
	}
}

// ensureQuantCppArgs configures arguments for the llama.cpp / quant-cpp engine.
func (b *Builder) ensureTopologySpreadConstraints(podSpec *corev1.PodSpec, labels map[string]string) {
	if len(podSpec.TopologySpreadConstraints) > 0 {
		return
	}
	podSpec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       "topology.kubernetes.io/zone",
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
		{
			MaxSkew:           1,
			TopologyKey:       "kubernetes.io/hostname",
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}
}

func (b *Builder) ensureQuantCppArgs(llmSvc *servingv1alpha2.LLMInferenceService, c *corev1.Container, hwType HardwareType) {
	modelPath := api.ModelMountPath

	foundModelArg := false
	for _, arg := range c.Args {
		if arg == "-m" || arg == "--model" {
			foundModelArg = true
			break
		}
	}

	if !foundModelArg {
		c.Args = append(c.Args, "-m", modelPath)
	}

	// Long-context support: check annotations for ctx-size
	if ctxSize, ok := llmSvc.Annotations["ckodex.com/ctx-size"]; ok {
		c.Args = append(c.Args, "--ctx-size", ctxSize)
	}

	// Apple Silicon Optimization: auto-detect GPU layers if not specified
	if hwType == HardwareAppleSilicon {
		foundNGL := false
		for _, arg := range c.Args {
			if arg == "-ngl" || arg == "--n-gpu-layers" {
				foundNGL = true
				break
			}
		}
		if !foundNGL {
			// On Apple Silicon, we typically want max GPU layers (Metal)
			c.Args = append(c.Args, "--n-gpu-layers", "99")
		}
	}

	foundHost := false
	for _, arg := range c.Args {
		if arg == "--host" {
			foundHost = true
			break
		}
	}
	if !foundHost {
		c.Args = append(c.Args, "--host", "0.0.0.0", "--port", "8000")
	}
}

// rewriteImage replaces the registry part of an image string with the local registry.
func (b *Builder) rewriteImage(image string) string {
	if b.LocalRegistry == "" {
		return image
	}
	// Split by '/' to find the registry
	parts := strings.Split(image, "/")
	if len(parts) > 1 {
		// If the first part looks like a registry (contains . or :) or matches known prefixes
		// Alternatively, just prepend our local registry and keep the rest as the path
		// Convention: {local-registry}/{original-path}
		return fmt.Sprintf("%s/%s", b.LocalRegistry, strings.Join(parts, "/"))
	}
	// Simple images (e.g. "nginx")
	return fmt.Sprintf("%s/%s", b.LocalRegistry, image)
}

// storageResolveAirGap uses the storage package to rewrite URIs.
func (b *Builder) storageResolveAirGap(uri string) string {
	// We use the storage package's resolution logic.
	// We'll import "github.com/ckodex-labs/kserve-llm-operator/internal/storage"
	return ResolveAirGappedURI(uri, b.LocalRegistry)
}
