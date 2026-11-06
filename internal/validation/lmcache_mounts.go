package validation

import (
	"path"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// validateMultiprocessLMCacheMounts protects the shared IPC namespace from
// workload mounts that can hide the host /dev/shm used for CUDA IPC.
func validateMultiprocessLMCacheMounts(service *servingv1alpha2.LLMInferenceService) field.ErrorList {
	cache := service.Spec.KVCache
	if cache == nil || cache.Transfer == nil || cache.Transfer.LMCache == nil || cache.Transfer.LMCache.Mode != servingv1alpha2.LMCacheModeMultiprocess {
		return nil
	}
	errs := validateSharedMemoryMounts(service.Spec.Template.Spec, field.NewPath("spec", "template", "spec"))
	if service.Spec.Prefill != nil {
		errs = append(errs, validateSharedMemoryMounts(service.Spec.Prefill.Template.Spec, field.NewPath("spec", "prefill", "template", "spec"))...)
	}
	if service.Spec.Worker != nil {
		errs = append(errs, validateSharedMemoryMounts(service.Spec.Worker.Template.Spec, field.NewPath("spec", "worker", "template", "spec"))...)
	}
	return errs
}

func validateSharedMemoryMounts(spec corev1.PodSpec, root *field.Path) field.ErrorList {
	var errs field.ErrorList
	for i := range spec.Containers {
		errs = append(errs, sharedMemoryMountErrors(spec.Containers[i].VolumeMounts, root.Child("containers").Index(i))...)
	}
	for i := range spec.InitContainers {
		errs = append(errs, sharedMemoryMountErrors(spec.InitContainers[i].VolumeMounts, root.Child("initContainers").Index(i))...)
	}
	for i := range spec.EphemeralContainers {
		errs = append(errs, sharedMemoryMountErrors(spec.EphemeralContainers[i].VolumeMounts, root.Child("ephemeralContainers").Index(i))...)
	}
	return errs
}

func sharedMemoryMountErrors(mounts []corev1.VolumeMount, container *field.Path) field.ErrorList {
	var errs field.ErrorList
	for i, mount := range mounts {
		cleaned := path.Clean(mount.MountPath)
		if cleaned == "/" || cleaned == "/dev" || cleaned == "/dev/shm" || strings.HasPrefix(cleaned, "/dev/shm/") {
			errs = append(errs, field.Forbidden(container.Child("volumeMounts").Index(i).Child("mountPath"), "multiprocess LMCache requires shared host IPC; volume mounts must not shadow /dev/shm or its contents"))
		}
	}
	return errs
}
