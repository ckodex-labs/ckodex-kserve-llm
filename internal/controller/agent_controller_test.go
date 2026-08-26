package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

func TestAgentReconcile_NotFound_NoError(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "gone", Namespace: "default"}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestAgentReconcile_AddsFinalizer(t *testing.T) {
	scheme := newControllerScheme(t)
	agent := makeAgent("bot", "llama3")
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyLLMSvc("llama3"), agent).WithStatusSubresource(agent).Build(), Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"}})
	require.NoError(t, err)
	updated := &servingv1alpha2.Agent{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "bot", Namespace: "default"}, updated))
	assert.Contains(t, updated.Finalizers, api.FinalizerName)
}

func TestAgentReconcile_ModelReady_AgentReady(t *testing.T) {
	scheme := newControllerScheme(t)
	agent := makeAgent("bot", "llama3")
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyLLMSvc("llama3"), agent).WithStatusSubresource(agent).Build(), Scheme: scheme}
	reconcileAgent(t, r, "bot")
	updated := &servingv1alpha2.Agent{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "bot", Namespace: "default"}, updated))
	assert.True(t, updated.Status.Ready)
}

func TestAgentReconcile_ModelNotReady_AgentNotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	agent := makeAgent("bot", "llama3")
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(notReadyLLMSvc("llama3"), agent).WithStatusSubresource(agent).Build(), Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"}})
	require.NoError(t, err)
	updated := &servingv1alpha2.Agent{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "bot", Namespace: "default"}, updated))
	assert.False(t, updated.Status.Ready)
}

func TestAgentReconcile_ModelMissing_AgentNotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	agent := makeAgent("bot", "nonexistent")
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(agent).WithStatusSubresource(agent).Build(), Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"}})
	require.NoError(t, err)
	updated := &servingv1alpha2.Agent{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "bot", Namespace: "default"}, updated))
	assert.False(t, updated.Status.Ready)
}

func TestAgentReconcile_SkillRegistryMissing_AgentNotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	agent := makeAgent("bot", "llama3", servingv1alpha2.SkillRef{RegistryRef: "missing-registry", SkillName: "retrieval"})
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyLLMSvc("llama3"), agent).WithStatusSubresource(agent).Build(), Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"}})
	require.NoError(t, err)
	updated := &servingv1alpha2.Agent{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "bot", Namespace: "default"}, updated))
	assert.False(t, updated.Status.Ready)
}

func TestAgentReconcile_AllValid_Ready(t *testing.T) {
	scheme := newControllerScheme(t)
	agent := makeAgent("bot", "llama3", servingv1alpha2.SkillRef{RegistryRef: "tools", SkillName: "retrieval"}, servingv1alpha2.SkillRef{RegistryRef: "tools", SkillName: "summarize"})
	r := &AgentReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyLLMSvc("llama3"), makeSkillReg("tools", "retrieval", "summarize"), agent).WithStatusSubresource(agent).Build(), Scheme: scheme}
	reconcileAgent(t, r, "bot")
	updated := &servingv1alpha2.Agent{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "bot", Namespace: "default"}, updated))
	assert.True(t, updated.Status.Ready)
}

func TestAgentReconcile_Deletion_RemovesFinalizer(t *testing.T) {
	scheme := newControllerScheme(t)
	require.NoError(t, servingv1alpha2.AddToScheme(scheme))
	now := metav1.Now()
	agent := &servingv1alpha2.Agent{ObjectMeta: metav1.ObjectMeta{Name: "del-agent", Namespace: "default", Finalizers: []string{api.FinalizerName}, DeletionTimestamp: &now}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(agent).Build()
	r := &AgentReconciler{Client: cl, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "del-agent", Namespace: "default"}})
	require.NoError(t, err)
	updated := &servingv1alpha2.Agent{}
	err = cl.Get(context.Background(), types.NamespacedName{Name: "del-agent", Namespace: "default"}, updated)
	if apierrors.IsNotFound(err) {
		return
	}
	require.NoError(t, err)
	assert.NotContains(t, updated.Finalizers, api.FinalizerName)
}

func reconcileAgent(t *testing.T, r *AgentReconciler, name string) {
	t.Helper()
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: "default"}})
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: "default"}})
	require.NoError(t, err)
}
