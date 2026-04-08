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
)

// SPIREInjector defines the interface for SPIRE sidecar injection.
type SPIREInjector interface {
	InjectSidecar(podSpec *corev1.PodSpec, llmSvc *servingv1alpha2.LLMInferenceService)
}

// Builder constructs Deployment objects for LLM inference.
type Builder struct {
	Client   client.Client
	Recorder record.EventRecorder
	SPIRE    SPIREInjector
}

// Build constructs the desired Deployment spec.
func (b *Builder) Build(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32, hwType HardwareType) *appsv1.Deployment {
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
	b.ensureVLLMEnv(podSpec)

	if b.SPIRE != nil {
		b.SPIRE.InjectSidecar(podSpec, llmSvc)
	}

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
	if uri == "" || strings.HasPrefix(uri, "modelpack://") || strings.HasPrefix(uri, "hf-mount://") {
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

	parts := strings.SplitN(uri, "://", 2)
	scheme := ""
	if len(parts) > 1 {
		scheme = parts[0]
	}

	initializerImage := api.StorageInitializerImage
	if scheme != "hf" && scheme != "huggingface" {
		initializerImage = api.CKodexStorageInitializerImage
	}

	if hwType == HardwareAppleSilicon {
		initializerImage = api.CKodexStorageInitializerImage
	}

	container := &corev1.Container{
		Name:  "storage-initializer",
		Image: initializerImage,
		Args:  []string{uri, api.ModelMountPath},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      api.ModelVolumeName,
				MountPath: api.ModelMountPath,
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
	}

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
					Driver: "model.csi.modelpack.org",
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
	default:
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: api.ModelVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}
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
}

func (b *Builder) ensureHealthProbes(podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	if c.ReadinessProbe == nil {
		c.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/v1/models", Port: intstr.FromInt32(8000)},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
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
	c := &podSpec.Containers[0]
	if c.SecurityContext == nil {
		c.SecurityContext = &corev1.SecurityContext{
			RunAsUser:    ptr.To(int64(10001)),
			RunAsNonRoot: ptr.To(true),
		}
	}
}

func (b *Builder) ensureVLLMEnv(podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	envs := map[string]string{
		"HOME":                    "/tmp",
		"TORCHINDUCTOR_CACHE_DIR": "/tmp",
		"VLLM_LOGGING_LEVEL":      "INFO",
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

func (b *Builder) buildAnnotations(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	ann := make(map[string]string)
	if llmSvc.Spec.SLO != nil {
		ann["ckodex.com/slo-p99-latency-ms"] = fmt.Sprintf("%d", llmSvc.Spec.SLO.TargetP99LatencyMs)
	}
	if llmSvc.Spec.Canary != nil {
		ann["ckodex.com/canary-weight"] = fmt.Sprintf("%d", llmSvc.Spec.Canary.Weight)
	}
	return ann
}
