/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package webhook

import (
	"context"
	"fmt"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
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
	if err := ctrl.NewWebhookManagedBy(mgr, &servingv1.LLMInferenceService{}).
		WithValidator(&StableLLMInferenceServiceValidator{FedRAMPMode: cfg.FedRAMPMode}).
		WithDefaulter(&StableLLMInferenceServiceDefaulter{HFMirrorURL: cfg.HFMirrorURL}).
		Complete(); err != nil {
		return fmt.Errorf("setup stable llminferenceservice webhook: %w", err)
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

// ----- Mutating Webhook -----

// LLMInferenceServiceDefaulter injects defaults into LLMInferenceService CRs.
type LLMInferenceServiceDefaulter struct {
	// HFMirrorURL rewrites hf:// model URIs to hf-mirror:// so the storage initializer
	// fetches from the internal mirror rather than the public huggingface.co.
	// When empty, no rewriting occurs.
	HFMirrorURL string
}

// Default sets defaults for LLMInferenceService.
func isHuggingFaceURI(uri string) bool {
	return strings.HasPrefix(uri, "hf://") || strings.HasPrefix(uri, "hf-mount://") || strings.HasPrefix(uri, "hf-mirror://")
}

func uriHasRevision(uri string) bool {
	parts := strings.SplitN(uri, "://", 2)
	return len(parts) == 2 && strings.Contains(parts[1], "@")
}
