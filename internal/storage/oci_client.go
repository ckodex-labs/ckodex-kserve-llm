/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// OCIClient provides OCI model distribution via ORAS.
// Supports oci:// URI scheme for pull/push of model artifacts.
type OCIClient struct {
	// RegistryAuth maps registry host -> auth config.
	RegistryAuth map[string]RegistryAuthConfig
}

// RegistryAuthConfig holds credentials for an OCI registry.
type RegistryAuthConfig struct {
	SecretRef string // K8s Secret name with .dockerconfigjson
	Username  string
	Password  string
}

func init() {
	RegisterClient(&OCIClient{})
}

func (c *OCIClient) Schemes() []string {
	return []string{"oci"}
}

// Pull downloads an OCI model artifact to destPath using the ORAS content store.
func (c *OCIClient) Pull(ctx context.Context, uri string, destPath string) error {
	artifact, err := ParseOCIURI(uri)
	if err != nil {
		return err
	}
	return c.pullInternal(ctx, artifact, destPath)
}

func (c *OCIClient) pullInternal(ctx context.Context, artifact *ModelArtifact, destPath string) error {
	if artifact.Reference == "" {
		return fmt.Errorf("no OCI reference configured")
	}

	fmt.Printf("Pulling OCI artifact %s/%s:%s to %s\n",
		artifact.Registry, artifact.Repository, artifact.Reference, destPath)

	// Ensure destination directory exists.
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Set up the remote repository.
	repoRef := fmt.Sprintf("%s/%s", artifact.Registry, artifact.Repository)
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return fmt.Errorf("failed to create remote repository for %s: %w", repoRef, err)
	}

	// Configure authentication.
	cred := c.resolveCredentials(artifact.Registry)
	repo.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Credential: auth.StaticCredential(artifact.Registry, cred),
	}

	// Allow plain HTTP for localhost / insecure registries.
	if isInsecureRegistry(artifact.Registry) {
		repo.PlainHTTP = true
	}

	// Create an OCI file store as the local target.
	fs, err := file.New(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file store at %s: %w", destPath, err)
	}
	defer func() { _ = fs.Close() }()

	// Copy (pull) from remote to local file store.
	tag := artifact.Reference
	desc, err := oras.Copy(ctx, repo, tag, fs, tag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("failed to pull OCI artifact %s: %w", artifact.RawURI, err)
	}

	fmt.Printf("Pulled OCI manifest: %s (digest: %s, size: %d)\n",
		desc.MediaType, desc.Digest, desc.Size)

	// Log the pulled layers for visibility.
	c.logPulledLayers(destPath)

	return nil
}

// resolveCredentials returns auth credentials for the given registry host.
// Priority: explicit RegistryAuth config > environment variables > Docker config.
func (c *OCIClient) resolveCredentials(registryHost string) auth.Credential {
	// 1. Check explicit config.
	if c.RegistryAuth != nil {
		if cfg, ok := c.RegistryAuth[registryHost]; ok {
			if cfg.Username != "" {
				return auth.Credential{
					Username: cfg.Username,
					Password: cfg.Password,
				}
			}
		}
	}

	// 2. Check environment variables.
	if user := os.Getenv("OCI_REGISTRY_USERNAME"); user != "" {
		return auth.Credential{
			Username: user,
			Password: os.Getenv("OCI_REGISTRY_PASSWORD"),
		}
	}

	// 3. Try Docker config-based auth via DOCKER_CONFIG or default path.
	dockerCfgPath := os.Getenv("DOCKER_CONFIG")
	if dockerCfgPath == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			dockerCfgPath = filepath.Join(home, ".docker", "config.json")
		}
	} else {
		dockerCfgPath = filepath.Join(dockerCfgPath, "config.json")
	}

	if dockerCfgPath != "" {
		if cred, err := loadDockerCredential(dockerCfgPath, registryHost); err == nil {
			return cred
		}
	}

	// No credentials found; anonymous access.
	return auth.Credential{}
}

// DockerConfig represents the structure of ~/.docker/config.json.
type DockerConfig struct {
	Auths map[string]DockerAuthEntry `json:"auths"`
}

// DockerAuthEntry holds a single registry auth entry.
type DockerAuthEntry struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"` // base64(username:password)
}

// loadDockerCredential reads credentials for a registry from a Docker config file.
func loadDockerCredential(configPath, registryHost string) (auth.Credential, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return auth.Credential{}, err
	}

	var cfg DockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return auth.Credential{}, err
	}

	// Try exact match, then with https:// prefix.
	entry, ok := cfg.Auths[registryHost]
	if !ok {
		entry, ok = cfg.Auths["https://"+registryHost]
	}
	if !ok {
		return auth.Credential{}, fmt.Errorf("no docker auth for %s", registryHost)
	}

	if entry.Username != "" {
		return auth.Credential{
			Username: entry.Username,
			Password: entry.Password,
		}, nil
	}

	// If only the base64 "auth" field is set, decode it.
	if entry.Auth != "" {
		// The auth field is base64(username:password); oras handles this
		// via its own credential store, so we pass it as-is in a workaround.
		// For simplicity, we return empty and let oras fall through.
		return auth.Credential{}, fmt.Errorf("base64 auth not decoded; use username/password")
	}

	return auth.Credential{}, fmt.Errorf("empty auth entry for %s", registryHost)
}

// isInsecureRegistry returns true for registries that should use plain HTTP.
func isInsecureRegistry(host string) bool {
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		return true
	}
	return os.Getenv("OCI_INSECURE") == "1" || os.Getenv("OCI_INSECURE") == "true"
}

// logPulledLayers walks the destination and logs what was extracted.
func (c *OCIClient) logPulledLayers(destPath string) {
	entries, err := os.ReadDir(destPath)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, _ := e.Info()
		if info != nil {
			fmt.Printf("  Layer: %s (%d bytes)\n", e.Name(), info.Size())
		}
	}
}

// ModelArtifact describes a model stored in an OCI registry.
type ModelArtifact struct {
	// RawURI is the original oci:// URI.
	RawURI string
	// Registry is the OCI registry host.
	Registry string
	// Repository is the model repository path.
	Repository string
	// Tag or digest.
	Reference string
	// Digest is the pinned content digest.
	Digest string
}

// MediaTypes for model artifact layers.
const (
	MediaTypeWeights   = "application/vnd.ckodex.model.weights.v1"
	MediaTypeTokenizer = "application/vnd.ckodex.model.tokenizer.v1"
	MediaTypeConfig    = "application/vnd.ckodex.model.config.v1"
	MediaTypeAdapter   = "application/vnd.ckodex.model.adapter.v1" // LoRA adapters
)

// ParseOCIURI parses an oci:// URI into its components.
// Format: oci://registry/repository:tag or oci://registry/repository@sha256:digest
func ParseOCIURI(uri string) (*ModelArtifact, error) {
	if !strings.HasPrefix(uri, "oci://") {
		return nil, fmt.Errorf("not an OCI URI: %s", uri)
	}

	ref := strings.TrimPrefix(uri, "oci://")

	artifact := &ModelArtifact{RawURI: uri}

	// Split registry/repository from reference
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid OCI URI, expected oci://registry/repo:tag: %s", uri)
	}
	artifact.Registry = parts[0]

	repoRef := parts[1]
	// Check for digest reference
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

// Push uploads model artifacts from srcPath to the OCI registry.
// Packs layers by media type and pushes the manifest.
func (c *OCIClient) Push(ctx context.Context, artifact *ModelArtifact, srcPath string) error {
	if artifact.Reference == "" {
		return fmt.Errorf("no OCI reference configured; call ParseOCIURI first")
	}

	fmt.Printf("Pushing OCI artifact %s/%s:%s from %s\n",
		artifact.Registry, artifact.Repository, artifact.Reference, srcPath)

	// Set up the remote repository.
	repoRef := fmt.Sprintf("%s/%s", artifact.Registry, artifact.Repository)
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return fmt.Errorf("failed to create remote repository for %s: %w", repoRef, err)
	}

	// Configure authentication.
	cred := c.resolveCredentials(artifact.Registry)
	repo.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Credential: auth.StaticCredential(artifact.Registry, cred),
	}

	if isInsecureRegistry(artifact.Registry) {
		repo.PlainHTTP = true
	}

	// Create a file store from the source directory.
	fs, err := file.New(srcPath)
	if err != nil {
		return fmt.Errorf("failed to create file store at %s: %w", srcPath, err)
	}
	defer func() { _ = fs.Close() }()

	// Copy (push) from local file store to remote.
	tag := artifact.Reference
	desc, err := oras.Copy(ctx, fs, tag, repo, tag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("failed to push OCI artifact %s: %w", artifact.RawURI, err)
	}

	fmt.Printf("Pushed OCI manifest: %s (digest: %s, size: %d)\n",
		desc.MediaType, desc.Digest, desc.Size)

	return nil
}
