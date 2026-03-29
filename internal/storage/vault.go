/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"fmt"
	"os"

	vault "github.com/hashicorp/vault/api"
)

// VaultClient handles fetching secrets from HashiCorp Vault.
type VaultClient struct {
	client *vault.Client
}

// NewVaultClient creates a new VaultClient.
func NewVaultClient() (*VaultClient, error) {
	config := vault.DefaultConfig()
	// VAULT_ADDR and VAULT_TOKEN should be set via env vars or K8s auth
	client, err := vault.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &VaultClient{client: client}, nil
}

// FetchSecret retrieves a map of secrets from the given Vault path.
func (v *VaultClient) FetchSecret(ctx context.Context, path string) (map[string]interface{}, error) {
	secret, err := v.client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read from Vault path %s: %w", path, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("no data found at Vault path %s", path)
	}
	return secret.Data, nil
}

// InjectVaultSecrets fetches secrets from Vault and injects them into the current environment.
// This is used by the storage-initializer to set HF_TOKEN, GITHUB_TOKEN, etc.
func InjectVaultSecrets(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}

	v, err := NewVaultClient()
	if err != nil {
		return err
	}

	secrets, err := v.FetchSecret(ctx, path)
	if err != nil {
		return err
	}

	for k, v := range secrets {
		val := fmt.Sprintf("%v", v)
		if err := os.Setenv(k, val); err != nil {
			return fmt.Errorf("failed to set env var %s from Vault: %w", k, err)
		}
		fmt.Printf("Injected secret %s from Vault\n", k)
	}

	return nil
}
