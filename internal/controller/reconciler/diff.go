package reconciler

import (
	"context"
	"strings"

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

	// Merge (not replace) top-level labels/annotations: apply the operator's
	// managed keys but preserve keys added by other controllers — notably
	// deployment.kubernetes.io/revision, which the Deployment controller re-adds
	// after every wholesale replace, producing an infinite reconcile loop.
	if syncManagedMap(&existing.Labels, desired.Labels, isManagedDeploymentLabel) {
		changed = true
	}
	if syncManagedMap(&existing.Annotations, desired.Annotations, isManagedDeploymentAnnotation) {
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
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Spec.Tolerations, desired.Spec.Template.Spec.Tolerations) {
		logger.Info("Deployment tolerations changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.Tolerations = desired.Spec.Template.Spec.Tolerations
		changed = true
	}

	return changed
}

// syncManagedMap converges operator-managed keys while preserving metadata owned
// by Kubernetes and other controllers.
func syncManagedMap(target *map[string]string, desired map[string]string, managed func(string) bool) bool {
	changed := false
	for k := range *target {
		if managed(k) {
			if _, ok := desired[k]; !ok {
				delete(*target, k)
				changed = true
			}
		}
	}
	for k, v := range desired {
		if !managed(k) {
			continue
		}
		if *target == nil {
			*target = make(map[string]string, len(desired))
		}
		if cur, ok := (*target)[k]; !ok || cur != v {
			(*target)[k] = v
			changed = true
		}
	}
	return changed
}

func isManagedDeploymentLabel(key string) bool {
	switch key {
	case "app.kubernetes.io/name",
		"app.kubernetes.io/instance",
		"app.kubernetes.io/managed-by",
		"serving.ckodex.com/model":
		return true
	default:
		return strings.HasPrefix(key, "ckodex.cost/")
	}
}

func isManagedDeploymentAnnotation(key string) bool {
	switch key {
	case "ckodex.com/canary-weight",
		"sidecar.istio.io/inject",
		"sidecar.istio.io/rewriteAppHTTPProbers",
		"sidecar.istio.io/discoveryNamespaces":
		return true
	default:
		return false
	}
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

// VolumesEqual compares two slices of volumes as a set keyed by name
// (order-insensitive), mirroring EnvsEqual. Volume ordering is not semantically
// meaningful; treating it as significant caused an infinite Deployment update
// loop when the builder and the Vector sidecar injector emitted the same volumes
// in different orders (existing != desired every reconcile).
func VolumesEqual(a, b []corev1.Volume) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]corev1.Volume, len(a))
	for i := range a {
		am[a[i].Name] = a[i]
	}
	for _, v := range b {
		av, ok := am[v.Name]
		if !ok || !equality.Semantic.DeepEqual(av, v) {
			return false
		}
	}
	return true
}
