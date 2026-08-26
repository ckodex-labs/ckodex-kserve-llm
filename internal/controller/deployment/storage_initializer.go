package deployment

import (
	"context"
	"path/filepath"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	corev1 "k8s.io/api/core/v1"
)

func (b *Builder) BuildStorageInitializer(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, hwType HardwareType, activeLMC *servingv1alpha2.LocalModelCache) *corev1.Container {
	uri := effectiveModelURI(llmSvc.Spec.Model)
	if storageInitializerSkipped(uri) || b.storageCacheReady(ctx, uri, activeLMC) {
		return nil
	}
	uri = b.prepareInitializerURI(uri, llmSvc, hwType)
	scheme := modelScheme(uri)
	container := b.newInitializerContainer(uri, scheme, hwType)
	b.configureInitializerEnvironment(container, llmSvc, scheme)
	if b.LocalCosignKeyPath != "" {
		b.configureCosignMount(container, llmSvc.Spec.Template.Spec)
	}
	b.applyRestrictedSecurityContext(container)
	return container
}

func effectiveModelURI(model servingv1alpha2.ModelSpec) string {
	if model.Revision == "" {
		return model.URI
	}
	return model.URI + "@" + model.Revision
}

func storageInitializerSkipped(uri string) bool {
	return uri == "" || strings.HasPrefix(uri, "modelpack://") || strings.HasPrefix(uri, "hf-mount://") || strings.HasPrefix(uri, "pvc://")
}

func (b *Builder) storageCacheReady(ctx context.Context, uri string, active *servingv1alpha2.LocalModelCache) bool {
	if active != nil {
		return localCacheReady(active)
	}
	return b.isLocalModelCacheReady(ctx, uri)
}

func localCacheReady(cache *servingv1alpha2.LocalModelCache) bool {
	for _, status := range cache.Status.NodeStatuses {
		if status.Phase == "Ready" {
			return true
		}
	}
	return false
}

func (b *Builder) prepareInitializerURI(uri string, llmSvc *servingv1alpha2.LLMInferenceService, hwType HardwareType) string {
	if b.EnableHardwareSelection && llmSvc.Spec.Model.HardwareAware {
		uri = b.transformModelURI(uri, hwType)
	}
	if b.AirGappedMode && b.LocalRegistry != "" {
		uri = b.storageResolveAirGap(uri)
	}
	return uri
}

func modelScheme(uri string) string {
	parts := strings.SplitN(uri, "://", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

func (b *Builder) newInitializerContainer(uri, scheme string, hwType HardwareType) *corev1.Container {
	image := api.HuggingFaceInitializerImage
	if b.HFInitializerImage != "" {
		image = b.HFInitializerImage
	}
	if !isHuggingFaceScheme(scheme) || hwType == HardwareAppleSilicon {
		image = b.Defaults.CustomStorageInitializerImage
		if image == "" {
			image = api.CKodexStorageInitializerImage
		}
	}
	if b.AirGappedMode && b.LocalRegistry != "" {
		image = b.rewriteImage(image)
	}
	return &corev1.Container{Name: "storage-initializer", Image: image, Args: []string{uri, api.ModelMountPath}, VolumeMounts: []corev1.VolumeMount{{Name: api.ModelVolumeName, MountPath: api.ModelMountPath}, {Name: "tmp-scratch", MountPath: "/tmp"}}}
}

func (b *Builder) configureInitializerEnvironment(container *corev1.Container, llmSvc *servingv1alpha2.LLMInferenceService, scheme string) {
	if isHuggingFaceScheme(scheme) {
		container.Env = append(container.Env, corev1.EnvVar{Name: "HOME", Value: "/tmp"}, corev1.EnvVar{Name: "HF_HOME", Value: "/tmp/huggingface"}, corev1.EnvVar{Name: "HF_HUB_DISABLE_UPDATE_CHECK", Value: "1"})
		if scheme == "hf-mirror" && b.HFMirrorURL != "" {
			container.Env = append(container.Env, corev1.EnvVar{Name: "HF_ENDPOINT", Value: b.HFMirrorURL})
		}
	}
	if llmSvc.Spec.Model.Storage == nil {
		return
	}
	storage := llmSvc.Spec.Model.Storage
	if storage.VaultRef != "" {
		container.Env = append(container.Env, corev1.EnvVar{Name: "VAULT_PATH", Value: storage.VaultRef})
	}
	if storage.VaultAddr != "" {
		container.Env = append(container.Env, corev1.EnvVar{Name: "VAULT_ADDR", Value: storage.VaultAddr})
	}
	if storage.SecretRef != nil {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: *storage.SecretRef}})
	}
	if storage.ExternalSecret != nil {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: llmSvc.Name + "-external-secret"}}})
	}
}

func (b *Builder) configureCosignMount(container *corev1.Container, podSpec corev1.PodSpec) {
	container.Env = append(container.Env, corev1.EnvVar{Name: "CKODEX_LOCAL_COSIGN_KEY_PATH", Value: b.LocalCosignKeyPath})
	b.copyMatchingVolumeMounts(container, &podSpec, b.LocalCosignKeyPath)
}

func (b *Builder) copyMatchingVolumeMounts(container *corev1.Container, podSpec *corev1.PodSpec, filePath string) {
	targetDir := filepath.Clean(filepath.Dir(filePath))
	for _, existing := range podSpec.Containers {
		for _, mount := range existing.VolumeMounts {
			if matchingMount(targetDir, filePath, mount.MountPath) && !hasVolumeMount(container.VolumeMounts, mount) {
				container.VolumeMounts = append(container.VolumeMounts, mount)
			}
		}
	}
}

func matchingMount(targetDir, filePath, mountPath string) bool {
	mountPath = filepath.Clean(mountPath)
	return mountPath == targetDir || mountPath == filepath.Clean(filePath) || strings.HasPrefix(targetDir, mountPath+string(filepath.Separator))
}

func hasVolumeMount(mounts []corev1.VolumeMount, candidate corev1.VolumeMount) bool {
	for _, mount := range mounts {
		if mount.Name == candidate.Name && mount.MountPath == candidate.MountPath && mount.SubPath == candidate.SubPath && mount.ReadOnly == candidate.ReadOnly {
			return true
		}
	}
	return false
}
