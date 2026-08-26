/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package webhook

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (d *LLMInferenceServiceDefaulter) Default(_ context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	d.defaultModelURI(llmSvc)
	d.defaultReplicas(llmSvc)
	d.defaultContainer(llmSvc)
	d.defaultSchedulerReplicas(llmSvc)
	d.defaultLMCache(llmSvc)
	return nil
}

func (d *LLMInferenceServiceDefaulter) defaultModelURI(llmSvc *servingv1alpha2.LLMInferenceService) {
	if d.HFMirrorURL != "" && strings.HasPrefix(llmSvc.Spec.Model.URI, "hf://") {
		llmSvc.Spec.Model.URI = "hf-mirror://" + strings.TrimPrefix(llmSvc.Spec.Model.URI, "hf://")
	}
}

func (d *LLMInferenceServiceDefaulter) defaultReplicas(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.Replicas == nil {
		one := int32(1)
		llmSvc.Spec.Replicas = &one
	}
}

func (d *LLMInferenceServiceDefaulter) defaultContainer(llmSvc *servingv1alpha2.LLMInferenceService) {
	if len(llmSvc.Spec.Template.Spec.Containers) == 0 {
		return
	}
	c := &llmSvc.Spec.Template.Spec.Containers[0]
	defaultSecurityContext(c)
	defaultContainerPorts(c)
}

func defaultSecurityContext(c *corev1.Container) {
	if c.SecurityContext == nil {
		c.SecurityContext = &corev1.SecurityContext{}
	}
	if c.SecurityContext.RunAsNonRoot == nil {
		t := true
		c.SecurityContext.RunAsNonRoot = &t
	}
	if c.SecurityContext.AllowPrivilegeEscalation == nil {
		f := false
		c.SecurityContext.AllowPrivilegeEscalation = &f
	}
}

func defaultContainerPorts(c *corev1.Container) {
	if len(c.Ports) == 0 {
		c.Ports = []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8000, Protocol: corev1.ProtocolTCP},
			{Name: "grpc", ContainerPort: 8001, Protocol: corev1.ProtocolTCP},
		}
	}
}

func (d *LLMInferenceServiceDefaulter) defaultSchedulerReplicas(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Replicas == nil {
		one := int32(1)
		llmSvc.Spec.Router.Scheduler.Replicas = &one
	}
}

func (d *LLMInferenceServiceDefaulter) defaultLMCache(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.KVCache == nil || llmSvc.Spec.KVCache.Transfer == nil || llmSvc.Spec.KVCache.Transfer.LMCache == nil {
		return
	}
	lmcache := llmSvc.Spec.KVCache.Transfer.LMCache
	if lmcache.Mode == "" {
		lmcache.Mode = servingv1alpha2.LMCacheModeInProcess
	}
	if lmcache.Mode == servingv1alpha2.LMCacheModeInProcess {
		defaultLMCacheResources(lmcache)
	}
}

func defaultLMCacheResources(lmcache *servingv1alpha2.LMCacheSpec) {
	if lmcache.ChunkSize == nil {
		lmcache.ChunkSize = ptr.To(int32(256))
	}
	if lmcache.LocalCPU == nil {
		lmcache.LocalCPU = ptr.To(true)
	}
	if lmcache.LocalCPUSizeGiB == nil {
		lmcache.LocalCPUSizeGiB = ptr.To(int32(20))
	}
}
