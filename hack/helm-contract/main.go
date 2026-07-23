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
	operatorImageRepository = "ghcr.io/ckodex-labs/ckodex-kserve-llm:"
	defaultRuntimeImage     = "vllm/vllm-openai:v0.25.1"
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
		objects, err := renderChart(ctx, chart)
		if err != nil {
			return err
		}
		if err := validateInstallContract(chart, objects); err != nil {
			return err
		}
	}
	if err := validateWebhookTLS(ctx); err != nil {
		return err
	}
	return validateStaticRBAC()
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

func validateInstallContract(chart string, objects []*unstructured.Unstructured) error {
	deployment, err := exactlyOne(objects, "Deployment")
	if err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	serviceAccount, _, _ := unstructured.NestedString(
		deployment.Object, "spec", "template", "spec", "serviceAccountName")
	if serviceAccount == "" || findNamed(objects, "ServiceAccount", serviceAccount) == nil {
		return fmt.Errorf("%s: Deployment service account %q is not rendered", chart, serviceAccount)
	}
	if err := validateBindings(objects, serviceAccount); err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	if err := validateRuntimeDefaults(deployment); err != nil {
		return fmt.Errorf("%s: %w", chart, err)
	}
	return validateWebhookDisabled(deployment, objects)
}

func validateBindings(objects []*unstructured.Unstructured, serviceAccount string) error {
	for _, kind := range []string{"ClusterRoleBinding", "RoleBinding"} {
		binding, err := exactlyOne(objects, kind)
		if err != nil {
			return err
		}
		roleKind, _, _ := unstructured.NestedString(binding.Object, "roleRef", "kind")
		roleName, _, _ := unstructured.NestedString(binding.Object, "roleRef", "name")
		if findNamed(objects, roleKind, roleName) == nil {
			return fmt.Errorf("%s references missing %s %q", kind, roleKind, roleName)
		}
		if !bindingHasServiceAccount(binding, serviceAccount) {
			return fmt.Errorf("%s does not bind ServiceAccount %q", kind, serviceAccount)
		}
	}
	return nil
}

func validateRuntimeDefaults(deployment *unstructured.Unstructured) error {
	containers, _, _ := unstructured.NestedSlice(
		deployment.Object, "spec", "template", "spec", "containers")
	if len(containers) == 0 {
		return errors.New("Deployment has no manager container")
	}
	manager := containers[0].(map[string]interface{})
	image, _ := manager["image"].(string)
	if !strings.HasPrefix(image, operatorImageRepository) {
		return fmt.Errorf("manager image %q does not use the public operator repository", image)
	}
	if value := envValue(manager, "CKODEX_RUNTIME_IMAGE"); value != defaultRuntimeImage {
		return fmt.Errorf("CKODEX_RUNTIME_IMAGE = %q, want %q", value, defaultRuntimeImage)
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
		volume := raw.(map[string]interface{})
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
		subject := raw.(map[string]interface{})
		if subject["kind"] == "ServiceAccount" && subject["name"] == name {
			return true
		}
	}
	return false
}

func envValue(container map[string]interface{}, name string) string {
	env, _, _ := unstructured.NestedSlice(container, "env")
	for _, raw := range env {
		value := raw.(map[string]interface{})
		if value["name"] == name {
			result, _ := value["value"].(string)
			return result
		}
	}
	return ""
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

func deploymentSecret(deployment *unstructured.Unstructured, volumeName string) string {
	volumes, _, _ := unstructured.NestedSlice(
		deployment.Object, "spec", "template", "spec", "volumes")
	for _, raw := range volumes {
		volume := raw.(map[string]interface{})
		if volume["name"] != volumeName {
			continue
		}
		secret, _ := volume["secret"].(map[string]interface{})
		name, _ := secret["secretName"].(string)
		return name
	}
	return ""
}
