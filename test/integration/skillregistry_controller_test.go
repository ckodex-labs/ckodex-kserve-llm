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
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// TestSkillRegistry_ValidEntriesReady verifies that a SkillRegistry with
// valid entries becomes Ready and reports the correct EntryCount.
func TestSkillRegistry_ValidEntriesReady(t *testing.T) {
	t.Parallel()
	name := fmt.Sprintf("registry-valid-%d", uniqueID())
	reg := &servingv1alpha2.SkillRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: servingv1alpha2.SkillRegistrySpec{
			Entries: []servingv1alpha2.SkillEntry{
				{Name: "skill-a", Version: "1.0.0", Description: "A skill", Endpoint: "http://skill-a:8080"},
				{Name: "skill-b", Version: "2.0.0", Description: "B skill", Endpoint: "http://skill-b:8080"},
			},
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, reg))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, reg) })

	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var r servingv1alpha2.SkillRegistry
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(reg), &r); err != nil {
				return false, nil
			}
			cond := meta.FindStatusCondition(r.Status.Conditions, "Ready")
			return cond != nil && cond.Status == metav1.ConditionTrue, nil
		},
	))

	var r servingv1alpha2.SkillRegistry
	require.NoError(t, suite.client.Get(suite.ctx, client.ObjectKeyFromObject(reg), &r))
	assert.Equal(t, int32(2), r.Status.EntryCount)
	cond := meta.FindStatusCondition(r.Status.Conditions, "Ready")
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "RegistryReady", cond.Reason)
}

// TestSkillRegistry_DuplicateNameFails verifies that a SkillRegistry with
// duplicate skill names is marked not-Ready with a ValidationFailed reason.
func TestSkillRegistry_DuplicateNameFails(t *testing.T) {
	t.Parallel()
	name := fmt.Sprintf("registry-dup-%d", uniqueID())
	reg := &servingv1alpha2.SkillRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: servingv1alpha2.SkillRegistrySpec{
			Entries: []servingv1alpha2.SkillEntry{
				{Name: "dup-skill", Version: "1.0.0", Description: "A skill", Endpoint: "http://a:8080"},
				{Name: "dup-skill", Version: "1.0.1", Description: "Duplicate", Endpoint: "http://b:8080"},
			},
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, reg))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, reg) })

	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var r servingv1alpha2.SkillRegistry
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(reg), &r); err != nil {
				return false, nil
			}
			return len(r.Status.Conditions) > 0, nil
		},
	))

	var r servingv1alpha2.SkillRegistry
	require.NoError(t, suite.client.Get(suite.ctx, client.ObjectKeyFromObject(reg), &r))
	cond := meta.FindStatusCondition(r.Status.Conditions, "Ready")
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, "ValidationFailed", cond.Reason)
	assert.Equal(t, int32(0), r.Status.EntryCount, "EntryCount must be 0 on validation failure")
}

// TestSkillRegistry_MissingEndpointFails verifies that a skill entry without
// an endpoint causes the registry to fail validation.
func TestSkillRegistry_MissingEndpointFails(t *testing.T) {
	t.Parallel()
	name := fmt.Sprintf("registry-no-ep-%d", uniqueID())
	reg := &servingv1alpha2.SkillRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: servingv1alpha2.SkillRegistrySpec{
			Entries: []servingv1alpha2.SkillEntry{
				{Name: "skill-x", Version: "1.0.0", Description: "Missing endpoint", Endpoint: ""},
			},
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, reg))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, reg) })

	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var r servingv1alpha2.SkillRegistry
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(reg), &r); err != nil {
				return false, nil
			}
			return len(r.Status.Conditions) > 0, nil
		},
	))

	var r servingv1alpha2.SkillRegistry
	require.NoError(t, suite.client.Get(suite.ctx, client.ObjectKeyFromObject(reg), &r))
	cond := meta.FindStatusCondition(r.Status.Conditions, "Ready")
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
}

// TestSkillRegistry_EmptyEntriesReady verifies that a registry with no entries
// is immediately Ready (empty catalog is valid — no rules to break).
func TestSkillRegistry_EmptyEntriesReady(t *testing.T) {
	t.Parallel()
	name := fmt.Sprintf("registry-empty-%d", uniqueID())
	reg := &servingv1alpha2.SkillRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec:       servingv1alpha2.SkillRegistrySpec{Entries: nil},
	}
	require.NoError(t, suite.client.Create(suite.ctx, reg))
	t.Cleanup(func() { _ = suite.client.Delete(suite.ctx, reg) })

	require.NoError(t, wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(context.Context) (bool, error) {
			var r servingv1alpha2.SkillRegistry
			if err := suite.client.Get(suite.ctx, client.ObjectKeyFromObject(reg), &r); err != nil {
				return false, nil
			}
			cond := meta.FindStatusCondition(r.Status.Conditions, "Ready")
			return cond != nil && cond.Status == metav1.ConditionTrue, nil
		},
	))

	var r servingv1alpha2.SkillRegistry
	require.NoError(t, suite.client.Get(suite.ctx, client.ObjectKeyFromObject(reg), &r))
	assert.Equal(t, int32(0), r.Status.EntryCount)
}
