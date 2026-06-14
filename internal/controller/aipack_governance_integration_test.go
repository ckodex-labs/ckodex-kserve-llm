/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

// Integration tests for the AIPack → LLMInferenceService governance flow.
//
// These tests verify that:
//   - AIPacks bound to an LLMInferenceService via the workload label are
//     included in governance reconciliation.
//   - The Compliance-SR-2-AIPack condition is set correctly based on
//     attestation state.
//   - Unbound AIPacks (wrong label or different namespace) are excluded.
//
// All tests use the fake client; no envtest / API server required.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const (
	aipackGovernanceCondition = "Compliance-SR-2-AIPack"
	workloadLabel             = "serving.ckodex.com/workload"
	testLLMName               = "llm-svc"
	testNamespace             = "default"
	modelDigest               = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// makeBaseModelAttestations returns a fully attested AIPackAttestation for a
// KindBaseModel artifact (all 4 required predicates present).
func makeBaseModelAttestations() *servingv1alpha2.AIPackAttestation {
	return &servingv1alpha2.AIPackAttestation{
		Predicates: []servingv1alpha2.PredicateEntry{
			{PredicateURI: servingv1alpha2.PredSLSAProvenance, Required: true},
			{PredicateURI: servingv1alpha2.PredCycloneDXBOM, Required: true},
			{PredicateURI: servingv1alpha2.PredAIBOM, Required: true},
			{PredicateURI: servingv1alpha2.PredTrainingResidency, Required: true},
		},
	}
}

// makeAIPackWithLabel constructs an AIPack that binds to llmName via workload label.
func makeAIPackWithLabel(name, ns, llmName string, kind servingv1alpha2.ArtifactKind, attestation *servingv1alpha2.AIPackAttestation) *servingv1alpha2.AIPack {
	pack := &servingv1alpha2.AIPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{workloadLabel: llmName},
		},
		Spec: servingv1alpha2.AIPackSpec{
			Kind:        kind,
			Attestation: attestation,
			Source: servingv1alpha2.AIPackSource{
				Ref: "registry.example.com/models/llama3@" + modelDigest,
			},
		},
	}
	return pack
}

// runGovernanceReconcile creates a fake client pre-populated with the given
// objects, runs one reconcile cycle for the named LLMInferenceService, and
// returns the updated LLMInferenceService.
func runGovernanceReconcile(
	t *testing.T,
	llmName string,
	objects ...interface{},
) *servingv1alpha2.LLMInferenceService {
	t.Helper()
	s := buildLLMScheme(t)
	builder := fake.NewClientBuilder().WithScheme(s)
	for _, obj := range objects {
		switch o := obj.(type) {
		case *servingv1alpha2.LLMInferenceService:
			builder = builder.WithObjects(o).WithStatusSubresource(o)
		case *servingv1alpha2.AIPack:
			builder = builder.WithObjects(o)
		}
	}
	cl := builder.Build()
	r := setupReconciler(cl, s)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: llmName, Namespace: testNamespace},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.LLMInferenceService
	require.NoError(t, cl.Get(context.Background(),
		k8stypes.NamespacedName{Name: llmName, Namespace: testNamespace},
		&updated,
	))
	return &updated
}

// TestAIPackGovernance_NoAIPacks verifies that an LLMInferenceService with no
// associated AIPacks gets Compliance-SR-2-AIPack = True / NoAIPacksAssociated.
func TestAIPackGovernance_NoAIPacks(t *testing.T) {
	llmSvc := makeLLMInferenceService(testLLMName, testNamespace)
	updated := runGovernanceReconcile(t, testLLMName, llmSvc)

	cond := apimeta.FindStatusCondition(updated.Status.Conditions, aipackGovernanceCondition)
	require.NotNil(t, cond, "Compliance-SR-2-AIPack condition must be present")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "NoAIPacksAssociated", cond.Reason)
}

// TestAIPackGovernance_FullyAttestedPack verifies that a bound AIPack with all
// required predicates present results in Compliance-SR-2-AIPack = True.
func TestAIPackGovernance_FullyAttestedPack(t *testing.T) {
	llmSvc := makeLLMInferenceService(testLLMName, testNamespace)
	pack := makeAIPackWithLabel("llama3", testNamespace, testLLMName,
		servingv1alpha2.KindBaseModel, makeBaseModelAttestations())

	updated := runGovernanceReconcile(t, testLLMName, llmSvc, pack)

	cond := apimeta.FindStatusCondition(updated.Status.Conditions, aipackGovernanceCondition)
	require.NotNil(t, cond, "Compliance-SR-2-AIPack condition must be present")
	assert.Equal(t, metav1.ConditionTrue, cond.Status,
		"fully-attested pack should result in True condition")
	assert.Equal(t, "AllAIPacksAttested", cond.Reason)
}

// TestAIPackGovernance_MissingPredicates verifies that a bound AIPack missing
// required predicates results in Compliance-SR-2-AIPack = False.
func TestAIPackGovernance_MissingPredicates(t *testing.T) {
	llmSvc := makeLLMInferenceService(testLLMName, testNamespace)
	// Attestation present but missing PredAIBOM and PredTrainingResidency.
	partialAttestation := &servingv1alpha2.AIPackAttestation{
		Predicates: []servingv1alpha2.PredicateEntry{
			{PredicateURI: servingv1alpha2.PredSLSAProvenance, Required: true},
			{PredicateURI: servingv1alpha2.PredCycloneDXBOM, Required: true},
		},
	}
	pack := makeAIPackWithLabel("llama3", testNamespace, testLLMName,
		servingv1alpha2.KindBaseModel, partialAttestation)

	updated := runGovernanceReconcile(t, testLLMName, llmSvc, pack)

	cond := apimeta.FindStatusCondition(updated.Status.Conditions, aipackGovernanceCondition)
	require.NotNil(t, cond, "Compliance-SR-2-AIPack condition must be present")
	assert.Equal(t, metav1.ConditionFalse, cond.Status,
		"pack with missing predicates should result in False condition")
	assert.Equal(t, "AIPackAttestationIncomplete", cond.Reason)
}

// TestAIPackGovernance_NoAttestationBlock verifies that a bound AIPack with no
// attestation block at all results in Compliance-SR-2-AIPack = False.
func TestAIPackGovernance_NoAttestationBlock(t *testing.T) {
	llmSvc := makeLLMInferenceService(testLLMName, testNamespace)
	pack := makeAIPackWithLabel("llama3", testNamespace, testLLMName,
		servingv1alpha2.KindBaseModel, nil) // no attestation

	updated := runGovernanceReconcile(t, testLLMName, llmSvc, pack)

	cond := apimeta.FindStatusCondition(updated.Status.Conditions, aipackGovernanceCondition)
	require.NotNil(t, cond, "Compliance-SR-2-AIPack condition must be present")
	assert.Equal(t, metav1.ConditionFalse, cond.Status,
		"pack with nil attestation should result in False condition")
}

// TestAIPackGovernance_UnboundPackExcluded verifies that an AIPack without the
// workload label is not included in governance reconciliation.
func TestAIPackGovernance_UnboundPackExcluded(t *testing.T) {
	llmSvc := makeLLMInferenceService(testLLMName, testNamespace)
	// AIPack with no labels — should be excluded.
	unboundPack := &servingv1alpha2.AIPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unbound-pack",
			Namespace: testNamespace,
		},
		Spec: servingv1alpha2.AIPackSpec{
			Kind:        servingv1alpha2.KindBaseModel,
			Attestation: nil,
			Source: servingv1alpha2.AIPackSource{
				Ref: "registry.example.com/models/other@" + modelDigest,
			},
		},
	}

	updated := runGovernanceReconcile(t, testLLMName, llmSvc, unboundPack)

	cond := apimeta.FindStatusCondition(updated.Status.Conditions, aipackGovernanceCondition)
	require.NotNil(t, cond, "Compliance-SR-2-AIPack condition must be present")
	// Unbound pack is excluded → no packs associated.
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "NoAIPacksAssociated", cond.Reason,
		"unbound AIPack must not contribute to governance count")
}

// TestAIPackGovernance_WrongWorkloadLabelExcluded verifies that an AIPack bound
// to a different LLMInferenceService is excluded from this reconcile.
func TestAIPackGovernance_WrongWorkloadLabelExcluded(t *testing.T) {
	llmSvc := makeLLMInferenceService(testLLMName, testNamespace)
	// Pack binds to a different LLM service.
	wrongPack := makeAIPackWithLabel("other-pack", testNamespace, "different-llm",
		servingv1alpha2.KindBaseModel, nil)

	updated := runGovernanceReconcile(t, testLLMName, llmSvc, wrongPack)

	cond := apimeta.FindStatusCondition(updated.Status.Conditions, aipackGovernanceCondition)
	require.NotNil(t, cond)
	assert.Equal(t, "NoAIPacksAssociated", cond.Reason,
		"pack bound to a different LLM must not appear in this service's governance count")
}

// TestAIPackGovernance_MixedAttestationState verifies the partial attestation
// path: multiple packs, some verified and some not.
func TestAIPackGovernance_MixedAttestationState(t *testing.T) {
	llmSvc := makeLLMInferenceService(testLLMName, testNamespace)
	verified := makeAIPackWithLabel("llama3-verified", testNamespace, testLLMName,
		servingv1alpha2.KindBaseModel, makeBaseModelAttestations())
	unverified := makeAIPackWithLabel("llama3-unverified", testNamespace, testLLMName,
		servingv1alpha2.KindBaseModel, nil) // missing attestation

	updated := runGovernanceReconcile(t, testLLMName, llmSvc, verified, unverified)

	cond := apimeta.FindStatusCondition(updated.Status.Conditions, aipackGovernanceCondition)
	require.NotNil(t, cond, "Compliance-SR-2-AIPack condition must be present")
	assert.Equal(t, metav1.ConditionFalse, cond.Status,
		"one unattested pack out of two should result in False condition")
	assert.Equal(t, "AIPackAttestationIncomplete", cond.Reason)
	assert.Contains(t, cond.Message, "1 of 2")
}
