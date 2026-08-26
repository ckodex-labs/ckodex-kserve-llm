/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChartAppVersion(t *testing.T) {
	chart := t.TempDir()
	path := filepath.Join(chart, "Chart.yaml")
	if err := os.WriteFile(path, []byte("apiVersion: v2\nappVersion: v1.2.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	version, err := chartAppVersion(chart)
	if err != nil {
		t.Fatal(err)
	}
	if version != "v1.2.3" {
		t.Fatalf("chartAppVersion() = %q, want v1.2.3", version)
	}
}

func TestChartAppVersionRequiresValue(t *testing.T) {
	chart := t.TempDir()
	path := filepath.Join(chart, "Chart.yaml")
	if err := os.WriteFile(path, []byte("apiVersion: v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := chartAppVersion(chart); err == nil {
		t.Fatal("chartAppVersion() error = nil, want missing appVersion error")
	}
}
