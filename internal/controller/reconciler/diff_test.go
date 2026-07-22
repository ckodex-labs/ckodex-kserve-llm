package reconciler

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func vol(name, claim string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim},
		},
	}
}

func TestVolumesEqual_OrderInsensitive(t *testing.T) {
	// Regression: the builder and the Vector sidecar injector emit the same
	// volumes in different orders. That must NOT read as a change (it caused an
	// infinite Deployment update loop).
	a := []corev1.Volume{vol("cache-store", "c"), vol("model-store", "m"), vol("vector-data", "v")}
	b := []corev1.Volume{vol("vector-data", "v"), vol("cache-store", "c"), vol("model-store", "m")}
	if !VolumesEqual(a, b) {
		t.Fatal("VolumesEqual=false for the same volume set reordered, want true")
	}
}

func TestVolumesEqual_DetectsContentChange(t *testing.T) {
	a := []corev1.Volume{vol("model-store", "old-claim")}
	b := []corev1.Volume{vol("model-store", "new-claim")}
	if VolumesEqual(a, b) {
		t.Fatal("VolumesEqual=true when a volume's ClaimName changed, want false")
	}
}

func TestVolumesEqual_DetectsAddRemove(t *testing.T) {
	a := []corev1.Volume{vol("model-store", "m")}
	b := []corev1.Volume{vol("model-store", "m"), vol("cache-store", "c")}
	if VolumesEqual(a, b) {
		t.Fatal("VolumesEqual=true when a volume was added, want false")
	}
}

// TestSyncDeployment_ReorderedVolumesConverge is the integration-level regression:
// two Deployments identical except for volume order must produce no change, so the
// reconciler converges instead of updating forever.
func TestSyncDeployment_ReorderedVolumesConverge(t *testing.T) {
	mk := func(vols []corev1.Volume) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "gemma4", Labels: map[string]string{"a": "b"}},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "vllm", Image: "vllm:1"}},
						Volumes:    vols,
					},
				},
			},
		}
	}
	existing := mk([]corev1.Volume{vol("cache-store", "c"), vol("model-store", "m"), vol("vector-data", "v")})
	desired := mk([]corev1.Volume{vol("vector-data", "v"), vol("model-store", "m"), vol("cache-store", "c")})

	if SyncDeployment(context.Background(), existing, desired, 1, false) {
		t.Fatal("SyncDeployment reported a change for identical volumes in different order (reconcile loop)")
	}
}

// TestSyncDeployment_PreservesUnmanagedAnnotations is the root-cause regression:
// the Deployment controller adds deployment.kubernetes.io/revision, which the
// operator does not manage. A wholesale replace strips it, the controller re-adds
// it, and the reconcile loops forever. SyncDeployment must merge, not replace.
func TestSyncDeployment_PreservesUnmanagedAnnotations(t *testing.T) {
	base := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "gemma4"},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm", Image: "vllm:1"}}},
				},
			},
		}
	}
	existing := base()
	existing.Annotations = map[string]string{"deployment.kubernetes.io/revision": "16"}
	desired := base() // operator's desired has no annotations (buildAnnotations == {})

	if SyncDeployment(context.Background(), existing, desired, 1, false) {
		t.Fatal("SyncDeployment reported a change; wholesale-replacing annotations strips the revision -> loop")
	}
	if existing.Annotations["deployment.kubernetes.io/revision"] != "16" {
		t.Fatalf("unmanaged annotation stripped: %v", existing.Annotations)
	}
}
