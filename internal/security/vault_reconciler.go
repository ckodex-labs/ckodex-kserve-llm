/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// VaultConfig defines Vault Agent injector settings.
type VaultConfig struct {
	// Address is the Vault server address.
	Address string `json:"address"`

	// Role is the Kubernetes auth role bound to the operator's service account.
	Role string `json:"role"`

	// SecretPath is the KV v2 path for model secrets (e.g., "secret/data/models").
	SecretPath string `json:"secretPath"`

	// AuthMethod is the Vault auth method ("kubernetes" or "approle").
	AuthMethod string `json:"authMethod"`

	// TransitKeyName is the transit engine key for encryption-at-rest (optional).
	TransitKeyName string `json:"transitKeyName,omitempty"`

	// TLSSkipVerify disables TLS verification (dev only).
	TLSSkipVerify bool `json:"tlsSkipVerify,omitempty"`
}

// DefaultVaultConfig returns production defaults.
func DefaultVaultConfig() VaultConfig {
	return VaultConfig{
		Address:    "http://vault.vault:8200",
		Role:       "ckodex-kserve-llm",
		SecretPath: "secret/data/models",
		AuthMethod: "kubernetes",
	}
}

// VaultReconciler injects Vault Agent sidecar annotations into model server Deployments.
// Secrets are fetched at runtime from Vault's KV engine — never baked into images
// or stored as raw K8s Secrets.
type VaultReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config VaultConfig
}

// ReconcileVault annotates the model server Deployment with Vault Agent injector annotations.
func (v *VaultReconciler) ReconcileVault(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "vault")

	var deploy appsv1.Deployment
	if err := v.Get(ctx, types.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}, &deploy); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Deployment not yet created
		}
		return fmt.Errorf("get deployment for vault: %w", err)
	}

	// Build Vault Agent injector annotations
	annotations := v.buildAnnotations(llmSvc)

	// Merge into pod template annotations
	if deploy.Spec.Template.Annotations == nil {
		deploy.Spec.Template.Annotations = make(map[string]string)
	}

	changed := false
	for k, desired := range annotations {
		if current, ok := deploy.Spec.Template.Annotations[k]; !ok || current != desired {
			deploy.Spec.Template.Annotations[k] = desired
			changed = true
		}
	}

	if !changed {
		return nil
	}

	logger.Info("injecting Vault Agent annotations", "deployment", deploy.Name)
	return v.Update(ctx, &deploy)
}

// buildAnnotations generates the complete set of Vault Agent injector annotations.
func (v *VaultReconciler) buildAnnotations(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	modelSecretPath := fmt.Sprintf("%s/%s/%s", v.Config.SecretPath, llmSvc.Namespace, llmSvc.Name)

	annotations := map[string]string{
		// Core agent inject
		"vault.hashicorp.com/agent-inject":            "true",
		"vault.hashicorp.com/agent-init-first":        "true",
		"vault.hashicorp.com/agent-pre-populate-only": "true",
		"vault.hashicorp.com/role":                    v.Config.Role,
		"vault.hashicorp.com/agent-run-as-user":       "1000",
		"vault.hashicorp.com/agent-run-as-group":      "1000",

		// HuggingFace token
		"vault.hashicorp.com/agent-inject-secret-hf-token": modelSecretPath,
		"vault.hashicorp.com/agent-inject-template-hf-token": fmt.Sprintf(
			`{{- with secret "%s" -}}{{ .Data.data.hf_token }}{{- end -}}`, modelSecretPath),

		// Registry credentials
		"vault.hashicorp.com/agent-inject-secret-registry-creds": modelSecretPath,
		"vault.hashicorp.com/agent-inject-template-registry-creds": fmt.Sprintf(
			`{{- with secret "%s" -}}{{ .Data.data.registry_password }}{{- end -}}`, modelSecretPath),

		// Resource limits for the sidecar
		"vault.hashicorp.com/agent-limits-cpu":   "100m",
		"vault.hashicorp.com/agent-limits-mem":   "64Mi",
		"vault.hashicorp.com/agent-requests-cpu": "50m",
		"vault.hashicorp.com/agent-requests-mem": "32Mi",
	}

	// Transit engine for at-rest encryption
	if v.Config.TransitKeyName != "" {
		annotations["vault.hashicorp.com/agent-inject-secret-transit-key"] = fmt.Sprintf("transit/keys/%s", v.Config.TransitKeyName)
	}

	if v.Config.TLSSkipVerify {
		annotations["vault.hashicorp.com/tls-skip-verify"] = "true"
	}

	return annotations
}
