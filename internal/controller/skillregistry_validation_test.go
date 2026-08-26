package controller

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEntries_Empty_OK(t *testing.T) {
	assert.NoError(t, (&SkillRegistryReconciler{}).validateEntries(&servingv1alpha2.SkillRegistry{}))
}

func TestValidateEntries_ValidEntries_OK(t *testing.T) {
	reg := skillReg("registry", validEntry("retrieval"), validEntry("summarize"))
	assert.NoError(t, (&SkillRegistryReconciler{}).validateEntries(reg))
}

func TestValidateEntries_EmptyName_Error(t *testing.T) {
	err := (&SkillRegistryReconciler{}).validateEntries(skillReg("registry", servingv1alpha2.SkillEntry{Version: "1.0.0", Endpoint: "http://foo:8080"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateEntries_EmptyEndpoint_Error(t *testing.T) {
	err := (&SkillRegistryReconciler{}).validateEntries(skillReg("registry", servingv1alpha2.SkillEntry{Name: "retrieval", Version: "1.0.0"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint is required")
}

func TestValidateEntries_EmptyVersion_Error(t *testing.T) {
	err := (&SkillRegistryReconciler{}).validateEntries(skillReg("registry", servingv1alpha2.SkillEntry{Name: "retrieval", Endpoint: "http://foo:8080"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version is required")
}

func TestValidateEntries_DuplicateName_Error(t *testing.T) {
	err := (&SkillRegistryReconciler{}).validateEntries(skillReg("registry", validEntry("retrieval"), validEntry("retrieval")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate skill name")
	assert.Contains(t, err.Error(), "retrieval")
}
