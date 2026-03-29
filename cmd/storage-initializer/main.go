/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ckodex-labs/kserve-llm-operator/internal/storage"
)

func main() {
	args := os.Args[1:]

	// Parse flags before positional arguments.
	var skipChecksum bool
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--skip-checksum":
			skipChecksum = true
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s [--skip-checksum] <uri> <destPath>\n", os.Args[0])
		os.Exit(1)
	}

	uri := positional[0]
	destPath := positional[1]

	// Propagate --skip-checksum to env so downstream clients can read it.
	if skipChecksum {
		os.Setenv("SKIP_CHECKSUM", "1")
	}

	fmt.Printf("Starting CKodex Storage Initializer...\n")

	// Inject secrets from Vault if configured
	ctx := context.Background() // Define ctx earlier for Vault injection
	vaultPath := os.Getenv("VAULT_PATH")
	if vaultPath != "" {
		fmt.Printf("Fetching secrets from Vault path: %s\n", vaultPath)
		if err := storage.InjectVaultSecrets(ctx, vaultPath); err != nil {
			fmt.Printf("Warning: Failed to inject Vault secrets: %v\n", err)
		}
	}

	fmt.Printf("Source URI: %s\n", uri)
	fmt.Printf("Destination: %s\n", destPath)

	if skipChecksum {
		fmt.Printf("Checksum verification: DISABLED (--skip-checksum)\n")
	}

	// Determine the scheme
	parts := strings.SplitN(uri, "://", 2)
	if len(parts) < 2 {
		fmt.Fprintf(os.Stderr, "Invalid URI: %s\n", uri)
		os.Exit(1)
	}
	scheme := parts[0]

	// Get the registered client
	client, err := storage.GetClient(scheme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check if destination is already populated (Idempotency / Cache Hit)
	if entries, err := os.ReadDir(destPath); err == nil && len(entries) > 0 {
		fmt.Printf("Optimization: Destination %s already contains %d files. Skipping download.\n", destPath, len(entries))
		os.Exit(0)
	}

	// Pull the artifact
	if err := client.Pull(ctx, uri, destPath); err != nil {
		fmt.Fprintf(os.Stderr, "Pull failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully downloaded model to %s\n", destPath)
}
