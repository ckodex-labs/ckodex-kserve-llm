package reconciler

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestSyncDeploymentConvergesManagedDeploymentFields(t *testing.T) {
	existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "model", Labels: map[string]string{"external": "keep"}, Annotations: map[string]string{"external": "keep"}}, Spec: appsv1.DeploymentSpec{
		Replicas: ptr.To(int32(1)), Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"old": "label"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "server", Image: "old"}}, InitContainers: []corev1.Container{{Name: "init", Image: "old"}}, Volumes: []corev1.Volume{{Name: "old"}}}},
	}}
	desired := existing.DeepCopy()
	desired.Labels = map[string]string{"app.kubernetes.io/name": "llm", "external": "new"}
	desired.Annotations = map[string]string{"ckodex.com/canary-weight": "50"}
	desired.Spec.Replicas = ptr.To(int32(3))
	desired.Spec.Template.Labels = map[string]string{"new": "label"}
	desired.Spec.Template.Spec.Containers[0].Image = "new"
	desired.Spec.Template.Spec.InitContainers[0].Image = "new"
	desired.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "new"}}
	desired.Spec.Template.Spec.Affinity = &corev1.Affinity{}

	if !SyncDeployment(context.Background(), existing, desired, 3, false) {
		t.Fatal("SyncDeployment did not report managed changes")
	}
	if existing.Spec.Replicas == nil || *existing.Spec.Replicas != 3 || existing.Labels["external"] != "keep" {
		t.Fatalf("unexpected convergence: %#v", existing)
	}
}

func TestSyncDeploymentLeavesReplicasToScaler(t *testing.T) {
	existing := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Replicas: ptr.To(int32(2)), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "server"}}}}}}
	desired := existing.DeepCopy()
	if SyncDeployment(context.Background(), existing, desired, 5, true) {
		t.Fatal("scaling-managed replicas should not cause a change")
	}
}
