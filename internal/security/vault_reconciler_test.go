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
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ---- scheme helpers --------------------------------------------------------

func TestDefaultVaultConfig_Address(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Equal(t, "http://vault.vault:8200", cfg.Address)
}

func TestDefaultVaultConfig_Role(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Equal(t, "ckodex-kserve-llm", cfg.Role)
}

func TestDefaultVaultConfig_SecretPath(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Equal(t, "secret/data/models", cfg.SecretPath)
}

func TestDefaultVaultConfig_AuthMethod_Kubernetes(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Equal(t, "kubernetes", cfg.AuthMethod)
}

// CRITICAL: TLSSkipVerify must be false by default.
// The annotation "vault.hashicorp.com/tls-skip-verify" must NEVER be injected
// unless the operator is explicitly configured for dev mode.
func TestDefaultVaultConfig_TLSSkipVerify_False(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.False(t, cfg.TLSSkipVerify, "TLSSkipVerify must default to false — disabling TLS is dev-only and must never reach production")
}

// CRITICAL: TransitKeyName must be empty by default.
// The transit annotation must not be injected unless explicitly configured.
func TestDefaultVaultConfig_TransitKeyName_Empty(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Empty(t, cfg.TransitKeyName, "TransitKeyName must default to empty — transit encryption is opt-in")
}

// ---- buildAnnotations — core inject ----------------------------------------

func TestBuildAnnotations_CoreInjectPresent(t *testing.T) {
	v := &VaultReconciler{Config: DefaultVaultConfig()}
	svc := minimalLLMSvc("llama3", "default")

	ann := v.buildAnnotations(svc)

	assert.Equal(t, "true", ann["vault.hashicorp.com/agent-inject"])
	assert.Equal(t, "true", ann["vault.hashicorp.com/agent-init-first"])
	assert.Equal(t, "true", ann["vault.hashicorp.com/agent-pre-populate-only"])
	assert.Equal(t, "ckodex-kserve-llm", ann["vault.hashicorp.com/role"])
	assert.Equal(t, "1000", ann["vault.hashicorp.com/agent-run-as-user"])
	assert.Equal(t, "1000", ann["vault.hashicorp.com/agent-run-as-group"])
}

func TestBuildAnnotations_ResourceLimits(t *testing.T) {
	v := &VaultReconciler{Config: DefaultVaultConfig()}
	ann := v.buildAnnotations(minimalLLMSvc("phi3", "default"))

	assert.Equal(t, "100m", ann["vault.hashicorp.com/agent-limits-cpu"])
	assert.Equal(t, "64Mi", ann["vault.hashicorp.com/agent-limits-mem"])
	assert.Equal(t, "50m", ann["vault.hashicorp.com/agent-requests-cpu"])
	assert.Equal(t, "32Mi", ann["vault.hashicorp.com/agent-requests-mem"])
}

// ---- buildAnnotations — security fallbacks (absence tests) -----------------

// CRITICAL: TLSSkipVerify=false must produce NO tls-skip-verify annotation.
// An accidental "false" string annotation could still alter vault agent behaviour.
func TestBuildAnnotations_TLSSkipVerifyFalse_NoAnnotation(t *testing.T) {
	cfg := DefaultVaultConfig()
	// Default: TLSSkipVerify=false
	v := &VaultReconciler{Config: cfg}

	ann := v.buildAnnotations(minimalLLMSvc("secure", "prod"))

	_, present := ann["vault.hashicorp.com/tls-skip-verify"]
	assert.False(t, present, "tls-skip-verify annotation must be absent when TLSSkipVerify=false")
}

// Verify that when TLSSkipVerify is explicitly enabled the annotation IS injected.
func TestBuildAnnotations_TLSSkipVerifyTrue_AnnotationPresent(t *testing.T) {
	cfg := DefaultVaultConfig()
	cfg.TLSSkipVerify = true
	v := &VaultReconciler{Config: cfg}

	ann := v.buildAnnotations(minimalLLMSvc("dev", "dev"))

	assert.Equal(t, "true", ann["vault.hashicorp.com/tls-skip-verify"])
}

// CRITICAL: Empty TransitKeyName must produce NO transit annotation.
func TestBuildAnnotations_TransitKeyNameEmpty_NoAnnotation(t *testing.T) {
	v := &VaultReconciler{Config: DefaultVaultConfig()}

	ann := v.buildAnnotations(minimalLLMSvc("secure", "prod"))

	_, present := ann["vault.hashicorp.com/agent-inject-secret-transit-key"]
	assert.False(t, present, "transit-key annotation must be absent when TransitKeyName is empty")
}

func TestBuildAnnotations_TransitKeyNameSet_AnnotationPresent(t *testing.T) {
	cfg := DefaultVaultConfig()
	cfg.TransitKeyName = "model-encryption-key"
	v := &VaultReconciler{Config: cfg}

	ann := v.buildAnnotations(minimalLLMSvc("encrypted", "prod"))

	val, present := ann["vault.hashicorp.com/agent-inject-secret-transit-key"]
	require.True(t, present, "transit annotation must be present when TransitKeyName is set")
	assert.Equal(t, "transit/keys/model-encryption-key", val)
}

func TestBuildAnnotations_SecretPathContainsNamespaceAndName(t *testing.T) {
	v := &VaultReconciler{Config: DefaultVaultConfig()}
	svc := minimalLLMSvc("phi3", "staging")

	ann := v.buildAnnotations(svc)

	// Model-scoped path must include namespace and name for isolation.
	hfSecret := ann["vault.hashicorp.com/agent-inject-secret-hf-token"]
	assert.Contains(t, hfSecret, "staging")
	assert.Contains(t, hfSecret, "phi3")
}

// ---- ReconcileVault --------------------------------------------------------

func TestReconcileVault_DeploymentNotFound_NoError(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	v := &VaultReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
		Config: DefaultVaultConfig(),
	}

	// Deployment does not exist — must return nil (no-op, deployment not yet created)
	require.NoError(t, v.ReconcileVault(context.Background(), svc))
}

func TestReconcileVault_AnnotatesDeployment(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "llama3", Namespace: "default"},
	}

	v := &VaultReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, dep).Build(),
		Scheme: scheme,
		Config: DefaultVaultConfig(),
	}

	require.NoError(t, v.ReconcileVault(context.Background(), svc))

	var updated appsv1.Deployment
	require.NoError(t, v.Get(context.Background(),
		types.NamespacedName{Name: "llama3", Namespace: "default"}, &updated))

	ann := updated.Spec.Template.Annotations
	require.NotNil(t, ann)
	assert.Equal(t, "true", ann["vault.hashicorp.com/agent-inject"])
}

func TestReconcileVault_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "llama3", Namespace: "default"},
	}

	v := &VaultReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, dep).Build(),
		Scheme: scheme,
		Config: DefaultVaultConfig(),
	}

	// Apply twice — both calls must succeed without error
	require.NoError(t, v.ReconcileVault(context.Background(), svc))
	require.NoError(t, v.ReconcileVault(context.Background(), svc))
}

// ---- DefaultOPAConfig ------------------------------------------------------
