/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package evidence

// Tests for ReconcileAdapters and digestSuffix.
// ReconcileAdapters creates LLMLoraAdapter CRs from pack.Spec.Composition.Adapters;
// digestSuffix extracts the last 8 chars of the OCI digest for use in adapter names.

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// evidenceTestScheme builds a runtime.Scheme with the types needed by
// ReconcileAdapters: serving v1alpha2 (AIPack, LLMLoraAdapter) and apps/v1.
func evidenceTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := servingv1alpha2.AddToScheme(s); err != nil {
		t.Fatalf("servingv1alpha2.AddToScheme: %v", err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatalf("appsv1.AddToScheme: %v", err)
	}
	return s
}

func basePack(name string) *servingv1alpha2.AIPack {
	return &servingv1alpha2.AIPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       "uid-pack-001",
		},
		Spec: servingv1alpha2.AIPackSpec{
			Kind: servingv1alpha2.KindAgent,
		},
	}
}

func baseLLMSvcForEvidence(name string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://test/model", Name: "test-model"},
		},
	}
}

// TestReconcileAdapters_CreatesLLMLoraAdapter_WithCorrectTargetService verifies that
// ReconcileAdapters creates a LLMLoraAdapter CR whose TargetService matches the
// bound LLMInferenceService name, and whose CR name follows the positional pattern.
func TestReconcileAdapters_CreatesLLMLoraAdapter_WithCorrectTargetService(t *testing.T) {
	scheme := evidenceTestScheme(t)

	pack := basePack("my-pack")
	// sha256 digest — last 8 chars of "abcdef0123456789": "01234567" (last 8 of hex after ":")
	pack.Spec.Composition = &servingv1alpha2.AIPackComposition{
		Adapters: []servingv1alpha2.AIPackRef{
			{Ref: "registry.io/ckodex/sql-lora@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		},
	}
	llmSvc := baseLLMSvcForEvidence("target-svc")

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &GovernanceReconciler{Client: c, Scheme: scheme}

	if err := r.ReconcileAdapters(context.Background(), pack, llmSvc); err != nil {
		t.Fatalf("ReconcileAdapters: %v", err)
	}

	// CR name = "{pack.Name}-lora-{index}" → "my-pack-lora-0"
	var lora servingv1alpha2.LLMLoraAdapter
	if err := c.Get(context.Background(), types.NamespacedName{Name: "my-pack-lora-0", Namespace: "default"}, &lora); err != nil {
		t.Fatalf("LLMLoraAdapter my-pack-lora-0 not found: %v", err)
	}
	if lora.Spec.TargetService != "target-svc" {
		t.Errorf("TargetService = %q, want target-svc", lora.Spec.TargetService)
	}
}

// TestReconcileAdapters_Idempotent_ExistingCRNotModified verifies that calling
// ReconcileAdapters twice does not error when the CR already exists.
func TestReconcileAdapters_Idempotent_ExistingCRNotModified(t *testing.T) {
	scheme := evidenceTestScheme(t)

	pack := basePack("idem-pack")
	pack.Spec.Composition = &servingv1alpha2.AIPackComposition{
		Adapters: []servingv1alpha2.AIPackRef{
			{Ref: "registry.io/ckodex/adapter@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef01deadbeef"},
		},
	}
	llmSvc := baseLLMSvcForEvidence("target-svc")

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &GovernanceReconciler{Client: c, Scheme: scheme}

	// First call creates the CR.
	if err := r.ReconcileAdapters(context.Background(), pack, llmSvc); err != nil {
		t.Fatalf("first ReconcileAdapters: %v", err)
	}
	// Second call must not error (idempotent skip of existing CR).
	if err := r.ReconcileAdapters(context.Background(), pack, llmSvc); err != nil {
		t.Fatalf("second ReconcileAdapters (idempotent): %v", err)
	}
}

// --- digestSuffix -----------------------------------------------------------

// TestDigestSuffix_Normal64CharSha256 verifies the standard sha256 digest path:
// last 8 chars of the hex portion after the colon.
func TestDigestSuffix_Normal64CharSha256(t *testing.T) {
	ref := "registry.io/image@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef01deadbeef"
	got := digestSuffix(ref)
	if got != "deadbeef" {
		t.Errorf("digestSuffix = %q, want deadbeef", got)
	}
}

// TestDigestSuffix_ShortRef verifies graceful handling of a very short ref:
// falls back to the last 8 chars of the entire string.
func TestDigestSuffix_ShortRef(t *testing.T) {
	ref := "sha256:abcdef01"
	got := digestSuffix(ref)
	if got != "abcdef01" {
		t.Errorf("digestSuffix short = %q, want abcdef01", got)
	}
}

// TestDigestSuffix_NoColonRef verifies graceful handling when there is no colon
// (a plain image name with no digest) — returns last 8 chars of the whole ref.
func TestDigestSuffix_NoColonRef(t *testing.T) {
	ref := "plainname"
	got := digestSuffix(ref)
	// "plainname" is 9 chars; last 8 = "lainname"
	if got != "lainname" {
		t.Errorf("digestSuffix no-colon = %q, want lainname", got)
	}
}
