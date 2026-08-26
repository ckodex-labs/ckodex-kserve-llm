/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package webhook wires the conversion webhook for hub-and-spoke versioning.
//
// Conversion architecture:
//   - v1.LLMInferenceService  → Hub (storage version, implements conversion.Hub via Hub() method)
//   - v1alpha2.LLMInferenceService → Spoke (implements ConvertTo / ConvertFrom in api/v1alpha2/)
//
// The conversion webhook is registered in SetupWebhooks via ctrl.NewWebhookManagedBy.
// controller-runtime automatically discovers the conversion methods by interface.
//
// Registration is enabled when the chart's webhook profile is enabled. The beta CRD
// profile points the API server at the /convert endpoint and cert-manager supplies its CA.
package webhook

import (
	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// Compile-time interface assertions. If these fail, the conversion chain is broken.
var (
	_ conversion.Hub         = &servingv1.LLMInferenceService{}
	_ conversion.Convertible = &servingv1alpha2.LLMInferenceService{}
)
