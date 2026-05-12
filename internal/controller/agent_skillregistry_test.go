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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

// newControllerScheme builds a scheme with all serving types registered.
func newControllerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	return s
}

// ---- SkillRegistryReconciler.validateEntries ----------------------------

func TestValidateEntries_Empty_OK(t *testing.T) {
	r := &SkillRegistryReconciler{}
	reg := &servingv1alpha2.SkillRegistry{
		Spec: servingv1alpha2.SkillRegistrySpec{Entries: nil},
	}
	assert.NoError(t, r.validateEntries(reg))
}

func TestValidateEntries_ValidEntries_OK(t *testing.T) {
	r := &SkillRegistryReconciler{}
	reg := &servingv1alpha2.SkillRegistry{
		Spec: servingv1alpha2.SkillRegistrySpec{
			Entries: []servingv1alpha2.SkillEntry{
				{Name: "retrieval", Version: "1.0.0", Endpoint: "http://retrieval:8080", Description: "RAG retrieval"},
				{Name: "summarize", Version: "2.0.0", Endpoint: "http://summarize:8080", Description: "Text summarization"},
			},
		},
	}
	assert.NoError(t, r.validateEntries(reg))
}

func TestValidateEntries_EmptyName_Error(t *testing.T) {
	r := &SkillRegistryReconciler{}
	reg := &servingv1alpha2.SkillRegistry{
		Spec: servingv1alpha2.SkillRegistrySpec{
			Entries: []servingv1alpha2.SkillEntry{
				{Name: "", Version: "1.0.0", Endpoint: "http://foo:8080"},
			},
		},
	}
	err := r.validateEntries(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateEntries_EmptyEndpoint_Error(t *testing.T) {
	r := &SkillRegistryReconciler{}
	reg := &servingv1alpha2.SkillRegistry{
		Spec: servingv1alpha2.SkillRegistrySpec{
			Entries: []servingv1alpha2.SkillEntry{
				{Name: "retrieval", Version: "1.0.0", Endpoint: ""},
			},
		},
	}
	err := r.validateEntries(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint is required")
}

func TestValidateEntries_EmptyVersion_Error(t *testing.T) {
	r := &SkillRegistryReconciler{}
	reg := &servingv1alpha2.SkillRegistry{
		Spec: servingv1alpha2.SkillRegistrySpec{
			Entries: []servingv1alpha2.SkillEntry{
				{Name: "retrieval", Version: "", Endpoint: "http://foo:8080"},
			},
		},
	}
	err := r.validateEntries(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version is required")
}

func TestValidateEntries_DuplicateName_Error(t *testing.T) {
	r := &SkillRegistryReconciler{}
	reg := &servingv1alpha2.SkillRegistry{
		Spec: servingv1alpha2.SkillRegistrySpec{
			Entries: []servingv1alpha2.SkillEntry{
				{Name: "retrieval", Version: "1.0.0", Endpoint: "http://foo:8080"},
				{Name: "retrieval", Version: "2.0.0", Endpoint: "http://bar:8080"},
			},
		},
	}
	err := r.validateEntries(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate skill name")
	assert.Contains(t, err.Error(), "retrieval")
}

// ---- SkillRegistryReconciler.Reconcile ----------------------------------

func skillReg(name string, entries ...servingv1alpha2.SkillEntry) *servingv1alpha2.SkillRegistry {
	return &servingv1alpha2.SkillRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       servingv1alpha2.SkillRegistrySpec{Entries: entries},
	}
}

func validEntry(name string) servingv1alpha2.SkillEntry {
	return servingv1alpha2.SkillEntry{
		Name:        name,
		Version:     "1.0.0",
		Endpoint:    "http://" + name + ":8080",
		Description: name,
	}
}

func TestSkillRegistryReconcile_ValidRegistry_ReadyTrue(t *testing.T) {
	scheme := newControllerScheme(t)
	reg := skillReg("my-registry", validEntry("skill-a"), validEntry("skill-b"))

	r := &SkillRegistryReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(reg).WithStatusSubresource(reg).Build(),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "my-registry", Namespace: "default"},
	})
	require.NoError(t, err)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "my-registry", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	var updated servingv1alpha2.SkillRegistry
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "my-registry", Namespace: "default"}, &updated))

	// Finalizer should be added.
	assert.Contains(t, updated.Finalizers, api.FinalizerName)

	// Status via status subresource.
	assert.Equal(t, int32(2), updated.Status.EntryCount)
}

func TestSkillRegistryReconcile_InvalidEntry_ReadyFalse(t *testing.T) {
	scheme := newControllerScheme(t)
	bad := servingv1alpha2.SkillEntry{Name: "bad", Version: "", Endpoint: "http://bad:8080"}
	reg := skillReg("bad-registry", bad)

	r := &SkillRegistryReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(reg).WithStatusSubresource(reg).Build(),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bad-registry", Namespace: "default"},
	})
	require.NoError(t, err)

	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bad-registry", Namespace: "default"},
	})
	require.NoError(t, err, "reconcile itself should not error — it patches status")

	var updated servingv1alpha2.SkillRegistry
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "bad-registry", Namespace: "default"}, &updated))

	assert.Equal(t, int32(0), updated.Status.EntryCount)
}

func TestSkillRegistryReconcile_NotFound_NoError(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &SkillRegistryReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "gone", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// ---- AgentReconciler helpers: validateModelRef / validateSkillRefs ------

func readyLLMSvc(name string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status:     servingv1alpha2.LLMInferenceServiceStatus{ModelReady: true},
	}
}

func notReadyLLMSvc(name string) *servingv1alpha2.LLMInferenceService {
	svc := readyLLMSvc(name)
	svc.Status.ModelReady = false
	return svc
}

func TestValidateModelRef_EmptyRef_NotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Spec:       servingv1alpha2.AgentConfiguration{ModelRef: ""},
	}
	ok, msg, err := r.validateModelRef(context.Background(), agent)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "modelRef is required")
}

func TestValidateModelRef_NotFound_NotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Spec:       servingv1alpha2.AgentConfiguration{ModelRef: "missing-model"},
	}
	ok, msg, err := r.validateModelRef(context.Background(), agent)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "not found")
}

func TestValidateModelRef_ModelNotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	llmSvc := notReadyLLMSvc("llama3")
	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build(),
		Scheme: scheme,
	}
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Spec:       servingv1alpha2.AgentConfiguration{ModelRef: "llama3"},
	}
	ok, msg, err := r.validateModelRef(context.Background(), agent)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "not ready")
}

func TestValidateModelRef_ModelReady(t *testing.T) {
	scheme := newControllerScheme(t)
	llmSvc := readyLLMSvc("llama3")
	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build(),
		Scheme: scheme,
	}
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Spec:       servingv1alpha2.AgentConfiguration{ModelRef: "llama3"},
	}
	ok, _, err := r.validateModelRef(context.Background(), agent)
	require.NoError(t, err)
	assert.True(t, ok)
}

// ---- validateSkillRefs ---------------------------------------------------

func makeSkillReg(name string, skills ...string) *servingv1alpha2.SkillRegistry {
	entries := make([]servingv1alpha2.SkillEntry, 0, len(skills))
	for _, s := range skills {
		entries = append(entries, validEntry(s))
	}
	return &servingv1alpha2.SkillRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       servingv1alpha2.SkillRegistrySpec{Entries: entries},
	}
}

func TestValidateSkillRefs_NoRefs_OK(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Spec:       servingv1alpha2.AgentConfiguration{Skills: nil},
	}
	ok, _, err := r.validateSkillRefs(context.Background(), agent)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestValidateSkillRefs_RegistryNotFound(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Spec: servingv1alpha2.AgentConfiguration{
			Skills: []servingv1alpha2.SkillRef{
				{RegistryRef: "missing-registry", SkillName: "retrieval"},
			},
		},
	}
	ok, msg, err := r.validateSkillRefs(context.Background(), agent)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "not found")
}

func TestValidateSkillRefs_SkillNotInRegistry(t *testing.T) {
	scheme := newControllerScheme(t)
	reg := makeSkillReg("tools", "summarize")
	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(reg).Build(),
		Scheme: scheme,
	}
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Spec: servingv1alpha2.AgentConfiguration{
			Skills: []servingv1alpha2.SkillRef{
				{RegistryRef: "tools", SkillName: "nonexistent-skill"},
			},
		},
	}
	ok, msg, err := r.validateSkillRefs(context.Background(), agent)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, msg, "not found in SkillRegistry")
}

func TestValidateSkillRefs_AllValid(t *testing.T) {
	scheme := newControllerScheme(t)
	reg := makeSkillReg("tools", "retrieval", "summarize")
	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(reg).Build(),
		Scheme: scheme,
	}
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Spec: servingv1alpha2.AgentConfiguration{
			Skills: []servingv1alpha2.SkillRef{
				{RegistryRef: "tools", SkillName: "retrieval"},
				{RegistryRef: "tools", SkillName: "summarize"},
			},
		},
	}
	ok, _, err := r.validateSkillRefs(context.Background(), agent)
	require.NoError(t, err)
	assert.True(t, ok)
}

// ---- AgentReconciler.Reconcile ------------------------------------------

func makeAgent(name, modelRef string, skills ...servingv1alpha2.SkillRef) *servingv1alpha2.Agent {
	return &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: servingv1alpha2.AgentConfiguration{
			Identity: servingv1alpha2.AgentIdentity{Name: name},
			ModelRef: modelRef,
			Skills:   skills,
		},
	}
}

func TestAgentReconcile_NotFound_NoError(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "gone", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestAgentReconcile_AddsFinalizer(t *testing.T) {
	scheme := newControllerScheme(t)
	llmSvc := readyLLMSvc("llama3")
	agent := makeAgent("bot", "llama3")

	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(llmSvc, agent).WithStatusSubresource(agent).Build(),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.Agent
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "bot", Namespace: "default"}, &updated))
	assert.Contains(t, updated.Finalizers, api.FinalizerName)
}

func TestAgentReconcile_ModelReady_AgentReady(t *testing.T) {
	scheme := newControllerScheme(t)
	llmSvc := readyLLMSvc("llama3")
	agent := makeAgent("bot", "llama3")

	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(llmSvc, agent).WithStatusSubresource(agent).Build(),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"},
	})
	require.NoError(t, err)

	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.Agent
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "bot", Namespace: "default"}, &updated))
	assert.True(t, updated.Status.Ready)
}

func TestAgentReconcile_ModelNotReady_AgentNotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	llmSvc := notReadyLLMSvc("llama3")
	agent := makeAgent("bot", "llama3")

	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(llmSvc, agent).WithStatusSubresource(agent).Build(),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.Agent
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "bot", Namespace: "default"}, &updated))
	assert.False(t, updated.Status.Ready)
}

func TestAgentReconcile_ModelMissing_AgentNotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	agent := makeAgent("bot", "nonexistent")

	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(agent).WithStatusSubresource(agent).Build(),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.Agent
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "bot", Namespace: "default"}, &updated))
	assert.False(t, updated.Status.Ready)
}

func TestAgentReconcile_SkillRegistryMissing_AgentNotReady(t *testing.T) {
	scheme := newControllerScheme(t)
	llmSvc := readyLLMSvc("llama3")
	agent := makeAgent("bot", "llama3",
		servingv1alpha2.SkillRef{RegistryRef: "missing-registry", SkillName: "retrieval"},
	)

	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(llmSvc, agent).WithStatusSubresource(agent).Build(),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.Agent
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "bot", Namespace: "default"}, &updated))
	assert.False(t, updated.Status.Ready)
}

func TestAgentReconcile_AllValid_Ready(t *testing.T) {
	scheme := newControllerScheme(t)
	llmSvc := readyLLMSvc("llama3")
	reg := makeSkillReg("tools", "retrieval", "summarize")
	agent := makeAgent("bot", "llama3",
		servingv1alpha2.SkillRef{RegistryRef: "tools", SkillName: "retrieval"},
		servingv1alpha2.SkillRef{RegistryRef: "tools", SkillName: "summarize"},
	)

	r := &AgentReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(llmSvc, reg, agent).WithStatusSubresource(agent).Build(),
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"},
	})
	require.NoError(t, err)

	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bot", Namespace: "default"},
	})
	require.NoError(t, err)

	var updated servingv1alpha2.Agent
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "bot", Namespace: "default"}, &updated))
	assert.True(t, updated.Status.Ready)
}

// ---- boolToConditionStatus ----------------------------------------------

func TestBoolToConditionStatus(t *testing.T) {
	assert.Equal(t, metav1.ConditionTrue, boolToConditionStatus(true))
	assert.Equal(t, metav1.ConditionFalse, boolToConditionStatus(false))
}

// TestAgentReconcile_Deletion_RemovesFinalizer verifies finalizer cleanup.
func TestAgentReconcile_Deletion_RemovesFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	now := metav1.Now()
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "del-agent",
			Namespace:         "default",
			Finalizers:        []string{api.FinalizerName},
			DeletionTimestamp: &now,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(agent).Build()
	r := &AgentReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "del-agent", Namespace: "default"},
	})
	require.NoError(t, err)

	updated := &servingv1alpha2.Agent{}
	err = cl.Get(context.Background(), types.NamespacedName{Name: "del-agent", Namespace: "default"}, updated)
	if apierrors.IsNotFound(err) {
		return
	}
	require.NoError(t, err)
	assert.NotContains(t, updated.Finalizers, api.FinalizerName)
}
