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
// Registration is currently commented out in cmd/manager/main.go pending TLS cert provisioning.
// When un-commenting, also add `servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"`
// to the main.go imports and register the v1 scheme.
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
