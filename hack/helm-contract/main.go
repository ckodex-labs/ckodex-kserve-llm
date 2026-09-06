/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	operatorImageRepository    = "ghcr.io/ckodex-labs/ckodex-kserve-llm:"
	initializerImageRepository = "ghcr.io/ckodex-labs/ckodex-kserve-llm-huggingface-initializer:"
	consoleImageRepository     = "ghcr.io/ckodex-labs/ckodex-kserve-llm-console:"
	defaultRuntimeImage        = "vllm/vllm-openai:v0.28.0"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "helm contract:", err)
		os.Exit(1)
	}
	fmt.Println("helm install contracts passed")
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, chart := range []string{"charts/ckodex-kserve-llm-operator", "deploy/helm"} {
		releaseTag, err := chartAppVersion(chart)
		if err != nil {
			return err
		}
		objects, err := renderChart(ctx, chart)
		if err != nil {
			return err
		}
		if err := validateInstallContract(chart, releaseTag, objects); err != nil {
			return err
		}
		if err := validateManagedEPPProfile(ctx, chart); err != nil {
			return err
		}
		consoleObjects, err := renderChart(
			ctx,
			chart,
			"console.enabled=true",
			"console.image.repository=ghcr.io/ckodex-labs/ckodex-kserve-llm-console",
		)
		if err != nil {
			return err
		}
		if err := validateConsoleInstallContract(chart, releaseTag, consoleObjects); err != nil {
			return err
		}
	}
	if err := validateWebhookTLS(ctx); err != nil {
		return err
	}
	if err := validateBetaConversionIdentity(ctx); err != nil {
		return err
	}
	return validateStaticRBAC()
}

func validateConsoleInstallContract(chart, releaseTag string, objects []*unstructured.Unstructured) error {
	deployment := findSuffix(objects, "Deployment", "-console")
	if deployment == nil {
		return fmt.Errorf("%s: console-enabled install has no console Deployment", chart)
	}
	serviceAccount, _, _ := unstructured.NestedString(
		deployment.Object, "spec", "template", "spec", "serviceAccountName")
	if serviceAccount == "" || findNamed(objects, "ServiceAccount", serviceAccount) == nil {
		return fmt.Errorf("%s: console Deployment service account %q is not rendered", chart, serviceAccount)
	}
	containers, _, _ := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	if len(containers) != 1 {
		return fmt.Errorf("%s: console Deployment has %d containers, want 1", chart, len(containers))
	}
	container, ok := containers[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s: console Deployment container is malformed", chart)
	}
	image, err := requiredString(container, "image", "console container")
	if err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	if image != consoleImageRepository+releaseTag {
		return fmt.Errorf("%s: console image %q does not follow the chart appVersion %q", chart, image, releaseTag)
	}
	if findSuffix(objects, "Service", "-console") == nil {
		return fmt.Errorf("%s: console-enabled install has no console Service", chart)
	}
	clusterRole := findSuffix(objects, "ClusterRole", "-console-observer")
	if clusterRole == nil {
		return fmt.Errorf("%s: console observer ClusterRole is not rendered", chart)
	}
	if err := validateConsoleReadOnlyRole(clusterRole); err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	clusterRoleBinding := findSuffix(objects, "ClusterRoleBinding", "-console-observer")
	if clusterRoleBinding == nil || !bindingHasServiceAccount(clusterRoleBinding, serviceAccount) {
		return fmt.Errorf("%s: console observer ClusterRoleBinding is missing or unbound", chart)
	}
	role := findSuffix(objects, "Role", "-console-spire-registrations")
	roleBinding := findSuffix(objects, "RoleBinding", "-console-spire-registrations")
	if role == nil || roleBinding == nil || !bindingHasServiceAccount(roleBinding, serviceAccount) {
		return fmt.Errorf("%s: console SPIRE registration Role or RoleBinding is missing or unbound", chart)
	}
	return nil
}

func validateConsoleReadOnlyRole(role *unstructured.Unstructured) error {
	rules, found, err := unstructured.NestedSlice(role.Object, "rules")
	if err != nil || !found {
		return errors.New("console observer ClusterRole has no rules")
	}
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok {
			return errors.New("console observer ClusterRole contains an invalid rule")
		}
		resources, _, _ := unstructured.NestedStringSlice(rule, "resources")
		verbs, _, _ := unstructured.NestedStringSlice(rule, "verbs")
		for _, resource := range resources {
			if resource == "selfsubjectreviews" || resource == "selfsubjectaccessreviews" {
				continue
			}
			for _, verb := range verbs {
				if verb == "create" || verb == "delete" || verb == "patch" || verb == "update" {
					return fmt.Errorf("console observer grants mutation verb %q on %s", verb, resource)
				}
			}
		}
	}
	return nil
}

func renderChart(ctx context.Context, chart string, values ...string) ([]*unstructured.Unstructured, error) {
	args := []string{"template", "contract", chart, "--namespace", "ckodex-system"}
	for _, value := range values {
		args = append(args, "--set", value)
	}
	output, err := exec.CommandContext(ctx, "helm", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("render %s: %w: %s", chart, err, output)
	}
	return decodeObjects(output)
}

func decodeObjects(manifest []byte) ([]*unstructured.Unstructured, error) {
	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 4096)
	var objects []*unstructured.Unstructured
	for {
		object := &unstructured.Unstructured{}
		if err := decoder.Decode(object); errors.Is(err, io.EOF) {
			return objects, nil
		} else if err != nil {
			return nil, fmt.Errorf("decode manifest: %w", err)
		}
		if object.GetKind() != "" {
			objects = append(objects, object)
		}
	}
}

func validateInstallContract(chart, releaseTag string, objects []*unstructured.Unstructured) error {
	deployment, err := exactlyOne(objects, "Deployment")
	if err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	serviceAccount, _, _ := unstructured.NestedString(
		deployment.Object, "spec", "template", "spec", "serviceAccountName")
	if serviceAccount == "" || findNamed(objects, "ServiceAccount", serviceAccount) == nil {
		return fmt.Errorf("%s: Deployment service account %q is not rendered", chart, serviceAccount)
	}
	rbacObjects, err := staticRBACObjects()
	if err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	allObjects := append(append([]*unstructured.Unstructured{}, objects...), rbacObjects...)
	if err := validateBindings(allObjects, objects, serviceAccount); err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	if err := validateRuntimeDefaults(deployment); err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	if err := validateReleaseImageDefaults(deployment, releaseTag); err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	return validateWebhookDisabled(deployment, objects)
}

func validateBindings(availableObjects []*unstructured.Unstructured, boundObjects []*unstructured.Unstructured, serviceAccount string) error {
	for _, kind := range []string{"ClusterRoleBinding", "RoleBinding"} {
		if kind == "ClusterRoleBinding" {
			binding, err := exactlyOneBoundBinding(availableObjects, kind, serviceAccount)
			if err != nil {
				return err
			}
			roleKind, _, _ := unstructured.NestedString(binding.Object, "roleRef", "kind")
			roleName, _, _ := unstructured.NestedString(binding.Object, "roleRef", "name")
			if findNamed(availableObjects, roleKind, roleName) == nil {
				return fmt.Errorf("%s references missing %s %q", kind, roleKind, roleName)
			}
			continue
		}
		roleBindings := boundBindings(availableObjects, kind, serviceAccount)
		if len(roleBindings) == 0 {
			return fmt.Errorf("rendered %d %s objects bound to service account %q, want at least one", len(roleBindings), kind, serviceAccount)
		}
		foundLeaderElectionBinding := false
		for _, binding := range roleBindings {
			roleKind, _, _ := unstructured.NestedString(binding.Object, "roleRef", "kind")
			roleName, _, _ := unstructured.NestedString(binding.Object, "roleRef", "name")
			if findNamed(availableObjects, roleKind, roleName) == nil {
				return fmt.Errorf("%s references missing %s %q", kind, roleKind, roleName)
			}
			if strings.HasSuffix(binding.GetName(), "-leader-election") {
				foundLeaderElectionBinding = true
			}
		}
		if !allRoleBindingsInBoundObjects(boundObjects, roleBindings) {
			return fmt.Errorf("%s: missing chart-rendered RoleBindings for service account %q", kind, serviceAccount)
		}
		if !foundLeaderElectionBinding {
			return fmt.Errorf("no %s for service account %q binds a leader-election role", kind, serviceAccount)
		}
	}
	return nil
}

func exactlyOneBoundBinding(
	objects []*unstructured.Unstructured,
	kind, serviceAccount string,
) (*unstructured.Unstructured, error) {
	var matches []*unstructured.Unstructured
	for _, object := range objects {
		if object.GetKind() != kind {
			continue
		}
		if bindingHasServiceAccount(object, serviceAccount) {
			matches = append(matches, object)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("rendered %d %s objects bound to service account %q, want exactly one", len(matches), kind, serviceAccount)
	}
	return matches[0], nil
}

func allRoleBindingsInBoundObjects(boundObjects, roleBindings []*unstructured.Unstructured) bool {
	for _, roleBinding := range roleBindings {
		matched := false
		for _, object := range boundObjects {
			if object.GetKind() == roleBinding.GetKind() && object.GetName() == roleBinding.GetName() {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func staticRBACObjects() ([]*unstructured.Unstructured, error) {
	var objects []*unstructured.Unstructured
	for _, path := range []string{
		"config/rbac/role.yaml",
		"config/rbac/tenant-role.yaml",
		"config/manager/manager.yaml",
	} {
		manifest, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		decoded, err := decodeObjects(manifest)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		objects = append(objects, decoded...)
	}
	return objects, nil
}

func boundBindings(
	objects []*unstructured.Unstructured,
	kind, serviceAccount string,
) []*unstructured.Unstructured {
	var bindings []*unstructured.Unstructured
	for _, object := range objects {
		if object.GetKind() != kind {
			continue
		}
		if bindingHasServiceAccount(object, serviceAccount) {
			bindings = append(bindings, object)
		}
	}
	return bindings
}

func validateRuntimeDefaults(deployment *unstructured.Unstructured) error {
	manager, err := managerContainer(deployment)
	if err != nil {
		return err
	}
	image, err := requiredString(manager, "image", "manager container")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(image, operatorImageRepository) {
		return fmt.Errorf("manager image %q does not use the public operator repository", image)
	}
	if value := envValue(manager, "CKODEX_RUNTIME_IMAGE"); value != defaultRuntimeImage {
		return fmt.Errorf("CKODEX_RUNTIME_IMAGE = %q, want %q", value, defaultRuntimeImage)
	}
	return nil
}

func validateReleaseImageDefaults(deployment *unstructured.Unstructured, releaseTag string) error {
	manager, err := managerContainer(deployment)
	if err != nil {
		return err
	}
	image, err := requiredString(manager, "image", "manager container")
	if err != nil {
		return err
	}
	if image != operatorImageRepository+releaseTag {
		return fmt.Errorf("manager image %q does not follow the chart appVersion %q", image, releaseTag)
	}
	if value := envValue(manager, "CKODEX_HUGGING_FACE_INITIALIZER_IMAGE"); value != initializerImageRepository+releaseTag {
		return fmt.Errorf("CKODEX_HUGGING_FACE_INITIALIZER_IMAGE = %q, want %q", value, initializerImageRepository+releaseTag)
	}
	return nil
}

func validateWebhookDisabled(
	deployment *unstructured.Unstructured,
	objects []*unstructured.Unstructured,
) error {
	for _, kind := range []string{
		"Certificate", "Issuer", "MutatingWebhookConfiguration", "ValidatingWebhookConfiguration",
	} {
		if countKind(objects, kind) != 0 {
			return fmt.Errorf("default install unexpectedly renders %s", kind)
		}
	}
	volumes, _, _ := unstructured.NestedSlice(
		deployment.Object, "spec", "template", "spec", "volumes")
	for _, raw := range volumes {
		volume, ok := raw.(map[string]interface{})
		if !ok {
			return errors.New("default Deployment contains a malformed volume")
		}
		if volume["name"] == "cert" {
			return errors.New("default Deployment has a dangling webhook certificate volume")
		}
	}
	return nil
}

func validateWebhookTLS(ctx context.Context) error {
	objects, err := renderChart(
		ctx, "deploy/helm", "webhook.enabled=true", "certManager.enabled=true")
	if err != nil {
		return err
	}
	deployment, err := exactlyOne(objects, "Deployment")
	if err != nil {
		return err
	}
	certificate := findCertificate(objects, "-webhook-cert")
	if certificate == nil {
		return errors.New("webhook-enabled install has no leaf Certificate")
	}
	secretName, _, _ := unstructured.NestedString(certificate.Object, "spec", "secretName")
	if secretName == "" || deploymentSecret(deployment, "cert") != secretName {
		return fmt.Errorf("webhook Certificate secret %q does not match Deployment volume", secretName)
	}
	for _, kind := range []string{"MutatingWebhookConfiguration", "ValidatingWebhookConfiguration"} {
		webhook, err := exactlyOne(objects, kind)
		if err != nil {
			return err
		}
		if webhook.GetAnnotations()["cert-manager.io/inject-ca-from"] == "" {
			return fmt.Errorf("%s has no cert-manager CA injection annotation", kind)
		}
	}
	return validateWebhookRequiresTLS(ctx)
}

func validateWebhookRequiresTLS(ctx context.Context) error {
	args := []string{
		"template", "contract", "deploy/helm", "--namespace", "ckodex-system",
		"--set", "webhook.enabled=true", "--set", "certManager.enabled=false",
	}
	output, err := exec.CommandContext(ctx, "helm", args...).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "certManager.enabled must be true") {
		return errors.New("webhook without cert-manager did not fail closed")
	}
	return nil
}

func validateBetaConversionIdentity(ctx context.Context) error {
	objects, err := renderChart(
		ctx,
		"deploy/helm",
		"fullnameOverride=ckodex-kserve-llm-operator",
		"webhook.enabled=true",
		"certManager.enabled=true",
	)
	if err != nil {
		return err
	}
	const (
		serviceName = "ckodex-kserve-llm-operator-webhook-service"
		certName    = "ckodex-kserve-llm-operator-webhook-cert"
	)
	service := findNamed(objects, "Service", serviceName)
	if service == nil || service.GetNamespace() != "ckodex-system" {
		return fmt.Errorf("beta conversion service %q is not rendered in ckodex-system", serviceName)
	}
	certificate := findNamed(objects, "Certificate", certName)
	if certificate == nil || certificate.GetNamespace() != "ckodex-system" {
		return fmt.Errorf("beta conversion Certificate %q is not rendered in ckodex-system", certName)
	}
	secretName, _, _ := unstructured.NestedString(certificate.Object, "spec", "secretName")
	if secretName != certName {
		return fmt.Errorf("beta conversion Certificate secret %q, want %q", secretName, certName)
	}
	for _, kind := range []string{"MutatingWebhookConfiguration", "ValidatingWebhookConfiguration"} {
		webhook, err := exactlyOne(objects, kind)
		if err != nil {
			return err
		}
		webhooks, found, err := unstructured.NestedSlice(webhook.Object, "webhooks")
		if err != nil || !found || len(webhooks) == 0 {
			return fmt.Errorf("%s has no webhook entries", kind)
		}
		entry, ok := webhooks[0].(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s has an invalid webhook entry", kind)
		}
		name, _, _ := unstructured.NestedString(entry, "clientConfig", "service", "name")
		namespace, _, _ := unstructured.NestedString(entry, "clientConfig", "service", "namespace")
		if name != serviceName || namespace != "ckodex-system" {
			return fmt.Errorf("%s targets %s/%s, want ckodex-system/%s", kind, namespace, name, serviceName)
		}
	}
	return nil
}

func validateStaticRBAC() error {
	var objects []*unstructured.Unstructured
	for _, path := range []string{
		"config/rbac/role.yaml",
		"config/rbac/tenant-role.yaml",
		"config/manager/manager.yaml",
	} {
		manifest, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		decoded, err := decodeObjects(manifest)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		objects = append(objects, decoded...)
	}
	for _, object := range objects {
		if !strings.HasSuffix(object.GetKind(), "RoleBinding") {
			continue
		}
		roleKind, _, _ := unstructured.NestedString(object.Object, "roleRef", "kind")
		roleName, _, _ := unstructured.NestedString(object.Object, "roleRef", "name")
		if findNamed(objects, roleKind, roleName) == nil {
			return fmt.Errorf("%s %q references missing %s %q",
				object.GetKind(), object.GetName(), roleKind, roleName)
		}
	}
	return nil
}

func exactlyOne(objects []*unstructured.Unstructured, kind string) (*unstructured.Unstructured, error) {
	var found []*unstructured.Unstructured
	for _, object := range objects {
		if object.GetKind() == kind {
			found = append(found, object)
		}
	}
	if len(found) != 1 {
		return nil, fmt.Errorf("rendered %d %s objects, want exactly one", len(found), kind)
	}
	return found[0], nil
}

func findNamed(objects []*unstructured.Unstructured, kind, name string) *unstructured.Unstructured {
	for _, object := range objects {
		if object.GetKind() == kind && object.GetName() == name {
			return object
		}
	}
	return nil
}

func findSuffix(objects []*unstructured.Unstructured, kind, suffix string) *unstructured.Unstructured {
	for _, object := range objects {
		if object.GetKind() == kind && strings.HasSuffix(object.GetName(), suffix) {
			return object
		}
	}
	return nil
}

func countKind(objects []*unstructured.Unstructured, kind string) int {
	count := 0
	for _, object := range objects {
		if object.GetKind() == kind {
			count++
		}
	}
	return count
}

func bindingHasServiceAccount(binding *unstructured.Unstructured, name string) bool {
	subjects, _, _ := unstructured.NestedSlice(binding.Object, "subjects")
	for _, raw := range subjects {
		subject, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if subject["kind"] == "ServiceAccount" && subject["name"] == name {
			return true
		}
	}
	return false
}

func findCertificate(
	objects []*unstructured.Unstructured,
	suffix string,
) *unstructured.Unstructured {
	for _, object := range objects {
		if object.GetKind() == "Certificate" && strings.HasSuffix(object.GetName(), suffix) {
			return object
		}
	}
	return nil
}
