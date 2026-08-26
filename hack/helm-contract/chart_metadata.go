/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

type chartMetadata struct {
	AppVersion string `yaml:"appVersion"`
}

func chartAppVersion(chart string) (string, error) {
	root, err := os.OpenRoot(chart)
	if err != nil {
		return "", fmt.Errorf("open chart root %q: %w", chart, err)
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile("Chart.yaml")
	if err != nil {
		return "", fmt.Errorf("read %s/Chart.yaml: %w", chart, err)
	}
	var metadata chartMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("parse %s/Chart.yaml: %w", chart, err)
	}
	if strings.TrimSpace(metadata.AppVersion) == "" {
		return "", fmt.Errorf("%s/Chart.yaml: appVersion is required", chart)
	}
	return metadata.AppVersion, nil
}
