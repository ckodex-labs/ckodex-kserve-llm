/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SeaweedFSClient provides SeaweedFS model distribution.
type SeaweedFSClient struct {
	// Config holds connection settings.
	Config SeaweedFSConfig
	// client is the internal HTTP client.
	client *http.Client
}

// SeaweedFSConfig holds SeaweedFS connection settings.
type SeaweedFSConfig struct {
	// FilerURL is the SeaweedFS Filer HTTP endpoint.
	FilerURL string `json:"filerURL"`

	// BasePath is the root path for model artifacts (e.g., "/models").
	BasePath string `json:"basePath"`

	// Timeout is the HTTP client timeout.
	Timeout time.Duration `json:"timeout"`
}

// DefaultSeaweedFSConfig returns production defaults.
func DefaultSeaweedFSConfig() SeaweedFSConfig {
	return SeaweedFSConfig{
		FilerURL: "http://seaweedfs-filer.storage:8888",
		BasePath: "/models",
		Timeout:  5 * time.Minute,
	}
}

// SeaweedFSArtifact describes a model artifact in SeaweedFS.
type SeaweedFSArtifact struct {
	// RawURI is the original seaweedfs:// URI.
	RawURI string
	// FilerHost is the filer hostname:port.
	FilerHost string
	// Path is the path on the filer (e.g., "/models/llama3").
	Path string
}

// ParseSeaweedFSURI parses a seaweedfs:// URI into its components.
// Format: seaweedfs://filer-host:port/path/to/model
func ParseSeaweedFSURI(uri string) (*SeaweedFSArtifact, error) {
	if !strings.HasPrefix(uri, "seaweedfs://") {
		return nil, fmt.Errorf("not a SeaweedFS URI: %s", uri)
	}

	ref := strings.TrimPrefix(uri, "seaweedfs://")
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid SeaweedFS URI, expected seaweedfs://host:port/path: %s", uri)
	}

	return &SeaweedFSArtifact{
		RawURI:    uri,
		FilerHost: parts[0],
		Path:      "/" + parts[1],
	}, nil
}

func init() {
	RegisterClient(&SeaweedFSClient{})
}

func (s *SeaweedFSClient) Schemes() []string {
	return []string{"seaweedfs"}
}

// Pull downloads a model artifact from SeaweedFS to the destPath.
func (s *SeaweedFSClient) Pull(ctx context.Context, uri string, destPath string) error {
	artifact, err := ParseSeaweedFSURI(uri)
	if err != nil {
		return err
	}
	// Assuming destPath is a directory, we download the directory or files into it.
	return s.Download(ctx, artifact.Path, destPath)
}

// Upload uploads a local file to SeaweedFS Filer.
func (s *SeaweedFSClient) Upload(ctx context.Context, localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file %s: %w", localPath, err)
	}
	defer func() { _ = file.Close() }()

	url := fmt.Sprintf("%s%s%s", s.Config.FilerURL, s.Config.BasePath, remotePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, file)
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("upload to %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Download downloads a file from SeaweedFS Filer to a local path.
func (s *SeaweedFSClient) Download(ctx context.Context, remotePath, localPath string) error {
	url := fmt.Sprintf("%s%s%s", s.Config.FilerURL, s.Config.BasePath, remotePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("download from %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0o750); err != nil {
		return fmt.Errorf("create directory for %s: %w", localPath, err)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local file %s: %w", localPath, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write to %s: %w", localPath, err)
	}

	return nil
}

// Delete removes a file from SeaweedFS Filer.
func (s *SeaweedFSClient) Delete(ctx context.Context, remotePath string) error {
	url := fmt.Sprintf("%s%s%s", s.Config.FilerURL, s.Config.BasePath, remotePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("delete %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Exists checks if a remote path exists on SeaweedFS Filer.
func (s *SeaweedFSClient) Exists(ctx context.Context, remotePath string) (bool, error) {
	url := fmt.Sprintf("%s%s%s", s.Config.FilerURL, s.Config.BasePath, remotePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, fmt.Errorf("create head request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("head %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK, nil
}
