package validation

import (
	"strings"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
)

func TestMultiprocessLMCacheSharedMemoryMounts(t *testing.T) {
	service := func() *servingv1alpha2.LLMInferenceService {
		return &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
			KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
				Connector: "lmcache", LMCache: &servingv1alpha2.LMCacheSpec{Mode: servingv1alpha2.LMCacheModeMultiprocess, EngineRef: &corev1.LocalObjectReference{Name: "shared-kv"}},
			}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "model"}}}},
		}}
	}
	if errs := ValidateLLMInferenceServiceSurface(service()); len(errs) != 0 {
		t.Fatalf("unmounted multiprocess configuration must pass this guard: %v", errs)
	}
	for _, tc := range []struct {
		name      string
		mountPath string
		reject    bool
	}{
		{"shared memory", "/dev/shm", true},
		{"normalized shared memory", "/dev//shm/", true},
		{"shared memory child", "/dev/shm/child", true},
		{"device ancestor", "/dev", true},
		{"root ancestor", "/", true},
		{"similar name", "/dev/shm-other", false},
		{"unrelated mount", "/tmp", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := service()
			svc.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "test", MountPath: tc.mountPath}}
			errs := ValidateLLMInferenceServiceSurface(svc)
			if tc.reject && (len(errs) == 0 || !strings.Contains(errs.ToAggregate().Error(), "spec.template.spec.containers[0].volumeMounts[0].mountPath")) {
				t.Fatalf("expected a shared-memory mount error, got %v", errs)
			}
			if !tc.reject && len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
		})
	}
}

func TestMultiprocessLMCacheRejectsSharedMemoryInAllPodTemplates(t *testing.T) {
	bad := []corev1.VolumeMount{{Name: "shm", MountPath: "/dev/shm"}}
	svc := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
		KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
			Connector: "lmcache", LMCache: &servingv1alpha2.LMCacheSpec{Mode: servingv1alpha2.LMCacheModeMultiprocess},
		}},
		Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers:          []corev1.Container{{VolumeMounts: bad}},
			InitContainers:      []corev1.Container{{VolumeMounts: bad}},
			EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{VolumeMounts: bad}}},
		}},
		Prefill: &servingv1alpha2.PrefillSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{VolumeMounts: bad}}}}},
		Worker:  &servingv1alpha2.WorkerSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{VolumeMounts: bad}}}}},
	}}
	errs := validateMultiprocessLMCacheMounts(svc)
	if len(errs) != 5 {
		t.Fatalf("expected five mount errors, got %v", errs)
	}
	for _, expected := range []string{"spec.template.spec.containers", "spec.template.spec.initContainers", "spec.template.spec.ephemeralContainers", "spec.prefill.template.spec.containers", "spec.worker.template.spec.containers"} {
		if !strings.Contains(errs.ToAggregate().Error(), expected) {
			t.Errorf("missing %s in %v", expected, errs)
		}
	}
	svc.Spec.KVCache.Transfer.LMCache.Mode = servingv1alpha2.LMCacheModeInProcess
	if errs := validateMultiprocessLMCacheMounts(svc); len(errs) != 0 {
		t.Fatalf("in-process configuration must not be rejected by the multiprocess guard: %v", errs)
	}
}
