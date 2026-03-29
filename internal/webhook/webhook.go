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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const (
	// defaultVLLMImage is the default vLLM container image.
	defaultVLLMImage = "vllm/vllm-openai:latest"
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
	return ctrl.NewWebhookManagedBy(mgr, &servingv1alpha2.LLMInferenceService{}).
		WithValidator(&LLMInferenceServiceValidator{FedRAMPMode: cfg.FedRAMPMode}).
		WithDefaulter(&LLMInferenceServiceDefaulter{HFMirrorURL: cfg.HFMirrorURL}).
		Complete()
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
				"upload the model to an authorized registry and use oci://, s3://, or pvc:// instead")
		}

		validSchemes := []string{"hf://", "hf-mirror://", "s3://", "gs://", "pvc://", "oci://", "seaweedfs://", "http://", "https://"}
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
	}

	// Validate scaling
	if llmSvc.Spec.Scaling != nil {
		if llmSvc.Spec.Scaling.MinReplicas != nil && llmSvc.Spec.Scaling.MaxReplicas != nil {
			if *llmSvc.Spec.Scaling.MinReplicas > *llmSvc.Spec.Scaling.MaxReplicas {
				errs = append(errs, "spec.scaling.minReplicas must be <= spec.scaling.maxReplicas")
			}
		}
	}

	// Validate prefill
	if llmSvc.Spec.Prefill != nil {
		if len(llmSvc.Spec.Prefill.Template.Spec.Containers) == 0 {
			errs = append(errs, "spec.prefill.template.spec.containers must have at least one container")
		}
	}

	if len(errs) > 0 {
		return warnings, fmt.Errorf("validation failed: %s", strings.Join(errs, "; "))
	}

	return warnings, nil
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

	// Default container image if not set
	if len(llmSvc.Spec.Template.Spec.Containers) > 0 {
		c := &llmSvc.Spec.Template.Spec.Containers[0]

		if c.Image == "" {
			c.Image = defaultVLLMImage
		}

		// Inject security context
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

		// Default port
		if len(c.Ports) == 0 {
			c.Ports = []corev1.ContainerPort{
				{Name: "http", ContainerPort: 8000, Protocol: corev1.ProtocolTCP},
				{Name: "grpc", ContainerPort: 8001, Protocol: corev1.ProtocolTCP},
			}
		}
	}

	// Default scheduler replicas
	if llmSvc.Spec.Router.Scheduler.Replicas == nil {
		one := int32(1)
		llmSvc.Spec.Router.Scheduler.Replicas = &one
	}

	return nil
}
