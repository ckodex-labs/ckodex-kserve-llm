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

	"github.com/google/go-github/v62/github"
)

// GitHubClient pulls models from GitHub repositories or releases.
type GitHubClient struct {
	client *github.Client
}

// NewGitHubClient creates a new GitHubClient.
func NewGitHubClient(_ context.Context, _ string) *GitHubClient {
	// The token is currently handled in Pull() via GITHUB_TOKEN env var.
	// This constructor can be expanded if needed for explicit OAuth2 clients.
	return &GitHubClient{
		client: github.NewClient(nil),
	}
}

func init() {
	RegisterClient(&GitHubClient{
		client: github.NewClient(nil),
	})
}

func (c *GitHubClient) Schemes() []string {
	return []string{"github"}
}

// Pull downloads artifacts from a GitHub repository.
// URI format: github://[owner]/[repo]/[path](@ref)
func (c *GitHubClient) Pull(ctx context.Context, uri string, destPath string) error {
	ref := strings.TrimPrefix(uri, "github://")

	token := os.Getenv("GITHUB_TOKEN")
	client := c.client
	if token != "" {
		client = github.NewClient(nil).WithAuthToken(token)
	}

	// Split owner/repo/path and revision
	revision := "main"
	if idx := strings.Index(ref, "@"); idx >= 0 {
		revision = ref[idx+1:]
		ref = ref[:idx]
	}

	parts := strings.SplitN(ref, "/", 3)
	if len(parts) < 3 {
		return fmt.Errorf("invalid GitHub URI format, expected github://owner/repo/path: %s", uri)
	}
	owner, repo, path := parts[0], parts[1], parts[2]

	return c.pullRecursive(ctx, client, owner, repo, path, revision, destPath)
}

func (c *GitHubClient) pullRecursive(ctx context.Context, client *github.Client, owner, repo, path, revision, destPath string) error {
	fc, dc, _, err := client.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{
		Ref: revision,
	})
	if err != nil {
		return err
	}

	if fc != nil {
		// Single file
		return c.downloadFile(ctx, *fc.DownloadURL, filepath.Join(destPath, filepath.Base(path)))
	}

	// Directory
	for _, item := range dc {
		if item.Type != nil && *item.Type == "dir" {
			if err := c.pullRecursive(ctx, client, owner, repo, *item.Path, revision, destPath); err != nil {
				return err
			}
		} else if item.Type != nil && *item.Type == "file" {
			if err := c.downloadFile(ctx, *item.DownloadURL, filepath.Join(destPath, *item.Path)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *GitHubClient) downloadFile(ctx context.Context, downloadURL, destFile string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download from GitHub: %s", resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
		return err
	}

	out, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}
