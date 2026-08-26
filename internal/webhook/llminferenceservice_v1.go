/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package webhook

import (
	"context"
	"fmt"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// StableLLMInferenceServiceValidator adapts the shared validation policy to the
// storage version. Validation remains single-sourced in LLMInferenceServiceValidator
// so v1 and v1alpha2 cannot drift apart.
type StableLLMInferenceServiceValidator struct {
	FedRAMPMode bool
}

func (v *StableLLMInferenceServiceValidator) ValidateCreate(
	_ context.Context, llmSvc *servingv1.LLMInferenceService,
) (admission.Warnings, error) {
	alpha, err := convertStableToAlpha(llmSvc)
	if err != nil {
		return nil, err
	}
	return (&LLMInferenceServiceValidator{FedRAMPMode: v.FedRAMPMode}).validate(alpha)
}

func (v *StableLLMInferenceServiceValidator) ValidateUpdate(
	_ context.Context, _ *servingv1.LLMInferenceService, newObj *servingv1.LLMInferenceService,
) (admission.Warnings, error) {
	alpha, err := convertStableToAlpha(newObj)
	if err != nil {
		return nil, err
	}
	return (&LLMInferenceServiceValidator{FedRAMPMode: v.FedRAMPMode}).validate(alpha)
}

func (v *StableLLMInferenceServiceValidator) ValidateDelete(
	_ context.Context, llmSvc *servingv1.LLMInferenceService,
) (admission.Warnings, error) {
	if _, err := convertStableToAlpha(llmSvc); err != nil {
		return nil, err
	}
	return nil, nil
}

// StableLLMInferenceServiceDefaulter adapts the shared defaulting policy to the
// storage version and converts the resulting object back before admission returns.
type StableLLMInferenceServiceDefaulter struct {
	HFMirrorURL string
}

func (d *StableLLMInferenceServiceDefaulter) Default(
	ctx context.Context, llmSvc *servingv1.LLMInferenceService,
) error {
	alpha, err := convertStableToAlpha(llmSvc)
	if err != nil {
		return err
	}
	if err := (&LLMInferenceServiceDefaulter{HFMirrorURL: d.HFMirrorURL}).Default(ctx, alpha); err != nil {
		return err
	}
	if err := alpha.ConvertTo(llmSvc); err != nil {
		return fmt.Errorf("convert defaulted LLMInferenceService to v1: %w", err)
	}
	return nil
}

func convertStableToAlpha(src *servingv1.LLMInferenceService) (*servingv1alpha2.LLMInferenceService, error) {
	if src.Spec.Router.Scheduler != nil && src.Spec.Router.Scheduler.Config != nil && src.Spec.Router.Scheduler.Config.Inline != nil {
		return nil, fmt.Errorf("spec.router.scheduler.config.inline is not supported in stable v1 because its alpha scheduler profile cannot be represented losslessly")
	}
	dst := &servingv1alpha2.LLMInferenceService{}
	if err := dst.ConvertFrom(src); err != nil {
		return nil, fmt.Errorf("convert v1 LLMInferenceService to v1alpha2: %w", err)
	}
	return dst, nil
}
