package reconciler

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func baseDeployment(mutate func(*corev1.Container)) *appsv1.Deployment {
	c := corev1.Container{
		Name:            "vllm",
		Image:           "vllm/vllm-openai:v0.33.0",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"python", "-m", "vllm.entrypoints.openai.api_server"},
		Args:            []string{"--model", "kimi-k3"},
		WorkingDir:      "/workspace",
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8000, Protocol: corev1.ProtocolTCP},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:                ptr.To(int64(65532)),
			ReadOnlyRootFilesystem:   ptr.To(true),
			AllowPrivilegeEscalation: ptr.To(false),
		},
		StartupProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(8000), Scheme: corev1.URISchemeHTTP},
			},
			PeriodSeconds: 15, TimeoutSeconds: 1, FailureThreshold: 60, SuccessThreshold: 1,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(8000), Scheme: corev1.URISchemeHTTP},
			},
			PeriodSeconds: 10, TimeoutSeconds: 1, FailureThreshold: 3, SuccessThreshold: 1,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(8000), Scheme: corev1.URISchemeHTTP},
			},
			PeriodSeconds: 15, TimeoutSeconds: 1, FailureThreshold: 3, SuccessThreshold: 1,
		},
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{Command: []string{"/bin/sh", "-c", "kill -TERM 1"}},
			},
		},
	}
	if mutate != nil {
		mutate(&c)
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{c},
				},
			},
		},
	}
}

func TestSyncDeployment_ContainerCommandReconciliation(t *testing.T) {
	existing := baseDeployment(nil)
	desired := baseDeployment(func(c *corev1.Container) {
		c.Command = []string{"vllm", "serve"}
	})

	if !SyncDeployment(context.Background(), existing, desired, 1, false) {
		t.Fatal("expected SyncDeployment to detect container command change")
	}
	if existing.Spec.Template.Spec.Containers[0].Command[0] != "vllm" {
		t.Fatalf("command not updated, got: %v", existing.Spec.Template.Spec.Containers[0].Command)
	}

	// Nil vs empty command must converge without reporting a change.
	c1 := []corev1.Container{{Name: "a", Image: "img", Command: nil}}
	c2 := []corev1.Container{{Name: "a", Image: "img", Command: []string{}}}
	if !ContainersEqual(c1, c2) {
		t.Fatal("ContainersEqual=false for nil vs empty command, want true")
	}
}

func TestSyncDeployment_ContainerWorkingDirReconciliation(t *testing.T) {
	existing := baseDeployment(nil)
	desired := baseDeployment(func(c *corev1.Container) {
		c.WorkingDir = "/opt/models"
	})

	if !SyncDeployment(context.Background(), existing, desired, 1, false) {
		t.Fatal("expected SyncDeployment to detect container workingDir change")
	}
	if existing.Spec.Template.Spec.Containers[0].WorkingDir != "/opt/models" {
		t.Fatalf("workingDir not updated, got: %s", existing.Spec.Template.Spec.Containers[0].WorkingDir)
	}
}

func TestSyncDeployment_ContainerPortsReconciliation(t *testing.T) {
	existing := baseDeployment(nil)
	// Same ports reordered with default empty protocol vs explicit TCP must converge without change.
	desired := baseDeployment(func(c *corev1.Container) {
		c.Ports = []corev1.ContainerPort{
			{Name: "metrics", ContainerPort: 9090}, // empty Protocol defaults to TCP
			{Name: "http", ContainerPort: 8000, Protocol: corev1.ProtocolTCP},
		}
	})
	existing.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
		{Name: "http", ContainerPort: 8000, Protocol: corev1.ProtocolTCP},
		{Name: "metrics", ContainerPort: 9090, Protocol: corev1.ProtocolTCP},
	}

	if SyncDeployment(context.Background(), existing, desired, 1, false) {
		t.Fatal("SyncDeployment reported change for reordered ports with default protocol")
	}

	// Change port number -> must detect change.
	portChange := baseDeployment(func(c *corev1.Container) {
		c.Ports = []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
		}
	})
	if !SyncDeployment(context.Background(), existing, portChange, 1, false) {
		t.Fatal("expected SyncDeployment to detect port change")
	}
}

func TestSyncDeployment_ContainerSecurityContextReconciliation(t *testing.T) {
	existing := baseDeployment(nil)
	desired := baseDeployment(func(c *corev1.Container) {
		c.SecurityContext.RunAsUser = ptr.To(int64(10001))
	})

	if !SyncDeployment(context.Background(), existing, desired, 1, false) {
		t.Fatal("expected SyncDeployment to detect container securityContext change")
	}
	if *existing.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser != 10001 {
		t.Fatalf("RunAsUser not updated, got: %v", *existing.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser)
	}

	// Nil vs empty struct must be equal.
	sc1 := []corev1.Container{{Name: "a", Image: "img", SecurityContext: nil}}
	sc2 := []corev1.Container{{Name: "a", Image: "img", SecurityContext: &corev1.SecurityContext{}}}
	if !ContainersEqual(sc1, sc2) {
		t.Fatal("ContainersEqual=false for nil vs empty SecurityContext, want true")
	}
}

func TestSyncDeployment_ContainerProbesReconciliation(t *testing.T) {
	existing := baseDeployment(nil)
	// API server defaults TimeoutSeconds: 1, Scheme: HTTP if omitted.
	desired := baseDeployment(func(c *corev1.Container) {
		c.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(8000)}, // Scheme empty
			},
			PeriodSeconds: 10, TimeoutSeconds: 0, FailureThreshold: 3, SuccessThreshold: 0,
		}
	})

	if SyncDeployment(context.Background(), existing, desired, 1, false) {
		t.Fatal("SyncDeployment reported change for defaulted probe values (infinite loop)")
	}

	// Real change in path -> must detect.
	pathChange := baseDeployment(func(c *corev1.Container) {
		c.ReadinessProbe.HTTPGet.Path = "/v1/health"
	})
	if !SyncDeployment(context.Background(), existing, pathChange, 1, false) {
		t.Fatal("expected SyncDeployment to detect probe path change")
	}
}

func TestSyncDeployment_ContainerLifecycleReconciliation(t *testing.T) {
	existing := baseDeployment(nil)
	desired := baseDeployment(func(c *corev1.Container) {
		c.Lifecycle = &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{Command: []string{"/bin/sh", "-c", "sync; sleep 2"}},
			},
		}
	})

	if !SyncDeployment(context.Background(), existing, desired, 1, false) {
		t.Fatal("expected SyncDeployment to detect container lifecycle change")
	}
	if existing.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command[2] != "sync; sleep 2" {
		t.Fatalf("lifecycle command not updated")
	}

	// Nil vs empty struct must be equal.
	l1 := []corev1.Container{{Name: "a", Image: "img", Lifecycle: nil}}
	l2 := []corev1.Container{{Name: "a", Image: "img", Lifecycle: &corev1.Lifecycle{}}}
	if !ContainersEqual(l1, l2) {
		t.Fatal("ContainersEqual=false for nil vs empty Lifecycle, want true")
	}
}

func TestSyncDeployment_PodLevelFieldsReconciliation(t *testing.T) {
	existing := baseDeployment(nil)
	desired := baseDeployment(nil)
	desired.Spec.Template.Spec.NodeSelector = map[string]string{"gpu.nvidia.com/model": "rtx-3500"}
	desired.Spec.Template.Spec.TerminationGracePeriodSeconds = ptr.To(int64(90))
	desired.Spec.Template.Spec.ServiceAccountName = "custom-sa"
	desired.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		FSGroup:      ptr.To(int64(65532)),
	}

	if !SyncDeployment(context.Background(), existing, desired, 1, false) {
		t.Fatal("expected SyncDeployment to detect pod-level spec changes")
	}
	if existing.Spec.Template.Spec.NodeSelector["gpu.nvidia.com/model"] != "rtx-3500" {
		t.Fatalf("nodeSelector not updated")
	}
	if *existing.Spec.Template.Spec.TerminationGracePeriodSeconds != 90 {
		t.Fatalf("terminationGracePeriodSeconds not updated")
	}
	if existing.Spec.Template.Spec.ServiceAccountName != "custom-sa" {
		t.Fatalf("serviceAccountName not updated")
	}
	if *existing.Spec.Template.Spec.SecurityContext.FSGroup != 65532 {
		t.Fatalf("pod securityContext FSGroup not updated")
	}
}
