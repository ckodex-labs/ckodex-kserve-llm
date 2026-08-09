package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
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
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
	"github.com/ckodex-labs/kserve-llm-operator/internal/storage"
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
	OTEL_Endpoint           string // Contract: OTEL_EXPORTER_OTLP_ENDPOINT

	// AirGap configuration
	AirGappedMode      bool
	LocalRegistry      string // e.g. "local-registry.corp.internal"
	LocalCosignKeyPath string
	RuntimeImage       string
	HFInitializerImage string
	HFMirrorURL        string
	Defaults           operatorconfig.DefaultsConfig
}

// Build constructs the desired Deployment spec.
func (b *Builder) Build(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32, hwType HardwareType, loras []servingv1alpha2.LLMLoraAdapter) *appsv1.Deployment {
	return b.BuildWithRole(ctx, llmSvc, replicas, hwType, loras, "")
}

// BuildWithRole builds a model Deployment and, when configured, assigns its
// distributed KV-transfer role (producer, consumer, or both) to vLLM.
func (b *Builder) BuildWithRole(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32, hwType HardwareType, loras []servingv1alpha2.LLMLoraAdapter, kvRole string) *appsv1.Deployment {
	selectorLabels := deploymentSelectorLabels(llmSvc)
	labels := b.buildDeploymentLabels(llmSvc, selectorLabels)
	podSpec := b.buildPodSpec(ctx, llmSvc, hwType, loras, kvRole, selectorLabels)
	annotations := b.buildAnnotations(llmSvc, podSpec, kvRole)
	podAnnotations := b.buildPodAnnotations(llmSvc)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        llmSvc.Name,
			Namespace:   llmSvc.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: deploymentStrategyForReplicas(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: *podSpec,
			},
		},
	}
}

func deploymentSelectorLabels(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
		"serving.ckodex.com/model":     strings.ReplaceAll(llmSvc.Spec.Model.Name, "/", "."),
	}
}

func (b *Builder) buildDeploymentLabels(llmSvc *servingv1alpha2.LLMInferenceService, selectorLabels map[string]string) map[string]string {
	labels := copyStringMap(llmSvc.Spec.Template.Labels)
	for key, value := range selectorLabels {
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

func (b *Builder) buildPodSpec(
	ctx context.Context,
	llmSvc *servingv1alpha2.LLMInferenceService,
	hwType HardwareType,
	loras []servingv1alpha2.LLMLoraAdapter,
	kvRole string,
	selectorLabels map[string]string,
) *corev1.PodSpec {
	podSpec := llmSvc.Spec.Template.Spec.DeepCopy()
	if len(podSpec.Containers) > 0 && podSpec.Containers[0].Image == "" && b.RuntimeImage != "" {
		podSpec.Containers[0].Image = b.RuntimeImage
	}
	ApplyHardwareOptimizations(ctx, hwType, podSpec)
	b.applyStorage(ctx, llmSvc, hwType, podSpec)
	b.applyPodHardening(podSpec, selectorLabels)
	b.applyRuntimeWiring(llmSvc, hwType, loras, kvRole, podSpec)
	return podSpec
}

func (b *Builder) applyStorage(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, hwType HardwareType, podSpec *corev1.PodSpec) {
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
	if !b.applyLocalModelCache(ctx, llmSvc, podSpec) {
		if initContainer := b.BuildStorageInitializer(ctx, llmSvc, hwType, nil); initContainer != nil {
			podSpec.InitContainers = append([]corev1.Container{*initContainer}, podSpec.InitContainers...)
		}
	}
	b.ensureModelVolume(llmSvc, podSpec)
	b.ensureModelVolumeMount(llmSvc, podSpec)
}

func (b *Builder) applyPodHardening(podSpec *corev1.PodSpec, selectorLabels map[string]string) {
	b.ensureHealthProbes(podSpec)
	b.ensureSecurityContext(podSpec)
	b.ensureTopologySpreadConstraints(podSpec, selectorLabels)
}

func (b *Builder) applyRuntimeWiring(
	llmSvc *servingv1alpha2.LLMInferenceService,
	hwType HardwareType,
	loras []servingv1alpha2.LLMLoraAdapter,
	kvRole string,
	podSpec *corev1.PodSpec,
) {
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

func (b *Builder) buildPodAnnotations(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	annotations := copyStringMap(llmSvc.Spec.Template.Annotations)
	annotations["prometheus.io/scrape"] = "true"
	annotations["prometheus.io/port"] = "8000"
	if isMultiprocessLMCache(llmSvc) {
		annotations["serving.ckodex.com/lmcache-engine"] = llmSvc.Spec.KVCache.Transfer.LMCache.EngineRef.Name
	}
	return annotations
}

// BuildPrefill builds the dedicated prefill side of a PD deployment. It uses
// the user-provided prefill template while retaining the model/storage wiring
// of the primary service.
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

func (b *Builder) applyKVTransfer(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec, role string) {
	if len(podSpec.Containers) == 0 || llmSvc.Spec.KVCache == nil || llmSvc.Spec.KVCache.Transfer == nil {
		return
	}
	t := llmSvc.Spec.KVCache.Transfer
	if role == "" {
		role = t.Role
	}
	if role == "" {
		role = "kv_both"
	}
	connector := map[string]string{
		"nixl": "NixlConnector", "lmcache": "LMCacheConnectorV1", "mooncake": "MooncakeConnector",
	}[t.Connector]
	if connector == "" {
		return
	}
	extra := parseKVExtraConfig(t.ExtraConfig)
	if t.LMCache != nil && t.LMCache.Mode != servingv1alpha2.LMCacheModeMultiprocess {
		if t.LMCache.ChunkSize != nil {
			if _, exists := extra["chunk_size"]; !exists {
				extra["chunk_size"] = *t.LMCache.ChunkSize
			}
		}
		if t.LMCache.LocalCPU != nil {
			if _, exists := extra["local_cpu"]; !exists {
				extra["local_cpu"] = *t.LMCache.LocalCPU
			}
		}
		if t.LMCache.LocalCPUSizeGiB != nil {
			if _, exists := extra["max_local_cpu_size"]; !exists {
				extra["max_local_cpu_size"] = *t.LMCache.LocalCPUSizeGiB
			}
		}
	}
	c := &podSpec.Containers[0]
	if t.LMCache != nil && t.LMCache.Mode == servingv1alpha2.LMCacheModeMultiprocess && t.LMCache.EngineRef != nil {
		configEnv := "CKODEX_LMCACHE_KV_TRANSFER_CONFIG"
		if !hasEnv(c.Env, configEnv) {
			c.Env = append(c.Env, corev1.EnvVar{
				Name: configEnv,
				ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: t.LMCache.EngineRef.Name + "-connection"},
					Key:                  "kv-transfer-config",
				}},
			})
		}
		if !hasArg(c.Args, "--kv-transfer-config") {
			c.Args = append(c.Args, "--kv-transfer-config", "$("+configEnv+")")
		}
		podSpec.HostIPC = true
		setEnvDefault(c, "PYTHONHASHSEED", "0")
		return
	}
	cfg, err := json.Marshal(map[string]interface{}{
		"kv_connector": connector, "kv_role": role, "kv_connector_extra_config": extra,
	})
	if err != nil {
		return
	}
	if !hasArg(c.Args, "--kv-transfer-config") {
		c.Args = append(c.Args, "--kv-transfer-config", string(cfg))
	}
	// Connector configuration such as LMCache's LMCACHE_CONFIG_FILE belongs
	// in the runtime environment, not in a command-line secret. Preserve an
	// explicit pod-template value and only add missing names.
	for _, env := range t.Env {
		found := false
		for _, existing := range c.Env {
			if existing.Name == env.Name {
				found = true
				break
			}
		}
		if !found {
			c.Env = append(c.Env, *env.DeepCopy())
		}
	}
	if strings.EqualFold(t.Connector, "lmcache") {
		// KServe's LMCache integration requires the experimental vLLM adapter
		// flag on the supported runtime line. Keep it as a default, while
		// preserving an explicit pod-template or transfer.env override.
		if !hasEnv(c.Env, "LMCACHE_USE_EXPERIMENTAL") {
			c.Env = append(c.Env, corev1.EnvVar{Name: "LMCACHE_USE_EXPERIMENTAL", Value: "True"})
		}
		if t.LMCache != nil {
			setEnvDefault(c, "PYTHONHASHSEED", "0")
			if t.LMCache.ChunkSize != nil {
				setEnvDefault(c, "LMCACHE_CHUNK_SIZE", strconv.FormatInt(int64(*t.LMCache.ChunkSize), 10))
			}
			if t.LMCache.LocalCPU != nil {
				setEnvDefault(c, "LMCACHE_LOCAL_CPU", strconv.FormatBool(*t.LMCache.LocalCPU))
			}
			if t.LMCache.LocalCPUSizeGiB != nil {
				setEnvDefault(c, "LMCACHE_MAX_LOCAL_CPU_SIZE", strconv.FormatInt(int64(*t.LMCache.LocalCPUSizeGiB), 10))
			}
		}
	}
}

func setEnvDefault(c *corev1.Container, name, value string) {
	if !hasEnv(c.Env, name) {
		c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
	}
}

func isMultiprocessLMCache(llmSvc *servingv1alpha2.LLMInferenceService) bool {
	return llmSvc.Spec.KVCache != nil && llmSvc.Spec.KVCache.Transfer != nil &&
		llmSvc.Spec.KVCache.Transfer.LMCache != nil &&
		llmSvc.Spec.KVCache.Transfer.LMCache.Mode == servingv1alpha2.LMCacheModeMultiprocess &&
		llmSvc.Spec.KVCache.Transfer.LMCache.EngineRef != nil
}

func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src)+4)
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func hasEnv(env []corev1.EnvVar, name string) bool {
	for _, item := range env {
		if item.Name == name {
			return true
		}
	}
	return false
}

// parseKVExtraConfig keeps the portable CRD string map while emitting the
// scalar types expected by vLLM connector implementations. JSON literals are
// accepted for structured values; unparseable values remain strings.
func parseKVExtraConfig(src map[string]string) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		if parsed, err := strconv.ParseBool(value); err == nil {
			dst[key] = parsed
			continue
		}
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			dst[key] = parsed
			continue
		}
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			dst[key] = parsed
			continue
		}
		var structured interface{}
		if json.Unmarshal([]byte(value), &structured) == nil {
			switch structured.(type) {
			case map[string]interface{}, []interface{}:
				dst[key] = structured
				continue
			}
		}
		dst[key] = value
	}
	return dst
}

func deploymentStrategyForReplicas(replicas int32) appsv1.DeploymentStrategy {
	if replicas <= 1 {
		return appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}
	}

	return appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
			MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
	}
}

func isNilSPIREInjector(injector SPIREInjector) bool {
	if injector == nil {
		return true
	}

	v := reflect.ValueOf(injector)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// BuildStorageInitializer creates an init container for model download.
// Returns nil if a ready LocalModelCache is found (enabling zero-copy bypass).
// activeLMC is optional; if provided, it take precedence over listing from the client.
func (b *Builder) BuildStorageInitializer(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, hwType HardwareType, activeLMC *servingv1alpha2.LocalModelCache) *corev1.Container {
	uri := effectiveModelURI(llmSvc.Spec.Model)
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

	if b.AirGappedMode && b.LocalRegistry != "" {
		// Storage initialized in air-gap expects converted URIs (hf:// -> oci://)
		uri = b.storageResolveAirGap(uri)
	}

	parts := strings.SplitN(uri, "://", 2)
	scheme := ""
	if len(parts) > 1 {
		scheme = parts[0]
	}

	initializerImage := api.HuggingFaceInitializerImage
	if b.HFInitializerImage != "" {
		initializerImage = b.HFInitializerImage
	}
	if !isHuggingFaceScheme(scheme) {
		initializerImage = b.Defaults.CustomStorageInitializerImage
		if initializerImage == "" {
			initializerImage = api.CKodexStorageInitializerImage
		}
	}
	if b.AirGappedMode && b.LocalRegistry != "" {
		initializerImage = b.rewriteImage(initializerImage)
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
	if isHuggingFaceScheme(scheme) {
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "HOME", Value: "/tmp"},
			corev1.EnvVar{Name: "HF_HOME", Value: "/tmp/huggingface"},
			corev1.EnvVar{Name: "HF_HUB_DISABLE_UPDATE_CHECK", Value: "1"},
		)
		if scheme == "hf-mirror" && b.HFMirrorURL != "" {
			container.Env = append(container.Env, corev1.EnvVar{Name: "HF_ENDPOINT", Value: b.HFMirrorURL})
		}
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

	if b.LocalCosignKeyPath != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "CKODEX_LOCAL_COSIGN_KEY_PATH",
			Value: b.LocalCosignKeyPath,
		})
		b.copyMatchingVolumeMounts(container, &llmSvc.Spec.Template.Spec, b.LocalCosignKeyPath)
	}

	// Apply universal restricted security context
	b.applyRestrictedSecurityContext(container)

	return container
}

func effectiveModelURI(model servingv1alpha2.ModelSpec) string {
	if model.Revision == "" {
		return model.URI
	}
	return model.URI + "@" + model.Revision
}

func (b *Builder) copyMatchingVolumeMounts(container *corev1.Container, podSpec *corev1.PodSpec, filePath string) {
	targetDir := filepath.Clean(filepath.Dir(filePath))
	for _, existing := range podSpec.Containers {
		for _, mount := range existing.VolumeMounts {
			mountPath := filepath.Clean(mount.MountPath)
			if mountPath != targetDir && mountPath != filepath.Clean(filePath) && !strings.HasPrefix(targetDir, mountPath+string(filepath.Separator)) {
				continue
			}
			if hasVolumeMount(container.VolumeMounts, mount) {
				continue
			}
			container.VolumeMounts = append(container.VolumeMounts, mount)
		}
	}
}

func hasVolumeMount(mounts []corev1.VolumeMount, candidate corev1.VolumeMount) bool {
	for _, mount := range mounts {
		if mount.Name == candidate.Name &&
			mount.MountPath == candidate.MountPath &&
			mount.SubPath == candidate.SubPath &&
			mount.ReadOnly == candidate.ReadOnly {
			return true
		}
	}
	return false
}

func (b *Builder) ensureResources(c *corev1.Container) {
	if c.Resources.Requests == nil {
		c.Resources.Requests = make(corev1.ResourceList)
	}
	if _, ok := c.Resources.Requests[corev1.ResourceCPU]; !ok {
		cpu := b.Defaults.VLLMCPURequest
		if cpu == "" {
			cpu = api.DefaultVLLMCPURequest
		}
		c.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if _, ok := c.Resources.Requests[corev1.ResourceMemory]; !ok {
		memory := b.Defaults.VLLMMemoryRequest
		if memory == "" {
			memory = api.DefaultVLLMMemoryRequest
		}
		c.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(memory)
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
				// Keep cleanup inside this container. Killing arbitrary PIDs returned
				// by nvidia-smi could terminate another tenant's workload on a
				// time-sliced GPU; terminating PID 1 lets the driver reclaim this
				// container's CUDA context without crossing that boundary.
				Command: []string{"/bin/sh", "-c", "kill -TERM 1 2>/dev/null || true; sleep 10; kill -KILL 1 2>/dev/null || true; sleep 5"},
			},
		}
	}
}

func (b *Builder) applyGPUDeviceSelection(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 || llmSvc.Spec.Parallelism == nil || len(llmSvc.Spec.Parallelism.GPUDevices) == 0 {
		return
	}
	setEnvDefault(&podSpec.Containers[0], "NVIDIA_VISIBLE_DEVICES", strings.Join(llmSvc.Spec.Parallelism.GPUDevices, ","))
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

func isHuggingFaceScheme(scheme string) bool {
	return scheme == "hf" || scheme == "hf-mirror"
}

func parsePVCURI(uri string) (claim, subPath string) {
	ref := strings.TrimPrefix(uri, "pvc://")
	parts := strings.SplitN(ref, "/", 2)
	claim = parts[0]
	if len(parts) == 2 {
		subPath = strings.Trim(parts[1], "/")
	}
	return claim, subPath
}

func (b *Builder) ensureModelVolume(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	modelVolumeFound := false
	for _, v := range podSpec.Volumes {
		if v.Name == api.ModelVolumeName {
			modelVolumeFound = true
			break
		}
	}

	if !modelVolumeFound {
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
			// PV+PVC are provisioned by HFCSIReconciler before the pod is built.
			// The PVC name is the same deterministic formula used in hfcsi_reconciler.go.
			pvcName := fmt.Sprintf("hf-model-%s-%s", llmSvc.Namespace, llmSvc.Name)
			if len(pvcName) > 253 {
				pvcName = pvcName[:253]
			}
			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: api.ModelVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
						ReadOnly:  true,
					},
				},
			})
		case strings.HasPrefix(uri, "pvc://"):
			pvcName, _ := parsePVCURI(uri)
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
	}

	// Always add the 4Gi /tmp scratch space to support ReadOnlyRootFilesystem
	for _, volume := range podSpec.Volumes {
		if volume.Name == "tmp-scratch" {
			return
		}
	}
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "tmp-scratch",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: ptr.To(resource.MustParse("4Gi")),
			},
		},
	})
}

func (b *Builder) ensureModelVolumeMount(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	modelMountFound := false
	for i := range c.VolumeMounts {
		m := &c.VolumeMounts[i]
		if m.Name == api.ModelVolumeName {
			modelMountFound = true
			if !hasArg(c.Args, "--model") {
				m.MountPath = api.ModelMountPath
				m.ReadOnly = true
			}
			if strings.HasPrefix(llmSvc.Spec.Model.URI, "pvc://") {
				_, uriSubPath := parsePVCURI(llmSvc.Spec.Model.URI)
				if uriSubPath != "" {
					m.SubPath = uriSubPath
				}
			}
			break
		}
	}
	if !modelMountFound {
		mount := corev1.VolumeMount{
			Name:      api.ModelVolumeName,
			MountPath: api.ModelMountPath,
			ReadOnly:  true,
		}
		if strings.HasPrefix(llmSvc.Spec.Model.URI, "pvc://") {
			_, mount.SubPath = parsePVCURI(llmSvc.Spec.Model.URI)
		}
		c.VolumeMounts = append(c.VolumeMounts, mount)
	}

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
	if c.StartupProbe == nil {
		c.StartupProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(8000)},
			},
			// Large quantized and multimodal models may spend several minutes
			// loading weights and warming hardware-specific kernels. StartupProbe
			// gates liveness until that one-time initialization completes.
			InitialDelaySeconds: 30,
			PeriodSeconds:       15,
			FailureThreshold:    60,
		}
	}
	if c.ReadinessProbe == nil {
		c.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(8000)},
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
		"VLLM_TARGET_DEVICE":      "cpu",
		"USER":                    "nonroot",
		"LOGNAME":                 "nonroot",
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

func (b *Builder) buildAnnotations(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec, kvRole string) map[string]string {
	ann := make(map[string]string)
	if len(podSpec.Containers) > 0 {
		ann["serving.ckodex.com/runtime-image"] = podSpec.Containers[0].Image
	}
	if llmSvc.Spec.KVCache != nil && llmSvc.Spec.KVCache.Transfer != nil {
		ann["serving.ckodex.com/kv-connector"] = llmSvc.Spec.KVCache.Transfer.Connector
		if kvRole == "" {
			kvRole = llmSvc.Spec.KVCache.Transfer.Role
		}
		if kvRole == "" {
			kvRole = "kv_both"
		}
		ann["serving.ckodex.com/kv-role"] = kvRole
	}
	if llmSvc.Spec.Prefill != nil {
		ann["serving.ckodex.com/pd-disaggregation"] = "true"
	}
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

	// GGUF format: auto-route to quant-cpp engine (no need to set engine: quant-cpp explicitly).
	if llmSvc.Spec.Quantization != nil && llmSvc.Spec.Quantization.Method == "gguf" {
		c.Image = b.quantCppImage()
		if b.AirGappedMode && b.LocalRegistry != "" {
			c.Image = b.rewriteImage(c.Image)
		}
		b.ensureQuantCppArgs(llmSvc, c, hwType)
		return
	}

	engine := llmSvc.Spec.Engine
	if engine == "" {
		engine = "vllm"
	}

	switch engine {
	case "quant-cpp":
		c.Image = b.quantCppImage()
		if b.AirGappedMode && b.LocalRegistry != "" {
			c.Image = b.rewriteImage(c.Image)
		}
		b.ensureQuantCppArgs(llmSvc, c, hwType)
	default:
		// Default to vllm image if not already set by template
		if c.Image == "" {
			c.Image = b.RuntimeImage
			if c.Image == "" {
				c.Image = api.VLLMImage
			}
		}
		if b.AirGappedMode && b.LocalRegistry != "" {
			c.Image = b.rewriteImage(c.Image)
		}
		// The mounted model is mandatory even when a preset or user supplied other args.
		if !hasArg(c.Args, "--model") {
			c.Args = append([]string{"--model", api.ModelMountPath}, c.Args...)
		}
		if !hasArg(c.Args, "--host") {
			c.Args = append(c.Args, "--host", "0.0.0.0")
		}
		if !hasArg(c.Args, "--port") {
			c.Args = append(c.Args, "--port", "8000")
		}
		// Weight quantization is appended after existing args.
		if q := llmSvc.Spec.Quantization; q != nil {
			c.Args = append(c.Args, "--quantization", q.Method)
		}
	}
}

func (b *Builder) quantCppImage() string {
	if b.Defaults.QuantCppImage != "" {
		return b.Defaults.QuantCppImage
	}
	return api.QuantCppImage
}

func hasArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target || strings.HasPrefix(arg, target+"=") {
			return true
		}
	}
	return false
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
	return storage.ResolveAirGappedURI(uri, b.LocalRegistry)
}
