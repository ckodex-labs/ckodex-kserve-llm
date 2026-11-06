package reconciler

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestContainerPullPolicyDefaults(t *testing.T) {
	const digest = "sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e"
	for _, tc := range []struct {
		name, image string
		want        corev1.PullPolicy
	}{
		{"untagged", "registry.example:5000/model", corev1.PullAlways},
		{"latest", "registry.example:5000/model:latest", corev1.PullAlways},
		{"versioned", "registry.example:5000/model:v1", corev1.PullIfNotPresent},
		{"digest only", "registry.example/model@" + digest, corev1.PullIfNotPresent},
		{"tag and digest", "registry.example/model:v1@" + digest, corev1.PullIfNotPresent},
		{"latest and digest", "registry.example/model:latest@" + digest, corev1.PullAlways},
		{"invalid", "Bad Image!", corev1.PullIfNotPresent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			omitted := corev1.Container{Name: "model", Image: tc.image}
			defaulted := omitted
			defaulted.ImagePullPolicy = tc.want
			if !ContainersEqual([]corev1.Container{defaulted}, []corev1.Container{omitted}) {
				t.Fatalf("omitted policy does not converge with Kubernetes default %q", tc.want)
			}
			if !ContainersEqual([]corev1.Container{omitted}, []corev1.Container{defaulted}) {
				t.Fatal("comparison must be symmetric")
			}
			explicit := omitted
			explicit.ImagePullPolicy = corev1.PullNever
			if ContainersEqual([]corev1.Container{explicit}, []corev1.Container{omitted}) {
				t.Fatal("explicit Never must not match the omitted policy")
			}
		})
	}
}

func TestSyncDeploymentUpdatesPullPolicy(t *testing.T) {
	for _, initContainer := range []bool{false, true} {
		t.Run(map[bool]string{false: "container", true: "init container"}[initContainer], func(t *testing.T) {
			container := corev1.Container{Name: "model", Image: "example.com/model:v1", ImagePullPolicy: corev1.PullNever}
			existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "model"}, Spec: appsv1.DeploymentSpec{Replicas: ptr.To[int32](1)}}
			if initContainer {
				existing.Spec.Template.Spec.InitContainers = []corev1.Container{container}
			} else {
				existing.Spec.Template.Spec.Containers = []corev1.Container{container}
			}
			desired := existing.DeepCopy()
			if initContainer {
				desired.Spec.Template.Spec.InitContainers[0].ImagePullPolicy = corev1.PullIfNotPresent
			} else {
				desired.Spec.Template.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent
			}
			if !SyncDeployment(context.Background(), existing, desired, 1, false) {
				t.Fatal("pull policy change was ignored")
			}
			if SyncDeployment(context.Background(), existing, desired, 1, false) {
				t.Fatal("updated policy did not converge")
			}
		})
	}
}
