/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// newAIPackScheme returns a scheme with the serving v1alpha2 types registered.
func newAIPackScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	return s
}

// newAIPack constructs a minimal AIPack for testing.
func newAIPack(name, ns string, kind servingv1alpha2.ArtifactKind) *servingv1alpha2.AIPack {
	return &servingv1alpha2.AIPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: servingv1alpha2.AIPackSpec{
			Kind: kind,
			Source: servingv1alpha2.AIPackSource{
				Ref: "registry.example.com/models/llama3@sha256:" +
					"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
	}
}

// reconcileAIPack is a test helper that builds a fake client, runs one reconcile, and
// returns the updated AIPack object together with any error.
func reconcileAIPack(
	t *testing.T,
	pack *servingv1alpha2.AIPack,
) (*servingv1alpha2.AIPack, error) {
	t.Helper()
	scheme := newAIPackScheme(t)
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pack).
		WithStatusSubresource(&servingv1alpha2.AIPack{}).
		Build()
	r := &AIPackReconciler{
		Client: cl,
		Scheme: scheme,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: pack.Namespace, Name: pack.Name},
	})

	var updated servingv1alpha2.AIPack
	getErr := cl.Get(context.Background(),
		types.NamespacedName{Namespace: pack.Namespace, Name: pack.Name},
		&updated,
	)
	require.NoError(t, getErr)
	return &updated, err
}

// TestAIPackReconciler_ValidKind_BaseModel verifies that a BaseModel AIPack gets
// status.family = "model" and a Ready=True condition.
func TestAIPackReconciler_ValidKind_BaseModel(t *testing.T) {
	pack := newAIPack("llama3-base", "default", servingv1alpha2.KindBaseModel)
	updated, err := reconcileAIPack(t, pack)
	require.NoError(t, err)

	assert.Equal(t, servingv1alpha2.FamilyModel, updated.Status.Family,
		"status.family should be 'model' for KindBaseModel")

	cond := apimeta.FindStatusCondition(updated.Status.Conditions,
		string(servingv1alpha2.AIPackConditionReady))
	require.NotNil(t, cond, "Ready condition must be present")
	assert.Equal(t, metav1.ConditionTrue, cond.Status, "Ready condition should be True")
	assert.Equal(t, "ArtifactRegistered", cond.Reason)
}

// TestAIPackReconciler_AllKinds verifies that every valid ArtifactKind is reconciled
// to the correct status.family and a Ready=True condition.
func TestAIPackReconciler_AllKinds(t *testing.T) {
	cases := []struct {
		kind           servingv1alpha2.ArtifactKind
		expectedFamily servingv1alpha2.ArtifactFamily
	}{
		{servingv1alpha2.KindBaseModel, servingv1alpha2.FamilyModel},
		{servingv1alpha2.KindLoRA, servingv1alpha2.FamilyModel},
		{servingv1alpha2.KindFineTune, servingv1alpha2.FamilyModel},
		{servingv1alpha2.KindSkill, servingv1alpha2.FamilyCapability},
		{servingv1alpha2.KindTool, servingv1alpha2.FamilyCapability},
		{servingv1alpha2.KindMCPServer, servingv1alpha2.FamilyCapability},
		{servingv1alpha2.KindWorkflow, servingv1alpha2.FamilyCapability},
		{servingv1alpha2.KindPromptTemplate, servingv1alpha2.FamilyControl},
		{servingv1alpha2.KindGuardrail, servingv1alpha2.FamilyControl},
		{servingv1alpha2.KindPolicyBundle, servingv1alpha2.FamilyControl},
		{servingv1alpha2.KindRetrievalIndex, servingv1alpha2.FamilyKnowledge},
		{servingv1alpha2.KindDataset, servingv1alpha2.FamilyKnowledge},
		{servingv1alpha2.KindHarness, servingv1alpha2.FamilyAssurance},
		{servingv1alpha2.KindEval, servingv1alpha2.FamilyAssurance},
		{servingv1alpha2.KindAgent, servingv1alpha2.FamilyComposite},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.kind), func(t *testing.T) {
			pack := newAIPack("test-"+string(tc.kind), "default", tc.kind)
			updated, err := reconcileAIPack(t, pack)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedFamily, updated.Status.Family,
				"status.family mismatch for kind %s", tc.kind)

			cond := apimeta.FindStatusCondition(updated.Status.Conditions,
				string(servingv1alpha2.AIPackConditionReady))
			require.NotNil(t, cond, "Ready condition must be present for kind %s", tc.kind)
			assert.Equal(t, metav1.ConditionTrue, cond.Status,
				"Ready condition should be True for kind %s", tc.kind)
			assert.Equal(t, "ArtifactRegistered", cond.Reason)
		})
	}
}

// TestAIPackReconciler_UnknownKind verifies that an AIPack with an unknown kind
// gets a Ready=False condition with reason InvalidKind.
func TestAIPackReconciler_UnknownKind(t *testing.T) {
	pack := newAIPack("mystery-pack", "default", servingv1alpha2.ArtifactKind("NotARealKind"))
	updated, err := reconcileAIPack(t, pack)
	require.NoError(t, err, "reconcile should not return an error for unknown kind")

	cond := apimeta.FindStatusCondition(updated.Status.Conditions,
		string(servingv1alpha2.AIPackConditionReady))
	require.NotNil(t, cond, "Ready condition must be present")
	assert.Equal(t, metav1.ConditionFalse, cond.Status, "Ready condition should be False for unknown kind")
	assert.Equal(t, "InvalidKind", cond.Reason)
}
