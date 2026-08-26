/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (r *LocalModelCacheReconciler) buildCachePVC(lmc *servingv1alpha2.LocalModelCache, pvcName, namespace, nodeName, modelHash string) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: cachePVCMetadata(lmc, pvcName, namespace, nodeName, modelHash), Spec: cachePVCSpec(lmc)}
	if lmc.Spec.StorageClassName != nil {
		pvc.Spec.StorageClassName = lmc.Spec.StorageClassName
	}
	return pvc
}

func cachePVCMetadata(lmc *servingv1alpha2.LocalModelCache, pvcName, namespace, nodeName, modelHash string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: pvcName, Namespace: namespace,
		Labels:      map[string]string{labelLocalCache: lmc.Name, labelNode: nodeName, labelModelHash: modelHash},
		Annotations: map[string]string{"serving.ckodex.com/model-uri": lmc.Spec.SourceModelURI},
	}
}

func cachePVCSpec(lmc *servingv1alpha2.LocalModelCache) corev1.PersistentVolumeClaimSpec {
	return corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: lmc.Spec.ModelSizeQuantity()}},
	}
}

func (r *LocalModelCacheReconciler) buildWarmupJob(lmc *servingv1alpha2.LocalModelCache, jobName, pvcName, namespace, nodeName string) *batchv1.Job {
	backoffLimit := int32(3)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace, Labels: warmupJobLabels(lmc, nodeName)},
		Spec:       batchv1.JobSpec{BackoffLimit: &backoffLimit, Template: corev1.PodTemplateSpec{Spec: r.warmupPodSpec(lmc, pvcName, nodeName)}},
	}
}

func warmupJobLabels(lmc *servingv1alpha2.LocalModelCache, nodeName string) map[string]string {
	return map[string]string{labelLocalCache: lmc.Name, labelNode: nodeName}
}

func (r *LocalModelCacheReconciler) warmupPodSpec(lmc *servingv1alpha2.LocalModelCache, pvcName, nodeName string) corev1.PodSpec {
	nonRoot := true
	nonRootUID := int64(65532)
	nonRootGID := int64(65532)
	podSpec := corev1.PodSpec{
		NodeSelector:                  map[string]string{"kubernetes.io/hostname": nodeName},
		RestartPolicy:                 corev1.RestartPolicyOnFailure,
		TerminationGracePeriodSeconds: ptr.To(r.cacheTerminationGracePeriod()),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &nonRoot, RunAsUser: &nonRootUID, RunAsGroup: &nonRootGID,
			FSGroup: &nonRootGID, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Containers: []corev1.Container{r.warmupContainer(lmc)},
		Volumes: []corev1.Volume{
			{Name: "cache", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName}}},
			{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
	}
	applyWarmupStorageConfig(lmc, &podSpec)
	return podSpec
}

func applyWarmupStorageConfig(lmc *servingv1alpha2.LocalModelCache, podSpec *corev1.PodSpec) {
	if lmc.Spec.Storage == nil {
		return
	}
	if lmc.Spec.Storage.SecretName != "" {
		podSpec.Containers[0].EnvFrom = append(podSpec.Containers[0].EnvFrom, corev1.EnvFromSource{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: lmc.Spec.Storage.SecretName}}})
	}
	if lmc.Spec.Storage.ServiceAccountName != "" {
		podSpec.ServiceAccountName = lmc.Spec.Storage.ServiceAccountName
	}
}

func (r *LocalModelCacheReconciler) warmupContainer(lmc *servingv1alpha2.LocalModelCache) corev1.Container {
	nonRoot := true
	nonRootUID := int64(65532)
	nonRootGID := int64(65532)
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true
	image := r.Defaults.CustomStorageInitializerImage
	if image == "" {
		image = CKodexStorageInitializerImage
	}
	return corev1.Container{
		Name: "warmup", Image: image, ImagePullPolicy: corev1.PullIfNotPresent,
		Args: []string{lmc.Spec.SourceModelURI, ModelMountPath},
		Env:  warmupEnvironment(lmc), VolumeMounts: []corev1.VolumeMount{
			{Name: "cache", MountPath: ModelMountPath, ReadOnly: false}, {Name: "tmp", MountPath: "/tmp", ReadOnly: false},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot: &nonRoot, RunAsUser: &nonRootUID, RunAsGroup: &nonRootGID,
			AllowPrivilegeEscalation: &allowPrivilegeEscalation, ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
			Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		}, Resources: r.warmupResources(),
	}
}

func warmupEnvironment(lmc *servingv1alpha2.LocalModelCache) []corev1.EnvVar {
	env := append([]corev1.EnvVar{}, lmc.Spec.Env...)
	return append(env,
		corev1.EnvVar{Name: "S3_ENDPOINT", Value: SeaweedFSFilerS3Endpoint},
		corev1.EnvVar{Name: "AWS_ENDPOINT_URL", Value: SeaweedFSFilerS3Endpoint},
		corev1.EnvVar{Name: "AWS_NO_SIGN_REQUEST", Value: "yes"},
		corev1.EnvVar{Name: "S3_USE_HTTPS", Value: "false"},
		corev1.EnvVar{Name: "S3_USE_PATH_STYLE", Value: "true"},
		corev1.EnvVar{Name: "TMPDIR", Value: "/tmp"},
	)
}

func (r *LocalModelCacheReconciler) warmupResources() corev1.ResourceRequirements {
	cpuRequest := r.Defaults.CacheCPURequest
	if cpuRequest == "" {
		cpuRequest = DefaultCacheCPURequest
	}
	memoryRequest := r.Defaults.CacheMemoryRequest
	if memoryRequest == "" {
		memoryRequest = DefaultCacheMemoryRequest
	}
	cpu := resource.MustParse(cpuRequest)
	memory := resource.MustParse(memoryRequest)
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
		Limits:   corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
	}
}

func (r *LocalModelCacheReconciler) cacheTerminationGracePeriod() int64 {
	if r.Defaults.ASRTerminationGracePeriodSeconds != 0 {
		return r.Defaults.ASRTerminationGracePeriodSeconds
	}
	return ASRTerminationGracePeriod
}
