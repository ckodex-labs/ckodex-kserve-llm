package deployment

import (
	"context"
	"fmt"
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

func (b *Builder) applyLocalModelCache(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) bool {
	active := b.getReadyLMC(ctx, llmSvc.Spec.Model.URI)
	if active == nil {
		return false
	}
	readyNodes := readyCacheNodes(active)
	if len(readyNodes) == 0 {
		return false
	}
	if podSpec.Affinity == nil {
		podSpec.Affinity = &corev1.Affinity{}
	}
	if podSpec.Affinity.NodeAffinity == nil {
		podSpec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = cacheNodeSelector(readyNodes)
	podSpec.Volumes = append(podSpec.Volumes, cacheHostVolume(active.Name))
	return true
}

func readyCacheNodes(cache *servingv1alpha2.LocalModelCache) []string {
	var nodes []string
	for _, status := range cache.Status.NodeStatuses {
		if status.Phase == "Ready" {
			nodes = append(nodes, status.NodeName)
		}
	}
	return nodes
}

func cacheNodeSelector(nodes []string) *corev1.NodeSelector {
	return &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: nodes}}}}}
}

func cacheHostVolume(name string) corev1.Volume {
	return corev1.Volume{Name: api.ModelVolumeName, VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: fmt.Sprintf("/tmp/ckodex/models/%s", name), Type: ptr.To(corev1.HostPathDirectoryOrCreate)}}}
}

func (b *Builder) getReadyLMC(ctx context.Context, modelURI string) *servingv1alpha2.LocalModelCache {
	var list servingv1alpha2.LocalModelCacheList
	if err := b.Client.List(ctx, &list); err != nil {
		return nil
	}
	for index := range list.Items {
		if list.Items[index].Spec.SourceModelURI == modelURI && localCacheReady(&list.Items[index]) {
			return &list.Items[index]
		}
	}
	return nil
}

func (b *Builder) transformModelURI(uri string, hwType HardwareType) string {
	if !strings.HasPrefix(uri, "oci://") || strings.Contains(uri, "@sha256:") {
		return uri
	}
	suffix := hardwareModelSuffix(hwType)
	if strings.Contains(uri, ":") {
		return uri + suffix
	}
	return uri + ":latest" + suffix
}

func hardwareModelSuffix(hwType HardwareType) string {
	switch hwType {
	case HardwareNVIDIA:
		return "-nvidia"
	case HardwareAppleSiliconMPS:
		return "-mps"
	case HardwareAMD:
		return "-rocm"
	default:
		return "-cpu"
	}
}

func (b *Builder) isLocalModelCacheReady(ctx context.Context, modelURI string) bool {
	return b.getReadyLMC(ctx, modelURI) != nil
}

func isHuggingFaceScheme(scheme string) bool { return scheme == "hf" || scheme == "hf-mirror" }

func parsePVCURI(uri string) (claim, subPath string) {
	parts := strings.SplitN(strings.TrimPrefix(uri, "pvc://"), "/", 2)
	claim = parts[0]
	if len(parts) == 2 {
		subPath = strings.Trim(parts[1], "/")
	}
	return claim, subPath
}

func (b *Builder) ensureModelVolume(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	if !hasVolume(podSpec.Volumes, api.ModelVolumeName) {
		podSpec.Volumes = append(podSpec.Volumes, modelVolume(llmSvc))
	}
	if !hasVolume(podSpec.Volumes, "tmp-scratch") {
		podSpec.Volumes = append(podSpec.Volumes, scratchVolume())
	}
}

func hasVolume(volumes []corev1.Volume, name string) bool {
	for _, volume := range volumes {
		if volume.Name == name {
			return true
		}
	}
	return false
}

func modelVolume(llmSvc *servingv1alpha2.LLMInferenceService) corev1.Volume {
	uri := llmSvc.Spec.Model.URI
	switch {
	case strings.HasPrefix(uri, "modelpack://"):
		return corev1.Volume{Name: api.ModelVolumeName, VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: "model.csi.modelpack.org", VolumeAttributes: map[string]string{"modelRef": strings.TrimPrefix(uri, "modelpack://")}}}}
	case strings.HasPrefix(uri, "hf-mount://"):
		return hfMountVolume(llmSvc)
	case strings.HasPrefix(uri, "pvc://"):
		claim, _ := parsePVCURI(uri)
		return pvcVolume(claim)
	default:
		return corev1.Volume{Name: api.ModelVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}
	}
}

func hfMountVolume(llmSvc *servingv1alpha2.LLMInferenceService) corev1.Volume {
	name := fmt.Sprintf("hf-model-%s-%s", llmSvc.Namespace, llmSvc.Name)
	if len(name) > 253 {
		name = name[:253]
	}
	return pvcVolume(name)
}

func pvcVolume(claim string) corev1.Volume {
	return corev1.Volume{Name: api.ModelVolumeName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim, ReadOnly: true}}}
}

func scratchVolume() corev1.Volume {
	return corev1.Volume{Name: "tmp-scratch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: ptr.To(resource.MustParse("4Gi"))}}}
}

func (b *Builder) ensureModelVolumeMount(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	c := &podSpec.Containers[0]
	mountModelVolume(c, llmSvc)
	if !hasMountPath(c.VolumeMounts, "/tmp") {
		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{Name: "tmp-scratch", MountPath: "/tmp"})
	}
}

func mountModelVolume(c *corev1.Container, llmSvc *servingv1alpha2.LLMInferenceService) {
	for index := range c.VolumeMounts {
		if c.VolumeMounts[index].Name == api.ModelVolumeName {
			configureModelMount(&c.VolumeMounts[index], c, llmSvc)
			return
		}
	}
	mount := corev1.VolumeMount{Name: api.ModelVolumeName, MountPath: api.ModelMountPath, ReadOnly: true}
	if strings.HasPrefix(llmSvc.Spec.Model.URI, "pvc://") {
		_, mount.SubPath = parsePVCURI(llmSvc.Spec.Model.URI)
	}
	c.VolumeMounts = append(c.VolumeMounts, mount)
}

func configureModelMount(mount *corev1.VolumeMount, c *corev1.Container, llmSvc *servingv1alpha2.LLMInferenceService) {
	if !hasArg(c.Args, "--model") {
		mount.MountPath = api.ModelMountPath
		mount.ReadOnly = true
	}
	if strings.HasPrefix(llmSvc.Spec.Model.URI, "pvc://") {
		if _, subPath := parsePVCURI(llmSvc.Spec.Model.URI); subPath != "" {
			mount.SubPath = subPath
		}
	}
}

func hasMountPath(mounts []corev1.VolumeMount, path string) bool {
	for _, mount := range mounts {
		if mount.MountPath == path {
			return true
		}
	}
	return false
}
