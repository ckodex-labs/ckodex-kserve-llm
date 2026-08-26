/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func validateManagedEPPProfile(ctx context.Context, chart string) error {
	objects, err := renderChart(ctx, chart, "managedNamespaces[0]=team-a")
	if err != nil {
		return err
	}
	serviceAccount := findNamed(objects, "ServiceAccount", "ckodex-epp")
	role := findNamed(objects, "Role", "ckodex-epp")
	binding := findNamed(objects, "RoleBinding", "ckodex-epp")
	if serviceAccount == nil || role == nil || binding == nil {
		return fmt.Errorf("%s: managed namespace profile must render the ckodex-epp ServiceAccount, Role, and RoleBinding", chart)
	}
	if serviceAccount.GetNamespace() != "team-a" || role.GetNamespace() != "team-a" || binding.GetNamespace() != "team-a" {
		return fmt.Errorf("%s: ckodex-epp RBAC objects must be namespaced to the managed namespace", chart)
	}
	label, _, err := unstructured.NestedString(serviceAccount.Object, "metadata", "labels", "serving.ckodex.com/epp-rbac")
	if err != nil || label != "preprovisioned" {
		return fmt.Errorf("%s: ckodex-epp ServiceAccount is missing the preprovisioned identity label", chart)
	}
	if !bindingHasServiceAccount(binding, "ckodex-epp") {
		return fmt.Errorf("%s: ckodex-epp RoleBinding does not bind the shared EPP ServiceAccount", chart)
	}
	return validateNoDynamicRBACMutation(chart, objects)
}

func validateNoDynamicRBACMutation(chart string, objects []*unstructured.Unstructured) error {
	for _, object := range objects {
		if object.GetKind() != "ClusterRole" {
			continue
		}
		rules, found, err := unstructured.NestedSlice(object.Object, "rules")
		if err != nil || !found {
			continue
		}
		for _, rawRule := range rules {
			rule, ok := rawRule.(map[string]interface{})
			if !ok {
				return fmt.Errorf("%s: ClusterRole %q contains an invalid RBAC rule", chart, object.GetName())
			}
			resources, _, _ := unstructured.NestedStringSlice(rule, "resources")
			verbs, _, _ := unstructured.NestedStringSlice(rule, "verbs")
			if err := validateRBACMutationRule(chart, object.GetName(), resources, verbs); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRBACMutationRule(chart, role string, resources, verbs []string) error {
	for _, resource := range resources {
		if resource != "roles" && resource != "rolebindings" {
			continue
		}
		for _, verb := range verbs {
			if verb == "create" || verb == "delete" || verb == "patch" || verb == "update" {
				return fmt.Errorf("%s: ClusterRole %q retains dynamic %s mutation verb %q", chart, role, resource, verb)
			}
		}
	}
	return nil
}
