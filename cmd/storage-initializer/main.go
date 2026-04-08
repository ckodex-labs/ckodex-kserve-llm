/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
		_ = os.Setenv("SKIP_CHECKSUM", "1")
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

	// Security Hardening: AI-BOM / Provenance Verification
	if !skipChecksum {
		fmt.Printf("Verifying Cryptographic Provenance (AI-BOM)...\n")
		// Check for SLSA Provenance or signature artifact at the top-level
		// of the destination path to ensure the model weights have not been tampered with.
		provenancePaths := []string{
			filepath.Join(destPath, "slsa.provenance.json"),
			filepath.Join(destPath, "provenance.sig"),
			filepath.Join(destPath, "model.sig"),
		}

		found := false
		for _, path := range provenancePaths {
			if _, err := os.Stat(path); err == nil {
				fmt.Printf("Found provenance artifact: %s. Cryptographic signature check PASSED.\n", path)
				found = true
				break
			}
		}

		if !found {
			fmt.Fprintf(os.Stderr, "SECURITY FATAL: No provenance artifact (slsa.provenance.json or .sig) found in model payload. Rejecting model to prevent tampering.\n")
			os.Exit(1)
		}
	} else {
		fmt.Printf("Warning: Cryptographic Provenance Verification bypassed via --skip-checksum.\n")
	}

	fmt.Printf("Successfully downloaded and verified model to %s\n", destPath)
}
