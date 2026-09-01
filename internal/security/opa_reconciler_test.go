/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ---- scheme helpers --------------------------------------------------------

func TestDefaultOPAConfig_MaxGPUs(t *testing.T) {
	cfg := DefaultOPAConfig()
	assert.Equal(t, int64(8), cfg.MaxGPUsPerNamespace)
}

func TestDefaultOPAConfig_MaxReplicas(t *testing.T) {
	cfg := DefaultOPAConfig()
	assert.Equal(t, int64(16), cfg.MaxReplicasPerService)
}

func TestDefaultOPAConfig_AllowedRegistries_Count(t *testing.T) {
	cfg := DefaultOPAConfig()
	assert.Len(t, cfg.AllowedRegistries, 5)
}

func TestDefaultOPAConfig_AllowedRegistries_Values(t *testing.T) {
	cfg := DefaultOPAConfig()
	assert.Contains(t, cfg.AllowedRegistries, "ghcr.io/ckodex/")
	assert.Contains(t, cfg.AllowedRegistries, "vllm/")
	assert.Contains(t, cfg.AllowedRegistries, "lmsysorg/")
	assert.Contains(t, cfg.AllowedRegistries, "kserve/")
	assert.Contains(t, cfg.AllowedRegistries, "gcr.io/distroless/")
}

// ---- buildConstraintTemplate -----------------------------------------------

func TestBuildConstraintTemplate_APIVersion(t *testing.T) {
	ct := buildConstraintTemplate("testpolicy", "package test\n", nil)
	assert.Equal(t, "templates.gatekeeper.sh/v1", ct.GetAPIVersion())
	assert.Equal(t, "ConstraintTemplate", ct.GetKind())
}

func TestBuildConstraintTemplate_NameAndLabel(t *testing.T) {
	ct := buildConstraintTemplate("llmresourcequota", "package x\n", nil)
	assert.Equal(t, "llmresourcequota", ct.GetName())
	labels := ct.GetLabels()
	assert.Equal(t, "ckodex-kserve-llm-operator", labels["app.kubernetes.io/managed-by"])
}

func TestBuildConstraintTemplate_RegoPresent(t *testing.T) {
	rego := "package mypkg\nviolation[{}] { true }"
	ct := buildConstraintTemplate("mypolicy", rego, nil)

	targets, _, _ := unstructured.NestedSlice(ct.Object, "spec", "targets")
	require.Len(t, targets, 1)
	target := targets[0].(map[string]interface{})
	assert.Equal(t, rego, target["rego"])
}

func TestBuildConstraintTemplate_WithParams_ValidationPresent(t *testing.T) {
	params := []map[string]interface{}{
		{"name": "maxReplicas", "type": "integer"},
	}
	ct := buildConstraintTemplate("quotapolicy", "package q\n", params)

	_, found, err := unstructured.NestedMap(ct.Object, "spec", "crd", "spec", "validation", "openAPIV3Schema", "properties")
	require.NoError(t, err)
	assert.True(t, found, "params should produce openAPIV3Schema properties")
}

func TestBuildConstraintTemplate_NoParams_NoValidation(t *testing.T) {
	ct := buildConstraintTemplate("noparam", "package x\n", nil)

	_, found, _ := unstructured.NestedMap(ct.Object, "spec", "crd", "spec", "validation")
	assert.False(t, found, "nil params must produce no validation section")
}

// ---- buildConstraint -------------------------------------------------------

func TestBuildConstraint_APIVersion(t *testing.T) {
	c := buildConstraint("llmresourcequota", "llm-rq", "default", nil)
	assert.Equal(t, "constraints.gatekeeper.sh/v1beta1", c.GetAPIVersion())
	assert.Equal(t, "llmresourcequota", c.GetKind())
}

func TestBuildConstraint_NamespaceInMatch(t *testing.T) {
	c := buildConstraint("mykind", "my-constraint", "prod", nil)
	namespaces, _, _ := unstructured.NestedSlice(c.Object, "spec", "match", "namespaces")
	require.Len(t, namespaces, 1)
	assert.Equal(t, "prod", namespaces[0])
}

func TestBuildConstraint_KindMatchesAPIGroup(t *testing.T) {
	c := buildConstraint("llmresourcequota", "rq", "default", nil)
	kinds, _, _ := unstructured.NestedSlice(c.Object, "spec", "match", "kinds")
	require.Len(t, kinds, 1)
	k := kinds[0].(map[string]interface{})
	groups := k["apiGroups"].([]interface{})
	assert.Equal(t, "serving.ckodex.com", groups[0])
}

func TestBuildConstraint_WithParams(t *testing.T) {
	c := buildConstraint("mykind", "my-c", "ns", map[string]interface{}{"maxGPUs": int64(8)})
	params, found, _ := unstructured.NestedMap(c.Object, "spec", "parameters")
	require.True(t, found)
	assert.Equal(t, int64(8), params["maxGPUs"])
}

func TestBuildConstraint_NilParams_NoParameters(t *testing.T) {
	c := buildConstraint("mykind", "my-c", "ns", nil)
	_, found, _ := unstructured.NestedMap(c.Object, "spec", "parameters")
	assert.False(t, found, "nil params must produce no parameters field")
}

// ---- buildParamProperties --------------------------------------------------

func TestBuildParamProperties_IntegerType(t *testing.T) {
	params := []map[string]interface{}{
		{"name": "maxReplicas", "type": "integer"},
	}
	props := buildParamProperties(params)
	p := props["maxReplicas"].(map[string]interface{})
	assert.Equal(t, "integer", p["type"])
}

func TestBuildParamProperties_ArrayWithItems(t *testing.T) {
	params := []map[string]interface{}{
		{"name": "allowedRegistries", "type": "array", "items": map[string]interface{}{"type": "string"}},
	}
	props := buildParamProperties(params)
	p := props["allowedRegistries"].(map[string]interface{})
	assert.Equal(t, "array", p["type"])
	assert.NotNil(t, p["items"])
}

func TestBuildParamProperties_MultipleParams(t *testing.T) {
	params := []map[string]interface{}{
		{"name": "a", "type": "integer"},
		{"name": "b", "type": "string"},
	}
	props := buildParamProperties(params)
	assert.Len(t, props, 2)
	assert.Contains(t, props, "a")
	assert.Contains(t, props, "b")
}

// ---- OPAReconciler.ReconcileOPA --------------------------------------------

func TestReconcileOPA_CreatesAllConstraints(t *testing.T) {
	scheme := secScheme(t)
	o := &OPAReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, o.ReconcileOPA(context.Background(), "default", DefaultOPAConfig()))

	// Verify the resource-quota constraint was created (checks applyUnstructured path)
	var c unstructured.Unstructured
	c.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "constraints.gatekeeper.sh",
		Version: "v1beta1",
		Kind:    "llmresourcequota",
	})
	require.NoError(t, o.Get(context.Background(),
		types.NamespacedName{Name: "llm-resource-quota"}, &c))
}

func TestReconcileOPA_ImageAllowlistConstraintCreated(t *testing.T) {
	scheme := secScheme(t)
	o := &OPAReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, o.ReconcileOPA(context.Background(), "prod", DefaultOPAConfig()))

	var c unstructured.Unstructured
	c.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "constraints.gatekeeper.sh",
		Version: "v1beta1",
		Kind:    "llmimageallowlist",
	})
	require.NoError(t, o.Get(context.Background(),
		types.NamespacedName{Name: "llm-image-allowlist"}, &c))
}

func TestReconcileOPA_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	o := &OPAReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, o.ReconcileOPA(context.Background(), "default", DefaultOPAConfig()))
	require.NoError(t, o.ReconcileOPA(context.Background(), "default", DefaultOPAConfig()))
}

// ---- SPIFFEIDForService ----------------------------------------------------
