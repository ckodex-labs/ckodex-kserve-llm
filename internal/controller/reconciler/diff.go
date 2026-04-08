package reconciler

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

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
