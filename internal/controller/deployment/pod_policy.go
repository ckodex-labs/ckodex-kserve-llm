package deployment

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

func (b *Builder) ensureResources(c *corev1.Container) {
	if c.Resources.Requests == nil {
		c.Resources.Requests = make(corev1.ResourceList)
	}
	if _, ok := c.Resources.Requests[corev1.ResourceCPU]; !ok {
		cpu := b.Defaults.VLLMCPURequest
		if cpu == "" {
			cpu = api.DefaultVLLMCPURequest
		}
		c.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if _, ok := c.Resources.Requests[corev1.ResourceMemory]; !ok {
		memory := b.Defaults.VLLMMemoryRequest
		if memory == "" {
			memory = api.DefaultVLLMMemoryRequest
		}
		c.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(memory)
	}
	if c.Resources.Limits == nil {
		c.Resources.Limits = make(corev1.ResourceList)
	}
	if _, ok := c.Resources.Limits[corev1.ResourceCPU]; !ok {
		c.Resources.Limits[corev1.ResourceCPU] = c.Resources.Requests[corev1.ResourceCPU]
	}
	if _, ok := c.Resources.Limits[corev1.ResourceMemory]; !ok {
		c.Resources.Limits[corev1.ResourceMemory] = c.Resources.Requests[corev1.ResourceMemory]
	}
}

func (b *Builder) injectPreStop(c *corev1.Container) {
	if c.Lifecycle == nil {
		c.Lifecycle = &corev1.Lifecycle{}
	}
	if c.Lifecycle.PreStop == nil {
		c.Lifecycle.PreStop = &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				// Keep cleanup inside this container. Killing arbitrary PIDs returned
				// by nvidia-smi could terminate another tenant's workload on a
				// time-sliced GPU; terminating PID 1 lets the driver reclaim this
				// container's CUDA context without crossing that boundary.
				Command: []string{"/bin/sh", "-c", "kill -TERM 1 2>/dev/null || true; sleep 10; kill -KILL 1 2>/dev/null || true; sleep 5"},
			},
		}
	}
}

func (b *Builder) ensureHealthProbes(podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	if c.StartupProbe == nil {
		c.StartupProbe = healthProbe(30, 15, 60, 0)
	}
	if c.ReadinessProbe == nil {
		c.ReadinessProbe = healthProbe(30, 10, 0, 3)
	}
	if c.LivenessProbe == nil {
		c.LivenessProbe = healthProbe(120, 15, 0, 0)
	}
}

func healthProbe(delay, period, failure, success int32) *corev1.Probe {
	probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(8000)}}, InitialDelaySeconds: delay, PeriodSeconds: period}
	if failure != 0 {
		probe.FailureThreshold = failure
	}
	if success != 0 {
		probe.SuccessThreshold = success
	}
	return probe
}

func (b *Builder) ensureSecurityContext(podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	for i := range podSpec.Containers {
		b.applyRestrictedSecurityContext(&podSpec.Containers[i])
	}
	for i := range podSpec.InitContainers {
		b.applyRestrictedSecurityContext(&podSpec.InitContainers[i])
	}
	if podSpec.SecurityContext == nil {
		podSpec.SecurityContext = &corev1.PodSecurityContext{FSGroup: ptr.To(int64(65532)), RunAsNonRoot: ptr.To(true), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}
	}
}

func (b *Builder) applyRestrictedSecurityContext(c *corev1.Container) {
	if c.SecurityContext == nil {
		c.SecurityContext = &corev1.SecurityContext{}
	}
	c.SecurityContext.RunAsUser = ptr.To(int64(65532))
	c.SecurityContext.RunAsGroup = ptr.To(int64(65532))
	c.SecurityContext.RunAsNonRoot = ptr.To(true)
	c.SecurityContext.ReadOnlyRootFilesystem = ptr.To(true)
	c.SecurityContext.AllowPrivilegeEscalation = ptr.To(false)
	c.SecurityContext.Capabilities = &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}
	c.SecurityContext.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
}

func (b *Builder) ensureTopologySpreadConstraints(podSpec *corev1.PodSpec, labels map[string]string) {
	if len(podSpec.TopologySpreadConstraints) > 0 {
		return
	}
	podSpec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{spreadConstraint("topology.kubernetes.io/zone", labels), spreadConstraint("kubernetes.io/hostname", labels)}
}

func spreadConstraint(key string, labels map[string]string) corev1.TopologySpreadConstraint {
	return corev1.TopologySpreadConstraint{MaxSkew: 1, TopologyKey: key, WhenUnsatisfiable: corev1.ScheduleAnyway, LabelSelector: &metav1.LabelSelector{MatchLabels: labels}}
}
