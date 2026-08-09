/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package webhook

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// WebhookConfig carries runtime policy settings injected at manager startup.
type WebhookConfig struct {
	// HFMirrorURL, when non-empty, causes the mutating webhook to rewrite hf:// URIs
	// to hf-mirror:// so the storage initializer fetches from the internal mirror.
	// Must include scheme: e.g. "https://hf-mirror.corp.internal".
	HFMirrorURL string

	// FedRAMPMode, when true, causes the validating webhook to reject any
	// LLMInferenceService whose model URI still starts with hf:// — direct
	// HuggingFace access is not permitted in FedRAMP environments.
	FedRAMPMode bool
}

// SetupWebhooks registers all webhooks with the manager using the supplied config.
func SetupWebhooks(mgr ctrl.Manager, cfg WebhookConfig) error {
	if err := ctrl.NewWebhookManagedBy(mgr, &servingv1alpha2.LLMInferenceService{}).
		WithValidator(&LLMInferenceServiceValidator{FedRAMPMode: cfg.FedRAMPMode}).
		WithDefaulter(&LLMInferenceServiceDefaulter{HFMirrorURL: cfg.HFMirrorURL}).
		Complete(); err != nil {
		return fmt.Errorf("setup llminferenceservice webhook: %w", err)
	}
	if err := SetupAIPackWebhook(mgr); err != nil {
		return fmt.Errorf("setup aipack webhook: %w", err)
	}
	return nil
}

// ----- Validating Webhook -----

// LLMInferenceServiceValidator validates LLMInferenceService CRs.
type LLMInferenceServiceValidator struct {
	// FedRAMPMode rejects hf:// URIs — direct HuggingFace access is not FedRAMP-authorized.
	FedRAMPMode bool
}

// ValidateCreate validates a new LLMInferenceService.
func (v *LLMInferenceServiceValidator) ValidateCreate(_ context.Context, llmSvc *servingv1alpha2.LLMInferenceService) (admission.Warnings, error) {
	return v.validate(llmSvc)
}

// ValidateUpdate validates an updated LLMInferenceService.
func (v *LLMInferenceServiceValidator) ValidateUpdate(_ context.Context, _ *servingv1alpha2.LLMInferenceService, newObj *servingv1alpha2.LLMInferenceService) (admission.Warnings, error) {
	return v.validate(newObj)
}

// ValidateDelete validates deletion (no-op).
func (v *LLMInferenceServiceValidator) ValidateDelete(_ context.Context, _ *servingv1alpha2.LLMInferenceService) (admission.Warnings, error) {
	return nil, nil
}

func (v *LLMInferenceServiceValidator) validate(llmSvc *servingv1alpha2.LLMInferenceService) (admission.Warnings, error) {
	var errs []string
	var warnings admission.Warnings

	// Validate model URI
	if llmSvc.Spec.Model.URI == "" {
		errs = append(errs, "spec.model.uri is required")
	} else {
		// FedRAMP: direct HuggingFace downloads are not permitted — all model artifacts must
		// originate from a FedRAMP-authorized registry. Reject hf:// URIs at admission time
		// so operators don't accidentally route traffic outside the authorization boundary.
		if v.FedRAMPMode && strings.HasPrefix(llmSvc.Spec.Model.URI, "hf://") {
			errs = append(errs, "spec.model.uri: hf:// URIs are not permitted in FedRAMP mode; "+
				"upload the model to an authorized registry and use oci://, ocis://, s3://, or pvc:// instead")
		}

		uriLower := strings.ToLower(llmSvc.Spec.Model.URI)

		// Security Hardening: block credential smuggling inside URIs
		if strings.Contains(llmSvc.Spec.Model.URI, "@") && (strings.HasPrefix(uriLower, "http://") || strings.HasPrefix(uriLower, "https://")) {
			errs = append(errs, "spec.model.uri containing embedded credentials (user:pass@...) is forbidden to prevent SSRF")
		}

		// Security Hardening: block unsafe tensor formats
		if strings.HasSuffix(uriLower, ".pkl") || strings.HasSuffix(uriLower, ".bin") || strings.HasSuffix(uriLower, ".pt") {
			errs = append(errs, "spec.model.uri pointing to unsafe formats (.pkl, .bin, .pt) is forbidden; use .safetensors")
		}

		validSchemes := []string{"hf://", "hf-mount://", "hf-mirror://", "s3://", "swfs://", "gs://", "pvc://", "oci://", "ocis://", "modelpack://", "seaweedfs://", "http://", "https://"}
		valid := false
		for _, scheme := range validSchemes {
			if strings.HasPrefix(llmSvc.Spec.Model.URI, scheme) {
				valid = true
				break
			}
		}
		if !valid {
			errs = append(errs, fmt.Sprintf("spec.model.uri must start with one of: %v", validSchemes))
		}

		if llmSvc.Spec.Model.Revision != "" {
			if !isHuggingFaceURI(uriLower) {
				errs = append(errs, "spec.model.revision is supported only for hf://, hf-mount://, or hf-mirror:// URIs")
			}
			if uriHasRevision(llmSvc.Spec.Model.URI) {
				errs = append(errs, "spec.model.revision cannot be combined with @revision URI syntax")
			}
		}
	}

	// Validate model name
	if llmSvc.Spec.Model.Name == "" {
		errs = append(errs, "spec.model.name is required")
	}

	// Validate containers
	if len(llmSvc.Spec.Template.Spec.Containers) == 0 {
		errs = append(errs, "spec.template.spec.containers must have at least one container")
	}

	// Validate parallelism + GPU resources
	if llmSvc.Spec.Parallelism != nil {
		tp := int32(1)
		if llmSvc.Spec.Parallelism.Tensor != nil {
			tp = *llmSvc.Spec.Parallelism.Tensor
		}
		if tp > 1 && len(llmSvc.Spec.Template.Spec.Containers) > 0 {
			gpuRes := llmSvc.Spec.Template.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"]
			if gpuRes.IsZero() {
				warnings = append(warnings, fmt.Sprintf(
					"tensor parallelism=%d requires GPU resources; nvidia.com/gpu limit not set", tp))
			}
		}
		if len(llmSvc.Spec.Parallelism.GPUDevices) > 0 {
			if len(llmSvc.Spec.Template.Spec.Containers) == 0 {
				errs = append(errs, "spec.parallelism.gpuDevices requires a primary container")
			} else {
				if tp != int32(len(llmSvc.Spec.Parallelism.GPUDevices)) {
					errs = append(errs, fmt.Sprintf(
						"spec.parallelism.gpuDevices has %d devices but tensor parallelism is %d",
						len(llmSvc.Spec.Parallelism.GPUDevices), tp))
				}
				if err := validateGPUDevices(llmSvc.Spec.Parallelism.GPUDevices); err != nil {
					errs = append(errs, err.Error())
				}
			}
		}
	}

	// Validate scaling
	if llmSvc.Spec.Scaling != nil {
		if llmSvc.Spec.Scaling.MinReplicas != nil && llmSvc.Spec.Scaling.MaxReplicas != nil {
			if *llmSvc.Spec.Scaling.MinReplicas > *llmSvc.Spec.Scaling.MaxReplicas {
				errs = append(errs, "spec.scaling.minReplicas must be <= spec.scaling.maxReplicas")
			}
		}
	}

	// Phase 5 Hardening: Advanced Validation
	if err := v.validateResources(llmSvc, &warnings); err != nil {
		errs = append(errs, err.Error())
	}
	if err := v.validateParallelism(llmSvc, &warnings); err != nil {
		errs = append(errs, err.Error())
	}

	// Validate prefill
	if llmSvc.Spec.Prefill != nil {
		if len(llmSvc.Spec.Prefill.Template.Spec.Containers) == 0 {
			errs = append(errs, "spec.prefill.template.spec.containers must have at least one container")
		}
		if llmSvc.Spec.KVCache == nil || llmSvc.Spec.KVCache.Transfer == nil || llmSvc.Spec.KVCache.Transfer.Connector == "" {
			errs = append(errs, "spec.prefill requires spec.kvCache.transfer.connector (nixl, lmcache, or mooncake)")
		}
	}

	if llmSvc.Spec.KVCache != nil && llmSvc.Spec.KVCache.Transfer != nil {
		t := llmSvc.Spec.KVCache.Transfer
		if t.LMCache != nil {
			if t.Connector != "lmcache" {
				errs = append(errs, "spec.kvCache.transfer.lmcache requires connector=lmcache")
			}
			mode := t.LMCache.Mode
			if mode == "" {
				mode = servingv1alpha2.LMCacheModeInProcess
			}
			switch mode {
			case servingv1alpha2.LMCacheModeInProcess:
				if t.LMCache.EngineRef != nil {
					errs = append(errs, "spec.kvCache.transfer.lmcache.engineRef is valid only in multiprocess mode")
				}
			case servingv1alpha2.LMCacheModeMultiprocess:
				if t.LMCache.EngineRef == nil || t.LMCache.EngineRef.Name == "" {
					errs = append(errs, "spec.kvCache.transfer.lmcache.engineRef.name is required in multiprocess mode")
				}
			}
		}
	}

	if llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Canary != nil {
		errs = append(errs, "spec.router.scheduler cannot be combined with spec.canary until both InferencePools are explicitly modeled")
	}
	if llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Worker != nil {
		errs = append(errs, "spec.router.scheduler is not supported with KServe multi-node workerSpec")
	}

	if len(errs) > 0 {
		return warnings, fmt.Errorf("validation failed: %s", strings.Join(errs, "; "))
	}

	return warnings, nil
}

func validateGPUDevices(devices []string) error {
	seen := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		device = strings.TrimSpace(device)
		if device == "" {
			return fmt.Errorf("spec.parallelism.gpuDevices must not contain empty values")
		}
		if _, ok := seen[device]; ok {
			return fmt.Errorf("spec.parallelism.gpuDevices contains duplicate device %q", device)
		}
		seen[device] = struct{}{}
	}
	return nil
}

func (v *LLMInferenceServiceValidator) validateResources(llmSvc *servingv1alpha2.LLMInferenceService, warnings *admission.Warnings) error {
	if len(llmSvc.Spec.Template.Spec.Containers) == 0 {
		return nil
	}
	c := &llmSvc.Spec.Template.Spec.Containers[0]

	// Enforce Guaranteed QoS for GPU workloads (requests == limits)
	gpuReq := c.Resources.Requests["nvidia.com/gpu"]
	gpuLimit := c.Resources.Limits["nvidia.com/gpu"]

	if !gpuReq.IsZero() || !gpuLimit.IsZero() {
		// Ensure requests match limits for CPU and Memory if GPU is involved
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
	}
	return nil
}

func (v *LLMInferenceServiceValidator) validateParallelism(llmSvc *servingv1alpha2.LLMInferenceService, warnings *admission.Warnings) error {
	if llmSvc.Spec.Parallelism == nil {
		return nil
	}

	tp := int32(1)
	if llmSvc.Spec.Parallelism.Tensor != nil {
		tp = *llmSvc.Spec.Parallelism.Tensor
	}

	// 1. Tensor Parallel must be a power of 2
	if tp > 0 && (tp&(tp-1)) != 0 {
		return fmt.Errorf("tensor parallelism (%d) must be a power of 2 (1, 2, 4, 8, etc.)", tp)
	}

	// 2. TP size must not exceed requested GPUs
	if len(llmSvc.Spec.Template.Spec.Containers) > 0 {
		gpuLimit := llmSvc.Spec.Template.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"]
		gpus := int32(gpuLimit.Value())
		if tp > gpus && gpus > 0 {
			return fmt.Errorf("tensor parallelism (%d) exceeds requested GPUs (%d)", tp, gpus)
		}
	}

	return nil
}

// ----- Mutating Webhook -----

// LLMInferenceServiceDefaulter injects defaults into LLMInferenceService CRs.
type LLMInferenceServiceDefaulter struct {
	// HFMirrorURL rewrites hf:// model URIs to hf-mirror:// so the storage initializer
	// fetches from the internal mirror rather than the public huggingface.co.
	// When empty, no rewriting occurs.
	HFMirrorURL string
}

// Default sets defaults for LLMInferenceService.
func (d *LLMInferenceServiceDefaulter) Default(_ context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	// Rewrite hf:// URIs to hf-mirror:// when an internal mirror is configured.
	// This allows tenants to continue using the public hf:// scheme in their CRs
	// while the operator transparently routes downloads through the internal mirror.
	if d.HFMirrorURL != "" && strings.HasPrefix(llmSvc.Spec.Model.URI, "hf://") {
		llmSvc.Spec.Model.URI = "hf-mirror://" + strings.TrimPrefix(llmSvc.Spec.Model.URI, "hf://")
	}

	// Default replicas
	if llmSvc.Spec.Replicas == nil {
		one := int32(1)
		llmSvc.Spec.Replicas = &one
	}

	// Leave an empty image for the controller to resolve from the live
	// CKODEX_RUNTIME_IMAGE configuration. Admission-time image defaulting would
	// persist a stale value and prevent the reconciler from applying that setting.
	if len(llmSvc.Spec.Template.Spec.Containers) > 0 {
		c := &llmSvc.Spec.Template.Spec.Containers[0]

		// Inject security context
		if c.SecurityContext == nil {
			c.SecurityContext = &corev1.SecurityContext{}
		}
		// Security hardening: enforce runAsNonRoot to prevent container escape
		if c.SecurityContext.RunAsNonRoot == nil {
			t := true
			c.SecurityContext.RunAsNonRoot = &t
		}
		if c.SecurityContext.AllowPrivilegeEscalation == nil {
			f := false
			c.SecurityContext.AllowPrivilegeEscalation = &f
		}

		// Default port
		if len(c.Ports) == 0 {
			c.Ports = []corev1.ContainerPort{
				{Name: "http", ContainerPort: 8000, Protocol: corev1.ProtocolTCP},
				{Name: "grpc", ContainerPort: 8001, Protocol: corev1.ProtocolTCP},
			}
		}
	}

	// Default scheduler replicas only after the user explicitly opts in.
	if llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Replicas == nil {
		one := int32(1)
		llmSvc.Spec.Router.Scheduler.Replicas = &one
	}

	if llmSvc.Spec.KVCache != nil && llmSvc.Spec.KVCache.Transfer != nil && llmSvc.Spec.KVCache.Transfer.LMCache != nil {
		lmcache := llmSvc.Spec.KVCache.Transfer.LMCache
		if lmcache.Mode == "" {
			lmcache.Mode = servingv1alpha2.LMCacheModeInProcess
		}
		if lmcache.Mode == servingv1alpha2.LMCacheModeInProcess {
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
	}

	return nil
}

func isHuggingFaceURI(uri string) bool {
	return strings.HasPrefix(uri, "hf://") || strings.HasPrefix(uri, "hf-mount://") || strings.HasPrefix(uri, "hf-mirror://")
}

func uriHasRevision(uri string) bool {
	parts := strings.SplitN(uri, "://", 2)
	return len(parts) == 2 && strings.Contains(parts[1], "@")
}
