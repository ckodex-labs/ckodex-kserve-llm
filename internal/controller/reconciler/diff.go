package reconciler

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SyncDeployment reconciles managed fields from desired into existing, returning true
// if any field changed. scalingManaged should be true when an HPA or KEDA resource
// owns replica count (in which case the controller skips the replica comparison).
func SyncDeployment(ctx context.Context, existing, desired *appsv1.Deployment, replicas int32, scalingManaged bool) bool {
	logger := log.FromContext(ctx)
	changed := false

	if !scalingManaged {
		if existing.Spec.Replicas == nil || *existing.Spec.Replicas != replicas {
			existing.Spec.Replicas = &replicas
			changed = true
		}
	}

	if !equality.Semantic.DeepEqual(existing.Labels, desired.Labels) {
		existing.Labels = desired.Labels
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Annotations, desired.Annotations) {
		existing.Annotations = desired.Annotations
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Labels, desired.Spec.Template.Labels) {
		existing.Spec.Template.Labels = desired.Spec.Template.Labels
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Annotations, desired.Spec.Template.Annotations) {
		existing.Spec.Template.Annotations = desired.Spec.Template.Annotations
		changed = true
	}
	if !ContainersEqual(existing.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers) {
		logger.Info("Deployment containers changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
		changed = true
	}
	if !ContainersEqual(existing.Spec.Template.Spec.InitContainers, desired.Spec.Template.Spec.InitContainers) {
		logger.Info("Deployment init containers changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.InitContainers = desired.Spec.Template.Spec.InitContainers
		changed = true
	}
	if !VolumesEqual(existing.Spec.Template.Spec.Volumes, desired.Spec.Template.Spec.Volumes) {
		logger.Info("Deployment volumes changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.Volumes = desired.Spec.Template.Spec.Volumes
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Spec.Affinity, desired.Spec.Template.Spec.Affinity) {
		logger.Info("Deployment affinity changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.Affinity = desired.Spec.Template.Spec.Affinity
		changed = true
	}

	return changed
}

// ContainersEqual compares two slices of containers looking only at managed fields.
// It performs order-insensitive environment variable comparison.
func ContainersEqual(a, b []corev1.Container) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
		if a[i].Image != b[i].Image {
			return false
		}
		if !equality.Semantic.DeepEqual(a[i].Args, b[i].Args) {
			return false
		}
		if !EnvsEqual(a[i].Env, b[i].Env) {
			return false
		}
		if !equality.Semantic.DeepEqual(a[i].EnvFrom, b[i].EnvFrom) {
			return false
		}
		if !equality.Semantic.DeepEqual(a[i].Resources, b[i].Resources) {
			return false
		}
		if !equality.Semantic.DeepEqual(a[i].VolumeMounts, b[i].VolumeMounts) {
			return false
		}
	}
	return true
}

// EnvsEqual compares two slices of environment variables as sets (order-insensitive).
func EnvsEqual(a, b []corev1.EnvVar) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]corev1.EnvVar)
	for _, e := range a {
		am[e.Name] = e
	}
	for _, e := range b {
		ev, ok := am[e.Name]
		if !ok || !equality.Semantic.DeepEqual(ev, e) {
			return false
		}
	}
	return true
}

// VolumeMountsEqual compares two slices of volume mounts.
func VolumeMountsEqual(a, b []corev1.VolumeMount) bool {
	return equality.Semantic.DeepEqual(a, b)
}

// VolumesEqual compares two slices of volumes.
func VolumesEqual(a, b []corev1.Volume) bool {
	return equality.Semantic.DeepEqual(a, b)
}
