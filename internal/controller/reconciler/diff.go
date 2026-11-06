package reconciler

import (
	"context"
	_ "crypto/sha256" // Register digest algorithms supported by Kubernetes image parsing.
	_ "crypto/sha512"
	"strings"

	"github.com/distribution/reference"
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
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Spec.NodeSelector, desired.Spec.Template.Spec.NodeSelector) {
		logger.Info("Deployment nodeSelector changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.NodeSelector = desired.Spec.Template.Spec.NodeSelector
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Spec.TopologySpreadConstraints, desired.Spec.Template.Spec.TopologySpreadConstraints) {
		logger.Info("Deployment topologySpreadConstraints changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.TopologySpreadConstraints = desired.Spec.Template.Spec.TopologySpreadConstraints
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Spec.SecurityContext, desired.Spec.Template.Spec.SecurityContext) {
		logger.Info("Deployment pod securityContext changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.SecurityContext = desired.Spec.Template.Spec.SecurityContext
		changed = true
	}
	if !equality.Semantic.DeepEqual(existing.Spec.Template.Spec.TerminationGracePeriodSeconds, desired.Spec.Template.Spec.TerminationGracePeriodSeconds) {
		logger.Info("Deployment terminationGracePeriodSeconds changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.TerminationGracePeriodSeconds = desired.Spec.Template.Spec.TerminationGracePeriodSeconds
		changed = true
	}
	if existing.Spec.Template.Spec.ServiceAccountName != desired.Spec.Template.Spec.ServiceAccountName {
		logger.Info("Deployment serviceAccountName changed, updating", "name", existing.Name)
		existing.Spec.Template.Spec.ServiceAccountName = desired.Spec.Template.Spec.ServiceAccountName
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
		if effectivePullPolicy(a[i]) != effectivePullPolicy(b[i]) {
			return false
		}
		if !CommandEqual(a[i].Command, b[i].Command) {
			return false
		}
		if !equality.Semantic.DeepEqual(a[i].Args, b[i].Args) {
			return false
		}
		if a[i].WorkingDir != b[i].WorkingDir {
			return false
		}
		if !PortsEqual(a[i].Ports, b[i].Ports) {
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
		if !SecurityContextEqual(a[i].SecurityContext, b[i].SecurityContext) {
			return false
		}
		if !ProbesEqual(a[i].StartupProbe, b[i].StartupProbe) {
			return false
		}
		if !ProbesEqual(a[i].ReadinessProbe, b[i].ReadinessProbe) {
			return false
		}
		if !ProbesEqual(a[i].LivenessProbe, b[i].LivenessProbe) {
			return false
		}
		if !LifecycleEqual(a[i].Lifecycle, b[i].Lifecycle) {
			return false
		}
	}
	return true
}

// effectivePullPolicy mirrors Kubernetes container defaulting when the policy
// is omitted. An invalid image defaults to IfNotPresent, as in the API server.
func effectivePullPolicy(container corev1.Container) corev1.PullPolicy {
	if container.ImagePullPolicy != "" {
		return container.ImagePullPolicy
	}
	name, err := reference.ParseNormalizedNamed(container.Image)
	if err != nil {
		return corev1.PullIfNotPresent
	}
	tagged, hasTag := name.(reference.Tagged)
	_, hasDigest := name.(reference.Digested)
	if (hasTag && tagged.Tag() == "latest") || (!hasTag && !hasDigest) {
		return corev1.PullAlways
	}
	return corev1.PullIfNotPresent
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

// CommandEqual compares container entrypoint commands treating nil and empty as equal.
func CommandEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return equality.Semantic.DeepEqual(a, b)
}

type portKey struct {
	port     int32
	protocol corev1.Protocol
}

func effectiveProtocol(p corev1.Protocol) corev1.Protocol {
	if p == "" {
		return corev1.ProtocolTCP
	}
	return p
}

// PortsEqual compares container ports as an order-insensitive set with protocol defaulting.
func PortsEqual(a, b []corev1.ContainerPort) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[portKey]corev1.ContainerPort, len(a))
	for _, cp := range a {
		normalized := cp
		normalized.Protocol = effectiveProtocol(cp.Protocol)
		am[portKey{port: cp.ContainerPort, protocol: normalized.Protocol}] = normalized
	}
	for _, cp := range b {
		normalized := cp
		normalized.Protocol = effectiveProtocol(cp.Protocol)
		existing, ok := am[portKey{port: cp.ContainerPort, protocol: normalized.Protocol}]
		if !ok || !equality.Semantic.DeepEqual(existing, normalized) {
			return false
		}
	}
	return true
}

// ProbesEqual compares probes with Kubernetes default normalization to avoid false updates.
func ProbesEqual(a, b *corev1.Probe) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return equality.Semantic.DeepEqual(normalizeProbe(a), normalizeProbe(b))
}

func normalizeProbe(p *corev1.Probe) *corev1.Probe {
	if p == nil {
		return nil
	}
	cp := p.DeepCopy()
	if cp.TimeoutSeconds == 0 {
		cp.TimeoutSeconds = 1
	}
	if cp.PeriodSeconds == 0 {
		cp.PeriodSeconds = 10
	}
	if cp.SuccessThreshold == 0 {
		cp.SuccessThreshold = 1
	}
	if cp.FailureThreshold == 0 {
		cp.FailureThreshold = 3
	}
	if cp.HTTPGet != nil && cp.HTTPGet.Scheme == "" {
		cp.HTTPGet.Scheme = corev1.URISchemeHTTP
	}
	return cp
}

// SecurityContextEqual compares security contexts, treating nil and empty structs as equal.
func SecurityContextEqual(a, b *corev1.SecurityContext) bool {
	if isSecurityContextEmpty(a) && isSecurityContextEmpty(b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return equality.Semantic.DeepEqual(a, b)
}

func isSecurityContextEmpty(sc *corev1.SecurityContext) bool {
	if sc == nil {
		return true
	}
	return equality.Semantic.DeepEqual(sc, &corev1.SecurityContext{})
}

// LifecycleEqual compares container lifecycle handlers, treating nil and empty as equal.
func LifecycleEqual(a, b *corev1.Lifecycle) bool {
	if isLifecycleEmpty(a) && isLifecycleEmpty(b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return equality.Semantic.DeepEqual(a, b)
}

func isLifecycleEmpty(l *corev1.Lifecycle) bool {
	if l == nil {
		return true
	}
	return equality.Semantic.DeepEqual(l, &corev1.Lifecycle{})
}
