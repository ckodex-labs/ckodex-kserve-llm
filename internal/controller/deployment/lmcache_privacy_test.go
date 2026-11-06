package deployment

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
)

func TestLMCacheUsageReportingDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		name        string
		templateEnv []corev1.EnvVar
		transferEnv []corev1.EnvVar
		wantTrack   string
		wantDNT     string
	}{
		{name: "private defaults", wantTrack: "false", wantDNT: "1"},
		{
			name: "template values take precedence",
			templateEnv: []corev1.EnvVar{
				{Name: "LMCACHE_TRACK_USAGE", Value: "true"},
				{Name: "DO_NOT_TRACK", Value: "0"},
			},
			transferEnv: []corev1.EnvVar{{Name: "LMCACHE_TRACK_USAGE", Value: "false"}},
			wantTrack:   "true", wantDNT: "0",
		},
		{
			name: "transfer values take precedence over defaults",
			transferEnv: []corev1.EnvVar{
				{Name: "LMCACHE_TRACK_USAGE", Value: "true"},
				{Name: "DO_NOT_TRACK", Value: "0"},
			},
			wantTrack: "true", wantDNT: "0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
				KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{Connector: "lmcache", Env: tc.transferEnv}},
			}}
			pod := &corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm", Env: tc.templateEnv}}}
			(&Builder{}).applyKVTransfer(service, pod, "")
			values := map[string][]string{}
			for _, env := range pod.Containers[0].Env {
				values[env.Name] = append(values[env.Name], env.Value)
			}
			for name, want := range map[string]string{"LMCACHE_TRACK_USAGE": tc.wantTrack, "DO_NOT_TRACK": tc.wantDNT} {
				if got := values[name]; len(got) != 1 || got[0] != want {
					t.Errorf("%s = %v, want one value %q", name, got, want)
				}
			}
		})
	}
}

func TestUsageReportingDefaultsDoNotAffectOtherConnectors(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
		KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{Connector: "nixl"}},
	}}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm"}}}
	(&Builder{}).applyKVTransfer(service, pod, "")
	for _, env := range pod.Containers[0].Env {
		if env.Name == "LMCACHE_TRACK_USAGE" || env.Name == "DO_NOT_TRACK" {
			t.Errorf("unexpected LMCache privacy default on NIXL connector: %s", env.Name)
		}
	}
}

func TestMultiprocessLMCachePreservesTransferEnvironmentAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		templateEnv []corev1.EnvVar
		transferEnv []corev1.EnvVar
		wantTrack   string
		wantDNT     string
	}{
		{name: "private defaults", wantTrack: "false", wantDNT: "1"},
		{
			name:        "template overrides transfer",
			templateEnv: []corev1.EnvVar{{Name: "LMCACHE_TRACK_USAGE", Value: "template"}, {Name: "DO_NOT_TRACK", Value: "0"}},
			transferEnv: []corev1.EnvVar{{Name: "LMCACHE_TRACK_USAGE", Value: "transfer"}, {Name: "OTHER_SETTING", Value: "configured"}},
			wantTrack:   "template", wantDNT: "0",
		},
		{
			name:        "transfer overrides defaults",
			transferEnv: []corev1.EnvVar{{Name: "LMCACHE_TRACK_USAGE", Value: "true"}, {Name: "DO_NOT_TRACK", Value: "0"}, {Name: "OTHER_SETTING", Value: "configured"}},
			wantTrack:   "true", wantDNT: "0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &servingv1alpha2.LLMInferenceService{Spec: servingv1alpha2.LLMInferenceServiceSpec{
				KVCache: &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
					Connector: "lmcache", Env: tc.transferEnv,
					LMCache: &servingv1alpha2.LMCacheSpec{Mode: servingv1alpha2.LMCacheModeMultiprocess, EngineRef: &corev1.LocalObjectReference{Name: "shared-kv"}},
				}},
			}}
			pod := &corev1.PodSpec{Containers: []corev1.Container{{Name: "vllm", Env: tc.templateEnv}}}
			(&Builder{}).applyKVTransfer(service, pod, "")
			values := map[string][]string{}
			for _, env := range pod.Containers[0].Env {
				values[env.Name] = append(values[env.Name], env.Value)
			}
			for name, want := range map[string]string{"LMCACHE_TRACK_USAGE": tc.wantTrack, "DO_NOT_TRACK": tc.wantDNT} {
				if got := values[name]; len(got) != 1 || got[0] != want {
					t.Errorf("%s = %v, want one value %q", name, got, want)
				}
			}
			if len(tc.transferEnv) > 1 {
				if got := values["OTHER_SETTING"]; len(got) != 1 || got[0] != "configured" {
					t.Errorf("OTHER_SETTING = %v, want configured", got)
				}
			}
			if !pod.HostIPC {
				t.Error("multiprocess connector did not configure host IPC")
			}
		})
	}
}
