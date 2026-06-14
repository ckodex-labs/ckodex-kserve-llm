/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package aipack_conformance — predicate conformance vectors (§6-§7).
// V-PRED-NNN  = valid/pass
// I-PRED-NNN  = invalid/fail
package aipack_conformance

import (
	"testing"

	v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/aipack"
)

// TestRequiredPredicates validates RequiredPredicates returns the correct set per §6.
// V-PRED-001: BaseModel has at least SLSA + CycloneDX + AI-BOM required predicates.
// V-PRED-002: Agent (C1) has at least agent:composition + SLSA predicates.
func TestRequiredPredicates(t *testing.T) {
	t.Run("V-PRED-001", func(t *testing.T) {
		preds := aipack.RequiredPredicates(v1alpha2.KindBaseModel)
		if len(preds) == 0 {
			t.Fatal("[V-PRED-001] BaseModel must have at least one required predicate")
		}
		mustContain(t, "V-PRED-001", preds, v1alpha2.PredSLSAProvenance)
		mustContain(t, "V-PRED-001", preds, v1alpha2.PredCycloneDXBOM)
		mustContain(t, "V-PRED-001", preds, v1alpha2.PredAIBOM)
	})
	t.Run("V-PRED-002", func(t *testing.T) {
		preds := aipack.RequiredPredicates(v1alpha2.KindAgent)
		if len(preds) == 0 {
			t.Fatal("[V-PRED-002] Agent must have at least one required predicate")
		}
		mustContain(t, "V-PRED-002", preds, v1alpha2.PredSLSAProvenance)
		mustContain(t, "V-PRED-002", preds, v1alpha2.PredAgentComposition)
	})
}

// TestMissingPredicates validates that MissingPredicates returns the gap between
// required and present predicates.
// I-PRED-001: empty presented set → all required BaseModel predicates are missing.
func TestMissingPredicates(t *testing.T) {
	t.Run("I-PRED-001", func(t *testing.T) {
		required := aipack.RequiredPredicates(v1alpha2.KindBaseModel)
		missing := aipack.MissingPredicates(v1alpha2.KindBaseModel, nil)
		if len(missing) != len(required) {
			t.Fatalf("[I-PRED-001] want %d missing predicates, got %d", len(required), len(missing))
		}
	})
}

// mustContain is a test helper that fails when want is not in the slice.
func mustContain(t *testing.T, id string, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Fatalf("[%s] expected %q in predicate set, not found", id, want)
}
