/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// TestAgentController_ReadyWhenModelReady verifies that when the referenced
// LLMInferenceService is Ready, the Agent transitions to Ready=true.
func TestAgentController_ReadyWhenModelReady(t *testing.T) {
	t.Parallel()
	id := uniqueID()
	modelName := fmt.Sprintf("llm-for-agent-ready-%d", id)
	agentName := fmt.Sprintf("agent-ready-test-%d", id)
	newLLMInferenceService(t, modelName)

	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName,
			Namespace: testNamespace,
		},
		Spec: servingv1alpha2.AgentConfiguration{
			Identity: servingv1alpha2.AgentIdentity{Name: "test-agent"},
			ModelRef: modelName,
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, agent))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, agent) })

	err := wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var a servingv1alpha2.Agent
			if err := suite.client.Get(suite.ctx, types.NamespacedName{
				Name: agentName, Namespace: testNamespace,
			}, &a); err != nil {
				return false, nil
			}
			return a.Status.Ready, nil
		},
	)
	require.NoError(t, err, "agent should become Ready")
}

// TestAgentController_NotReadyWhenModelMissing verifies that when the referenced
// LLMInferenceService does not exist, the Agent stays not-ready with
// a ModelNotReady condition reason.
func TestAgentController_NotReadyWhenModelMissing(t *testing.T) {
	t.Parallel()
	agentName := fmt.Sprintf("agent-missing-model-%d", uniqueID())
	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName,
			Namespace: testNamespace,
		},
		Spec: servingv1alpha2.AgentConfiguration{
			Identity: servingv1alpha2.AgentIdentity{Name: "test-agent"},
			ModelRef: "does-not-exist-llm",
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, agent))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, agent) })

	// Allow the reconciler at least one pass.
	err := wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var a servingv1alpha2.Agent
			if err := suite.client.Get(suite.ctx, types.NamespacedName{
				Name: agentName, Namespace: testNamespace,
			}, &a); err != nil {
				return false, nil
			}
			// Wait until at least one condition is set (reconciler ran).
			return len(a.Status.Conditions) > 0, nil
		},
	)
	require.NoError(t, err)

	var a servingv1alpha2.Agent
	require.NoError(t, suite.client.Get(suite.ctx, types.NamespacedName{
		Name: agentName, Namespace: testNamespace,
	}, &a))
	assert.False(t, a.Status.Ready, "agent should not be Ready when model is missing")
	cond := meta.FindStatusCondition(a.Status.Conditions, "Ready")
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
}

// TestAgentController_NotReadyWhenSkillMissing verifies that the Agent is not
// Ready when a referenced SkillRegistry does not exist.
func TestAgentController_NotReadyWhenSkillMissing(t *testing.T) {
	t.Parallel()
	modelName := fmt.Sprintf("llm-for-agent-skill-%d", uniqueID())
	agentName := fmt.Sprintf("agent-skill-missing-%d", uniqueID())
	newLLMInferenceService(t, modelName)

	agent := &servingv1alpha2.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName,
			Namespace: testNamespace,
		},
		Spec: servingv1alpha2.AgentConfiguration{
			Identity: servingv1alpha2.AgentIdentity{Name: "test-agent"},
			ModelRef: modelName,
			Skills: []servingv1alpha2.SkillRef{
				{RegistryRef: "nonexistent-registry", SkillName: "my-skill"},
			},
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, agent))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, agent) })

	err := wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var a servingv1alpha2.Agent
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(agent), &a); err != nil {
				return false, nil
			}
			return len(a.Status.Conditions) > 0, nil
		},
	)
	require.NoError(t, err)

	var a servingv1alpha2.Agent
	require.NoError(t, suite.client.Get(suite.ctx, client.ObjectKeyFromObject(agent), &a))
	assert.False(t, a.Status.Ready)
}
