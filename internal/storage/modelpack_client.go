/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelpack/modctl/pkg/backend"
	"github.com/modelpack/modctl/pkg/config"
)

// ModelpackClient provides native integration for pulling CNCF ModelPack artifacts.
type ModelpackClient struct {
	// Add config for authentication if needed later.
	RegistryAuth map[string]RegistryAuthConfig
}

func init() {
	RegisterClient(&ModelpackClient{})
}

func (c *ModelpackClient) Schemes() []string {
	return []string{"modelpack"}
}

// Pull downloads and extracts a ModelPack format model into the destPath.
func (c *ModelpackClient) Pull(ctx context.Context, uri string, destPath string) error {
	artifact, err := ParseModelpackURI(uri)
	if err != nil {
		return err
	}
	return c.pullInternal(ctx, artifact, destPath)
}

// ParseModelpackURI parses a modelpack:// URI into a ModelArtifact.
// Format: modelpack://registry.com/repository:tag
func ParseModelpackURI(uri string) (*ModelArtifact, error) {
	if !strings.HasPrefix(uri, "modelpack://") {
		return nil, fmt.Errorf("not a Modelpack URI: %s", uri)
	}

	ref := strings.TrimPrefix(uri, "modelpack://")
	artifact := &ModelArtifact{RawURI: uri}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid Modelpack URI, expected modelpack://registry/repo:tag")
	}
	artifact.Registry = parts[0]

	repoRef := parts[1]
	if idx := strings.Index(repoRef, "@"); idx >= 0 {
		artifact.Repository = repoRef[:idx]
		artifact.Digest = repoRef[idx+1:]
		artifact.Reference = artifact.Digest
	} else if idx := strings.LastIndex(repoRef, ":"); idx >= 0 {
		artifact.Repository = repoRef[:idx]
		artifact.Reference = repoRef[idx+1:]
	} else {
		artifact.Repository = repoRef
		artifact.Reference = "latest"
	}

	return artifact, nil
}

func (c *ModelpackClient) pullInternal(ctx context.Context, artifact *ModelArtifact, destPath string) error {
	if artifact.Reference == "" {
		return fmt.Errorf("no Modelpack reference configured; call ParseModelpackURI first")
	}

	storageDir := filepath.Join(os.TempDir(), "modelpack-storage")
	b, err := backend.New(storageDir)
	if err != nil {
		return fmt.Errorf("failed to initialize modctl backend: %w", err)
	}

	// target string is the standard registry/repo:tag or host/repo:tag
	target := fmt.Sprintf("%s/%s:%s", artifact.Registry, artifact.Repository, artifact.Reference)
	if artifact.Digest != "" {
		target = fmt.Sprintf("%s/%s@%s", artifact.Registry, artifact.Repository, artifact.Digest)
	}

	// Prepare config for Pull operation (sets the dest directory).
	cfg := &config.Pull{
		ExtractDir: destPath,
		// Allow to run insecurely for local dev if necessary
		// Insecure: true,
	}

	if err := b.Pull(ctx, target, cfg); err != nil {
		return fmt.Errorf("failed to native pull modelpack artifact %q: %w", target, err)
	}

	return nil
}

// Push is currently stubbed out, similar to OCIClient, or implements backend.Push when needed.
func (c *ModelpackClient) Push(ctx context.Context, artifact *ModelArtifact, srcPath string) error {
	storageDir := filepath.Join(os.TempDir(), "modelpack-storage")
	b, err := backend.New(storageDir)
	if err != nil {
		return fmt.Errorf("failed to initialize modctl backend: %w", err)
	}

	target := fmt.Sprintf("%s/%s:%s", artifact.Registry, artifact.Repository, artifact.Reference)

	cfg := &config.Push{
		// push config
	}

	if err := b.Push(ctx, target, cfg); err != nil {
		return fmt.Errorf("modelpack push failed: %w", err)
	}
	return nil
}
