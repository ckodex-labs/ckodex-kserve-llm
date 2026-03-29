/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateStorageCredentials_NoEnvVars_ReturnsNil(t *testing.T) {
	// Clear all relevant env vars to ensure a clean slate.
	t.Setenv("HF_TOKEN", "")
	t.Setenv("VAULT_PATH", "")
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")

	cfg := DefaultOperatorConfig()
	if err := ValidateStorageCredentials(&cfg); err != nil {
		t.Fatalf("expected nil error with no env vars set, got: %v", err)
	}
}

func TestValidateStorageCredentials_VaultPath_MissingVaultAddr_ReturnsError(t *testing.T) {
	t.Setenv("VAULT_PATH", "secret/data/hf-token")
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "s.sometoken")

	cfg := DefaultOperatorConfig()
	err := ValidateStorageCredentials(&cfg)
	if err == nil {
		t.Fatal("expected error when VAULT_ADDR is missing, got nil")
	}
	if !strings.Contains(err.Error(), "VAULT_ADDR") {
		t.Errorf("error should mention VAULT_ADDR, got: %v", err)
	}
}

func TestValidateStorageCredentials_VaultPath_MissingVaultToken_ReturnsError(t *testing.T) {
	t.Setenv("VAULT_PATH", "secret/data/hf-token")
	t.Setenv("VAULT_ADDR", "https://vault.example.com")
	t.Setenv("VAULT_TOKEN", "")

	cfg := DefaultOperatorConfig()
	err := ValidateStorageCredentials(&cfg)
	if err == nil {
		t.Fatal("expected error when VAULT_TOKEN is missing, got nil")
	}
	if !strings.Contains(err.Error(), "VAULT_TOKEN") {
		t.Errorf("error should mention VAULT_TOKEN, got: %v", err)
	}
}

func TestValidateStorageCredentials_VaultPath_BothMissing_ErrorContainsBoth(t *testing.T) {
	t.Setenv("VAULT_PATH", "secret/data/hf-token")
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")

	cfg := DefaultOperatorConfig()
	err := ValidateStorageCredentials(&cfg)
	if err == nil {
		t.Fatal("expected error when both VAULT_ADDR and VAULT_TOKEN are missing, got nil")
	}
	if !strings.Contains(err.Error(), "VAULT_ADDR") {
		t.Errorf("error should mention VAULT_ADDR, got: %v", err)
	}
	if !strings.Contains(err.Error(), "VAULT_TOKEN") {
		t.Errorf("error should mention VAULT_TOKEN, got: %v", err)
	}
}

func TestValidateStorageCredentials_AllVaultVarsSet_ReturnsNil(t *testing.T) {
	t.Setenv("VAULT_PATH", "secret/data/hf-token")
	t.Setenv("VAULT_ADDR", "https://vault.example.com")
	t.Setenv("VAULT_TOKEN", "s.somevalidtoken")

	cfg := DefaultOperatorConfig()
	if err := ValidateStorageCredentials(&cfg); err != nil {
		t.Fatalf("expected nil error when all vault vars are set, got: %v", err)
	}
}

func TestValidateStorageCredentials_HFTokenSet_NoVaultPath_ReturnsNil(t *testing.T) {
	t.Setenv("HF_TOKEN", "hf_testtoken")
	t.Setenv("VAULT_PATH", "")
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")

	cfg := DefaultOperatorConfig()
	if err := ValidateStorageCredentials(&cfg); err != nil {
		t.Fatalf("expected nil error when HF_TOKEN is set, got: %v", err)
	}
}

func TestValidateStorageCredentials_GCSCredFile_Exists_ReturnsNil(t *testing.T) {
	// Write a real (empty) temp file so os.Stat succeeds.
	f, err := os.CreateTemp(t.TempDir(), "gcs-creds-*.json")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	t.Setenv("HF_TOKEN", "tok")
	t.Setenv("VAULT_PATH", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", f.Name())

	cfg := DefaultOperatorConfig()
	if err := ValidateStorageCredentials(&cfg); err != nil {
		t.Fatalf("expected nil when credentials file exists, got: %v", err)
	}
}

func TestValidateStorageCredentials_GCSCredFile_Missing_ReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")
	t.Setenv("HF_TOKEN", "tok")
	t.Setenv("VAULT_PATH", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", missing)

	cfg := DefaultOperatorConfig()
	err := ValidateStorageCredentials(&cfg)
	if err == nil {
		t.Fatal("expected error when credentials file does not exist, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention 'does not exist', got: %v", err)
	}
}
