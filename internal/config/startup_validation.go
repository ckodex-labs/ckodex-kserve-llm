/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package config

import (
	"fmt"
	"log/slog"
	"os"
)

// gcCredentialsFile is the standard ADC env var for GCS / Google Cloud SDKs.
const gcCredentialsFile = "GOOGLE_APPLICATION_CREDENTIALS"

// ValidateStorageCredentials checks that credentials for enabled storage
// backends are present at startup. Logs warnings for optional backends,
// returns an error for required ones so the manager can exit cleanly.
func ValidateStorageCredentials(_ *OperatorConfig) error {
	// HF_TOKEN: warn only — public models (gpt2, Qwen 0.5B) do not need it.
	if os.Getenv("HF_TOKEN") == "" && os.Getenv("VAULT_PATH") == "" {
		slog.Warn("HF_TOKEN is not set and VAULT_PATH is not configured; " +
			"private HuggingFace models will fail at first pull")
	}

	// If VAULT_PATH is configured, the Vault client deps are required.
	if vaultPath := os.Getenv("VAULT_PATH"); vaultPath != "" {
		var missing []string
		if os.Getenv("VAULT_ADDR") == "" {
			missing = append(missing, "VAULT_ADDR")
		}
		if os.Getenv("VAULT_TOKEN") == "" {
			missing = append(missing, "VAULT_TOKEN")
		}
		if len(missing) > 0 {
			return fmt.Errorf("VAULT_PATH is set to %q but required credentials are missing: %v", vaultPath, missing)
		}
	}

	// GOOGLE_APPLICATION_CREDENTIALS: if set, the file must exist.
	// If not set, warn only — Workload Identity (GKE) needs no env var.
	if gcFile := os.Getenv(gcCredentialsFile); gcFile != "" {
		if _, err := os.Stat(gcFile); os.IsNotExist(err) {
			return fmt.Errorf("%s is set to %q but the file does not exist", gcCredentialsFile, gcFile)
		}
	} else {
		slog.Warn("GOOGLE_APPLICATION_CREDENTIALS is not set; GCS pulls will rely on Workload Identity or application-default credentials")
	}

	return nil
}
