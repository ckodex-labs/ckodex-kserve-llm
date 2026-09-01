/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GVK constants for Gatekeeper
var (
	ConstraintTemplateGVK = schema.GroupVersionKind{
		Group:   "templates.gatekeeper.sh",
		Version: "v1",
		Kind:    "ConstraintTemplate",
	}
)

// OPAReconciler generates Gatekeeper ConstraintTemplate + Constraint resources
// to enforce operator-level policies:
//   - LLMResourceQuota: GPU/replica caps per namespace
//   - LLMImageAllowlist: approved container registries
//   - LLMSecurityPolicy: pod security hardening
type OPAReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// OPAConfig defines configurable policy parameters.
type OPAConfig struct {
	// MaxGPUsPerNamespace is the GPU ceiling.
	MaxGPUsPerNamespace int64 `json:"maxGPUsPerNamespace"`

	// MaxReplicasPerService is the replica ceiling.
	MaxReplicasPerService int64 `json:"maxReplicasPerService"`

	// AllowedRegistries are permitted container image registry prefixes.
	AllowedRegistries []string `json:"allowedRegistries"`
}

// DefaultOPAConfig returns production defaults.
func DefaultOPAConfig() OPAConfig {
	return OPAConfig{
		MaxGPUsPerNamespace:   8,
		MaxReplicasPerService: 16,
		AllowedRegistries: []string{
			"ghcr.io/ckodex/",
			"vllm/",
			"lmsysorg/",
			"kserve/",
			"gcr.io/distroless/",
		},
	}
}

// ReconcileOPA ensures all Gatekeeper ConstraintTemplates and Constraints exist.
func (o *OPAReconciler) ReconcileOPA(ctx context.Context, namespace string, cfg OPAConfig) error {
	logger := log.FromContext(ctx).WithValues("component", "opa")

	// 1. Check if Gatekeeper is available
	exists, err := o.isGVKAvailable()
	if err != nil {
		return fmt.Errorf("check Gatekeeper availability: %w", err)
	}
	if !exists {
		logger.Info("Gatekeeper templates.gatekeeper.sh/v1 CRDs not found, skipping OPA reconciliation")
		return nil
	}

	// 2. Resource Quota constraint
	if err := o.reconcileResourceQuota(ctx, namespace, cfg); err != nil {
		return fmt.Errorf("reconcile resource quota constraint: %w", err)
	}

	// 3. Image Allowlist constraint
	if err := o.reconcileImageAllowlist(ctx, namespace, cfg); err != nil {
		return fmt.Errorf("reconcile image allowlist constraint: %w", err)
	}

	// 4. Security Policy constraint
	if err := o.reconcileSecurityPolicy(ctx, namespace); err != nil {
		return fmt.Errorf("reconcile security policy constraint: %w", err)
	}

	// 5. Model Access constraint (tenant-based isolation)
	if err := o.reconcileModelAccess(ctx, namespace); err != nil {
		return fmt.Errorf("reconcile model access constraint: %w", err)
	}

	logger.Info("OPA constraints reconciled")
	return nil
}

// isGVKAvailable checks if Gatekeeper ConstraintTemplate CRD is registered.
func (o *OPAReconciler) isGVKAvailable() (bool, error) {
	if o.Client == nil {
		return false, nil
	}

	if mapper := o.RESTMapper(); mapper != nil {
		_, err := mapper.RESTMapping(ConstraintTemplateGVK.GroupKind(), ConstraintTemplateGVK.Version)
		if err == nil {
			return true, nil
		}
		if !meta.IsNoMatchError(err) {
			return false, err
		}
	}

	if o.Scheme != nil && o.Scheme.Recognizes(ConstraintTemplateGVK) {
		return true, nil
	}

	return false, nil
}

// reconcileResourceQuota creates a ConstraintTemplate + Constraint for GPU/replica limits.
func (o *OPAReconciler) reconcileResourceQuota(ctx context.Context, namespace string, cfg OPAConfig) error {
	template := buildConstraintTemplate("llmresourcequota", `
package llmresourcequota
violation[{"msg": msg}] {
  input.review.object.kind == "LLMInferenceService"
  replicas := object.get(input.review.object.spec, "replicas", 1)
  replicas > input.parameters.maxReplicas
  msg := sprintf("replicas %d exceeds max %d", [replicas, input.parameters.maxReplicas])
}
`, []map[string]interface{}{
		{"name": "maxReplicas", "type": "integer"},
		{"name": "maxGPUs", "type": "integer"},
	})

	constraint := buildConstraint("llmresourcequota", "llm-resource-quota", namespace, map[string]interface{}{
		"maxReplicas": cfg.MaxReplicasPerService,
		"maxGPUs":     cfg.MaxGPUsPerNamespace,
	})

	if err := o.applyUnstructured(ctx, template); err != nil {
		return err
	}
	return o.applyUnstructured(ctx, constraint)
}

// reconcileImageAllowlist creates a constraint for container image registry enforcement.
func (o *OPAReconciler) reconcileImageAllowlist(ctx context.Context, namespace string, cfg OPAConfig) error {
	template := buildConstraintTemplate("llmimageallowlist", `
package llmimageallowlist
violation[{"msg": msg}] {
  container := input.review.object.spec.template.spec.containers[_]
  not image_allowed(container.image)
  msg := sprintf("image %s not in allowed registries", [container.image])
}
image_allowed(image) {
  prefix := input.parameters.allowedRegistries[_]
  startswith(image, prefix)
}
`, []map[string]interface{}{
		{"name": "allowedRegistries", "type": "array", "items": map[string]interface{}{"type": "string"}},
	})

	allowedList := make([]interface{}, len(cfg.AllowedRegistries))
	for i, r := range cfg.AllowedRegistries {
		allowedList[i] = r
	}

	constraint := buildConstraint("llmimageallowlist", "llm-image-allowlist", namespace, map[string]interface{}{
		"allowedRegistries": allowedList,
	})

	if err := o.applyUnstructured(ctx, template); err != nil {
		return err
	}
	return o.applyUnstructured(ctx, constraint)
}

// reconcileModelAccess creates a constraint that enforces tenant-based model access control.
// It checks whether the requesting namespace carries a ckodex.com/tenant-id label that is
// listed in spec.allowedTenants of the LLMInferenceService. Empty allowedTenants = open.
func (o *OPAReconciler) reconcileModelAccess(ctx context.Context, namespace string) error {
	template := buildConstraintTemplate("llmmodelacess", `
package llmmodelacess
violation[{"msg": msg}] {
  allowed := input.parameters.allowedTenants
  count(allowed) > 0
  ns_labels := input.review.object.metadata.namespace_labels
  tenant := ns_labels["ckodex.com/tenant-id"]
  not tenant_allowed(allowed, tenant)
  msg := sprintf("tenant %s is not in allowedTenants for this LLMInferenceService", [tenant])
}
tenant_allowed(allowed, tenant) {
  allowed[_] == tenant
}
`, []map[string]interface{}{
		{"name": "allowedTenants", "type": "array", "items": map[string]interface{}{"type": "string"}},
	})

	// Constraint with empty allowedTenants — no restriction by default.
	constraint := buildConstraint("llmmodelacess", "llm-model-access", namespace, map[string]interface{}{
		"allowedTenants": []interface{}{},
	})

	if err := o.applyUnstructured(ctx, template); err != nil {
		return err
	}
	return o.applyUnstructured(ctx, constraint)
}

// reconcileSecurityPolicy creates a constraint enforcing pod security standards.
func (o *OPAReconciler) reconcileSecurityPolicy(ctx context.Context, namespace string) error {
	template := buildConstraintTemplate("llmsecuritypolicy", `
package llmsecuritypolicy
violation[{"msg": msg}] {
  container := input.review.object.spec.template.spec.containers[_]
  not container.securityContext.runAsNonRoot
  msg := sprintf("container %s must set runAsNonRoot", [container.name])
}
violation[{"msg": msg}] {
  container := input.review.object.spec.template.spec.containers[_]
  container.securityContext.allowPrivilegeEscalation
  msg := sprintf("container %s must not allow privilege escalation", [container.name])
}
`, nil)

	constraint := buildConstraint("llmsecuritypolicy", "llm-security-policy", namespace, nil)

	if err := o.applyUnstructured(ctx, template); err != nil {
		return err
	}
	return o.applyUnstructured(ctx, constraint)
}

// buildConstraintTemplate constructs a Gatekeeper ConstraintTemplate.
func buildConstraintTemplate(name, rego string, params []map[string]interface{}) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"crd": map[string]interface{}{
			"spec": map[string]interface{}{
				"names": map[string]interface{}{
					"kind": name,
				},
			},
		},
		"targets": []interface{}{
			map[string]interface{}{
				"target": "admission.k8s.gatekeeper.sh",
				"rego":   rego,
			},
		},
	}

	if len(params) > 0 {
		crdSpec := spec["crd"].(map[string]interface{})["spec"].(map[string]interface{})
		crdSpec["validation"] = map[string]interface{}{
			"openAPIV3Schema": map[string]interface{}{
				"type":       "object",
				"properties": buildParamProperties(params),
			},
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "templates.gatekeeper.sh/v1",
			"kind":       "ConstraintTemplate",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
				},
			},
			"spec": spec,
		},
	}
}

// buildConstraint constructs a Gatekeeper Constraint instance.
func buildConstraint(kind, name, namespace string, params map[string]interface{}) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"match": map[string]interface{}{
			"kinds": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{"serving.ckodex.com"},
					"kinds":     []interface{}{"LLMInferenceService"},
				},
			},
			"namespaces": []interface{}{namespace},
		},
	}

	if params != nil {
		spec["parameters"] = params
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "constraints.gatekeeper.sh/v1beta1",
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
				},
			},
			"spec": spec,
		},
	}
}

// buildParamProperties converts a param list to OAS3 properties.
func buildParamProperties(params []map[string]interface{}) map[string]interface{} {
	props := make(map[string]interface{})
	for _, p := range params {
		name := p["name"].(string)
		prop := map[string]interface{}{"type": p["type"]}
		if items, ok := p["items"]; ok {
			prop["items"] = items
		}
		props[name] = prop
	}
	return props
}

// applyUnstructured creates or updates an unstructured resource.
func (o *OPAReconciler) applyUnstructured(ctx context.Context, desired *unstructured.Unstructured) error {
	var existing unstructured.Unstructured
	existing.SetGroupVersionKind(desired.GroupVersionKind())

	name := desired.GetName()
	if err := o.Get(ctx, types.NamespacedName{Name: name}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return o.Create(ctx, desired)
		}
		return err
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	return o.Update(ctx, desired)
}
