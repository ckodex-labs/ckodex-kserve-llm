package controller

import (
	"context"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidateModelRef_EmptyRef_NotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	agent := makeAgent("a", "")
	ok, msg, err := r.validateModelRef(context.Background(), agent)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "modelRef is required")
}

func TestValidateModelRef_NotFound_NotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	ok, msg, err := r.validateModelRef(context.Background(), makeAgent("a", "missing-model"))
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "not found")
}

func TestValidateModelRef_ModelNotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(notReadyLLMSvc("llama3")).Build(), Scheme: scheme}
	ok, msg, err := r.validateModelRef(context.Background(), makeAgent("a", "llama3"))
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "not ready")
}

func TestValidateModelRef_ModelReady(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyLLMSvc("llama3")).Build(), Scheme: scheme}
	ok, _, err := r.validateModelRef(context.Background(), makeAgent("a", "llama3"))
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestValidateSkillRefs_NoRefs_OK(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	ok, _, err := r.validateSkillRefs(context.Background(), makeAgent("a", "llama3"))
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestValidateSkillRefs_RegistryNotFound(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	agent := makeAgent("a", "llama3", servingv1alpha2.SkillRef{RegistryRef: "missing-registry", SkillName: "retrieval"})
	ok, msg, err := r.validateSkillRefs(context.Background(), agent)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "not found")
}

func TestValidateSkillRefs_SkillNotInRegistry(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(makeSkillReg("tools", "summarize")).Build(), Scheme: scheme}
	agent := makeAgent("a", "llama3", servingv1alpha2.SkillRef{RegistryRef: "tools", SkillName: "nonexistent-skill"})
	ok, msg, err := r.validateSkillRefs(context.Background(), agent)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "not found in SkillRegistry")
}

func TestValidateSkillRefs_AllValid(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(makeSkillReg("tools", "retrieval", "summarize")).Build(), Scheme: scheme}
	agent := makeAgent("a", "llama3", servingv1alpha2.SkillRef{RegistryRef: "tools", SkillName: "retrieval"}, servingv1alpha2.SkillRef{RegistryRef: "tools", SkillName: "summarize"})
	ok, _, err := r.validateSkillRefs(context.Background(), agent)
	require.NoError(t, err)
	assert.True(t, ok)
}
