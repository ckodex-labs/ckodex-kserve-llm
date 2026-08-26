package deployment

import (
	"context"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestStorageVolumeURITransformsAndModelVolumeKinds(t *testing.T) {
	builder := &Builder{}
	assert.Equal(t, "oci://repo/model-nvidia", builder.transformModelURI("oci://repo/model", HardwareNVIDIA))
	assert.Equal(t, "oci://repo/model:tag-mps", builder.transformModelURI("oci://repo/model:tag", HardwareAppleSiliconMPS))
	assert.Equal(t, "oci://repo/model:tag@sha256:abc", builder.transformModelURI("oci://repo/model:tag@sha256:abc", HardwareAMD))
	assert.Equal(t, "hf://repo/model", builder.transformModelURI("hf://repo/model", HardwareNVIDIA))
	assert.Equal(t, "-nvidia", hardwareModelSuffix(HardwareNVIDIA))
	assert.Equal(t, "-cpu", hardwareModelSuffix(HardwareUnknown))

	for _, tc := range []struct {
		uri   string
		check func(t *testing.T, volume corev1.Volume)
	}{
		{"modelpack://org/model", func(t *testing.T, v corev1.Volume) { assert.Equal(t, "org/model", v.CSI.VolumeAttributes["modelRef"]) }},
		{"pvc://claim/sub/path", func(t *testing.T, v corev1.Volume) { assert.Equal(t, "claim", v.PersistentVolumeClaim.ClaimName) }},
		{"s3://bucket/model", func(t *testing.T, v corev1.Volume) { assert.NotNil(t, v.EmptyDir) }},
	} {
		service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{Model: servingv1alpha2.ModelSpec{URI: tc.uri}}}
		tc.check(t, modelVolume(service))
	}
}

func TestStorageVolumeHFMountTruncatesAndMountsPVCSubPath(t *testing.T) {
	longName := "namespace-" + string(make([]byte, 260))
	service := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: longName, Namespace: "ns"}, Spec: servingv1alpha2.LLMInferenceServiceSpec{Model: servingv1alpha2.ModelSpec{URI: "hf-mount://org/model"}}}
	volume := modelVolume(service)
	assert.Len(t, volume.PersistentVolumeClaim.ClaimName, 253)

	pod := &corev1.PodSpec{Containers: []corev1.Container{{}}}
	service.Spec.Model.URI = "pvc://claim/sub/path"
	(&Builder{}).ensureModelVolumeMount(service, pod)
	require.Len(t, pod.Containers[0].VolumeMounts, 2)
	assert.Equal(t, "sub/path", pod.Containers[0].VolumeMounts[0].SubPath)
	assert.True(t, hasMountPath(pod.Containers[0].VolumeMounts, "/tmp"))
	assert.Equal(t, "claim", func() string { claim, _ := parsePVCURI("pvc://claim/sub/path/"); return claim }())
}

func TestApplyLocalModelCacheUsesReadyNodesAndRejectsMissingCache(t *testing.T) {
	cache := &servingv1alpha2.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache"}, Spec: servingv1alpha2.LocalModelCacheSpec{SourceModelURI: "hf://org/model"}, Status: servingv1alpha2.LocalModelCacheStatus{NodeStatuses: []servingv1alpha2.NodeCacheStatus{{NodeName: "node-a", Phase: "Ready"}, {NodeName: "node-b", Phase: "Downloading"}}}}
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, servingv1alpha2.AddToScheme(scheme))
	builder := &Builder{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(cache).Build()}
	pod := &corev1.PodSpec{}
	assert.True(t, builder.applyLocalModelCache(context.Background(), &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{Model: servingv1alpha2.ModelSpec{URI: "hf://org/model"}}}, pod))
	assert.Equal(t, []string{"node-a"}, pod.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)
	assert.Equal(t, "/tmp/ckodex/models/cache", pod.Volumes[0].HostPath.Path)
	assert.False(t, (&Builder{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}).applyLocalModelCache(context.Background(), &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{Model: servingv1alpha2.ModelSpec{URI: "hf://missing"}}}, &corev1.PodSpec{}))
	assert.True(t, isHuggingFaceScheme("hf"))
	assert.True(t, isHuggingFaceScheme("hf-mirror"))
	assert.False(t, isHuggingFaceScheme("s3"))
	assert.Equal(t, api.ModelVolumeName, cacheHostVolume("cache").Name)
}
