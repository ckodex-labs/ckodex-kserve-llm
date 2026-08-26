/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func managerContainer(deployment *unstructured.Unstructured) (map[string]interface{}, error) {
	containers, found, err := unstructured.NestedSlice(
		deployment.Object, "spec", "template", "spec", "containers")
	if err != nil {
		return nil, fmt.Errorf("read deployment containers: %w", err)
	}
	if !found || len(containers) == 0 {
		return nil, errors.New("deployment has no manager container")
	}
	manager, ok := containers[0].(map[string]interface{})
	if !ok {
		return nil, errors.New("deployment manager container is malformed")
	}
	return manager, nil
}

func requiredString(object map[string]interface{}, key, label string) (string, error) {
	value, ok := object[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s %q is missing or malformed", label, key)
	}
	return value, nil
}

func envValue(container map[string]interface{}, name string) string {
	env, _, _ := unstructured.NestedSlice(container, "env")
	for _, raw := range env {
		value, ok := raw.(map[string]interface{})
		if !ok || value["name"] != name {
			continue
		}
		result, ok := value["value"].(string)
		if ok {
			return result
		}
		return ""
	}
	return ""
}

func deploymentSecret(deployment *unstructured.Unstructured, volumeName string) string {
	volumes, _, _ := unstructured.NestedSlice(
		deployment.Object, "spec", "template", "spec", "volumes")
	for _, raw := range volumes {
		volume, ok := raw.(map[string]interface{})
		if !ok || volume["name"] != volumeName {
			continue
		}
		secret, ok := volume["secret"].(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := secret["secretName"].(string)
		if ok {
			return name
		}
		return ""
	}
	return ""
}
