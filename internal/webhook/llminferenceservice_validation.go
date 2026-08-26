/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package webhook

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	domainvalidation "github.com/ckodex-labs/kserve-llm-operator/internal/validation"
)

type validationState struct {
	errs     []string
	warnings admission.Warnings
}

func (v *LLMInferenceServiceValidator) validate(llmSvc *servingv1alpha2.LLMInferenceService) (admission.Warnings, error) {
	state := validationState{}
	state.validateSurface(llmSvc)
	state.validateEngine(llmSvc)
	state.validateModel(v, llmSvc)
	state.validateModelName(llmSvc)
	state.validateContainers(llmSvc)
	state.validateParallelismWarning(llmSvc)
	state.validateScaling(llmSvc)
	state.validateAdvanced(v, llmSvc)
	state.validatePrefill(llmSvc)
	state.validateKVCache(llmSvc)
	state.validateRouterConstraints(llmSvc)
	return state.result()
}

func (s *validationState) validateSurface(llmSvc *servingv1alpha2.LLMInferenceService) {
	if errs := domainvalidation.ValidateLLMInferenceServiceSurface(llmSvc); len(errs) > 0 {
		s.errs = append(s.errs, errs.ToAggregate().Error())
	}
}

func (s *validationState) validateEngine(llmSvc *servingv1alpha2.LLMInferenceService) {
	if err := domainvalidation.ValidateInferenceEngine(llmSvc.Spec.Engine); err != nil {
		s.errs = append(s.errs, err.Error())
	}
}

func (s *validationState) validateModel(v *LLMInferenceServiceValidator, llmSvc *servingv1alpha2.LLMInferenceService) {
	uri := llmSvc.Spec.Model.URI
	if uri == "" {
		s.errs = append(s.errs, "spec.model.uri is required")
		return
	}
	if v.FedRAMPMode && strings.HasPrefix(uri, "hf://") {
		s.errs = append(s.errs, "spec.model.uri: hf:// URIs are not permitted in FedRAMP mode; "+
			"upload the model to an authorized registry and use oci://, ocis://, s3://, or pvc:// instead")
	}
	s.validateModelSecurity(uri)
	s.validateModelScheme(uri)
	s.validateModelRevision(llmSvc)
}

func (s *validationState) validateModelSecurity(uri string) {
	uriLower := strings.ToLower(uri)
	if strings.Contains(uri, "@") && (strings.HasPrefix(uriLower, "http://") || strings.HasPrefix(uriLower, "https://")) {
		s.errs = append(s.errs, "spec.model.uri containing embedded credentials (user:pass@...) is forbidden to prevent SSRF")
	}
	if strings.HasSuffix(uriLower, ".pkl") || strings.HasSuffix(uriLower, ".bin") || strings.HasSuffix(uriLower, ".pt") {
		s.errs = append(s.errs, "spec.model.uri pointing to unsafe formats (.pkl, .bin, .pt) is forbidden; use .safetensors")
	}
}

func (s *validationState) validateModelScheme(uri string) {
	validSchemes := []string{"hf://", "hf-mount://", "hf-mirror://", "s3://", "swfs://", "gs://", "pvc://", "oci://", "ocis://", "modelpack://", "seaweedfs://", "http://", "https://"}
	for _, scheme := range validSchemes {
		if strings.HasPrefix(uri, scheme) {
			return
		}
	}
	s.errs = append(s.errs, fmt.Sprintf("spec.model.uri must start with one of: %v", validSchemes))
}

func (s *validationState) validateModelRevision(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.Model.Revision == "" {
		return
	}
	uri := llmSvc.Spec.Model.URI
	if !isHuggingFaceURI(strings.ToLower(uri)) {
		s.errs = append(s.errs, "spec.model.revision is supported only for hf://, hf-mount://, or hf-mirror:// URIs")
	}
	if uriHasRevision(uri) {
		s.errs = append(s.errs, "spec.model.revision cannot be combined with @revision URI syntax")
	}
}

func (s *validationState) validateModelName(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.Model.Name == "" {
		s.errs = append(s.errs, "spec.model.name is required")
	}
}

func (s *validationState) validateContainers(llmSvc *servingv1alpha2.LLMInferenceService) {
	if len(llmSvc.Spec.Template.Spec.Containers) == 0 {
		s.errs = append(s.errs, "spec.template.spec.containers must have at least one container")
	}
}

func (s *validationState) validateParallelismWarning(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.Parallelism == nil || len(llmSvc.Spec.Template.Spec.Containers) == 0 {
		return
	}
	tp := int32(1)
	if llmSvc.Spec.Parallelism.Tensor != nil {
		tp = *llmSvc.Spec.Parallelism.Tensor
	}
	gpuRes := llmSvc.Spec.Template.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"]
	if tp > 1 && gpuRes.IsZero() {
		s.warnings = append(s.warnings, fmt.Sprintf("tensor parallelism=%d requires GPU resources; nvidia.com/gpu limit not set", tp))
	}
}

func (s *validationState) validateScaling(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.Scaling == nil || llmSvc.Spec.Scaling.MinReplicas == nil || llmSvc.Spec.Scaling.MaxReplicas == nil {
		return
	}
	if *llmSvc.Spec.Scaling.MinReplicas > *llmSvc.Spec.Scaling.MaxReplicas {
		s.errs = append(s.errs, "spec.scaling.minReplicas must be <= spec.scaling.maxReplicas")
	}
}

func (s *validationState) validateAdvanced(v *LLMInferenceServiceValidator, llmSvc *servingv1alpha2.LLMInferenceService) {
	if err := v.validateResources(llmSvc); err != nil {
		s.errs = append(s.errs, err.Error())
	}
	if err := v.validateParallelism(llmSvc); err != nil {
		s.errs = append(s.errs, err.Error())
	}
}

func (s *validationState) validatePrefill(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.Prefill == nil {
		return
	}
	if len(llmSvc.Spec.Prefill.Template.Spec.Containers) == 0 {
		s.errs = append(s.errs, "spec.prefill.template.spec.containers must have at least one container")
	}
	transfer := llmSvc.Spec.KVCache != nil && llmSvc.Spec.KVCache.Transfer != nil
	if !transfer || llmSvc.Spec.KVCache.Transfer.Connector == "" {
		s.errs = append(s.errs, "spec.prefill requires spec.kvCache.transfer.connector (nixl, lmcache, or mooncake)")
	}
}

func (s *validationState) validateKVCache(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.KVCache == nil || llmSvc.Spec.KVCache.Transfer == nil || llmSvc.Spec.KVCache.Transfer.LMCache == nil {
		return
	}
	t := llmSvc.Spec.KVCache.Transfer
	if t.Connector != "lmcache" {
		s.errs = append(s.errs, "spec.kvCache.transfer.lmcache requires connector=lmcache")
	}
	mode := t.LMCache.Mode
	if mode == "" {
		mode = servingv1alpha2.LMCacheModeInProcess
	}
	s.validateLMCacheMode(t, mode)
}

func (s *validationState) validateLMCacheMode(t *servingv1alpha2.KVTransferSpec, mode servingv1alpha2.LMCacheMode) {
	switch mode {
	case servingv1alpha2.LMCacheModeInProcess:
		if t.LMCache.EngineRef != nil {
			s.errs = append(s.errs, "spec.kvCache.transfer.lmcache.engineRef is valid only in multiprocess mode")
		}
	case servingv1alpha2.LMCacheModeMultiprocess:
		if t.LMCache.EngineRef == nil || t.LMCache.EngineRef.Name == "" {
			s.errs = append(s.errs, "spec.kvCache.transfer.lmcache.engineRef.name is required in multiprocess mode")
		}
	}
}

func (s *validationState) validateRouterConstraints(llmSvc *servingv1alpha2.LLMInferenceService) {
	if llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Canary != nil {
		s.errs = append(s.errs, "spec.router.scheduler cannot be combined with spec.canary until both InferencePools are explicitly modeled")
	}
	if llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Worker != nil {
		s.errs = append(s.errs, "spec.router.scheduler is not supported with KServe multi-node workerSpec")
	}
}

func (s *validationState) result() (admission.Warnings, error) {
	if len(s.errs) > 0 {
		return s.warnings, fmt.Errorf("validation failed: %s", strings.Join(s.errs, "; "))
	}
	return s.warnings, nil
}

func (v *LLMInferenceServiceValidator) validateResources(llmSvc *servingv1alpha2.LLMInferenceService) error {
	if len(llmSvc.Spec.Template.Spec.Containers) == 0 {
		return nil
	}
	c := &llmSvc.Spec.Template.Spec.Containers[0]
	gpuReq := c.Resources.Requests["nvidia.com/gpu"]
	gpuLimit := c.Resources.Limits["nvidia.com/gpu"]
	if gpuReq.IsZero() && gpuLimit.IsZero() {
		return nil
	}
	return validateMatchingResources(c, gpuReq, gpuLimit)
}

func validateMatchingResources(c *corev1.Container, gpuReq, gpuLimit resource.Quantity) error {
	cpuReq := c.Resources.Requests[corev1.ResourceCPU]
	cpuLim := c.Resources.Limits[corev1.ResourceCPU]
	if !cpuReq.IsZero() && !cpuLim.IsZero() && cpuReq.Cmp(cpuLim) != 0 {
		return fmt.Errorf("CPU requests (%s) must match limits (%s) for GPU workloads to ensure Guaranteed QoS", cpuReq.String(), cpuLim.String())
	}
	memReq := c.Resources.Requests[corev1.ResourceMemory]
	memLim := c.Resources.Limits[corev1.ResourceMemory]
	if !memReq.IsZero() && !memLim.IsZero() && memReq.Cmp(memLim) != 0 {
		return fmt.Errorf("memory requests (%s) must match limits (%s) for GPU workloads to ensure Guaranteed QoS", memReq.String(), memLim.String())
	}
	if !gpuReq.IsZero() && !gpuLimit.IsZero() && gpuReq.Cmp(gpuLimit) != 0 {
		return fmt.Errorf("GPU requests (%s) must match limits (%s)", gpuReq.String(), gpuLimit.String())
	}
	return nil
}

func (v *LLMInferenceServiceValidator) validateParallelism(llmSvc *servingv1alpha2.LLMInferenceService) error {
	if llmSvc.Spec.Parallelism == nil {
		return nil
	}
	tp := int32(1)
	if llmSvc.Spec.Parallelism.Tensor != nil {
		tp = *llmSvc.Spec.Parallelism.Tensor
	}
	if tp > 0 && (tp&(tp-1)) != 0 {
		return fmt.Errorf("tensor parallelism (%d) must be a power of 2 (1, 2, 4, 8, etc.)", tp)
	}
	return validateParallelismGPU(llmSvc, tp)
}

func validateParallelismGPU(llmSvc *servingv1alpha2.LLMInferenceService, tp int32) error {
	if len(llmSvc.Spec.Template.Spec.Containers) == 0 {
		return nil
	}
	gpuLimit := llmSvc.Spec.Template.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"]
	gpus := int32(gpuLimit.Value())
	if tp > gpus && gpus > 0 {
		return fmt.Errorf("tensor parallelism (%d) exceeds requested GPUs (%d)", tp, gpus)
	}
	return nil
}
