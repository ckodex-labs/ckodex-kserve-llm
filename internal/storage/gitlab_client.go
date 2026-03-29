/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// GitLabClient pulls models from GitLab repositories.
type GitLabClient struct {
	client *gitlab.Client
}

// NewGitLabClient creates a new GitLabClient.
func NewGitLabClient(token string) (*GitLabClient, error) {
	c, err := gitlab.NewClient(token)
	if err != nil {
		return nil, err
	}
	return &GitLabClient{client: c}, nil
}

func init() {
	// Register with a default empty client for scheme detection.
	c, _ := gitlab.NewClient("")
	RegisterClient(&GitLabClient{client: c})
}

func (c *GitLabClient) Schemes() []string {
	return []string{"gitlab"}
}

// Pull downloads artifacts from a GitLab repository.
// URI format: gitlab://[project_id_or_path]/[path](@ref)
func (c *GitLabClient) Pull(ctx context.Context, uri string, destPath string) error {
	ref := strings.TrimPrefix(uri, "gitlab://")

	token := os.Getenv("GITLAB_TOKEN")
	client, err := gitlab.NewClient(token)
	if err != nil {
		return fmt.Errorf("failed to create GitLab client: %w", err)
	}

	// Split project_id/path and revision
	revision := "main"
	if idx := strings.Index(ref, "@"); idx >= 0 {
		revision = ref[idx+1:]
		ref = ref[:idx]
	}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return fmt.Errorf("invalid GitLab URI format, expected gitlab://project/path: %s", uri)
	}
	projectID, path := parts[0], parts[1]

	return c.pullRecursive(ctx, client, projectID, path, revision, destPath)
}

func (c *GitLabClient) pullRecursive(ctx context.Context, client *gitlab.Client, projectID, path, revision, destPath string) error {
	// Try to get as file first
	file, resp, err := client.RepositoryFiles.GetFile(projectID, path, &gitlab.GetFileOptions{
		Ref: gitlab.Ptr(revision),
	})

	if err == nil && file != nil {
		// Single file - use GetRawFile for efficiency
		data, _, err := client.RepositoryFiles.GetRawFile(projectID, path, &gitlab.GetRawFileOptions{
			Ref: gitlab.Ptr(revision),
		})
		if err != nil {
			return err
		}

		destFile := filepath.Join(destPath, filepath.Base(path))
		if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
			return err
		}
		return os.WriteFile(destFile, data, 0644)
	}

	// If not a file, try as directory (list tree)
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		// Might be a directory, list tree recursively
		nodes, _, err := client.Repositories.ListTree(projectID, &gitlab.ListTreeOptions{
			Path:      gitlab.Ptr(path),
			Ref:       gitlab.Ptr(revision),
			Recursive: gitlab.Ptr(true),
		})
		if err != nil {
			return err
		}

		for _, node := range nodes {
			if node.Type == "blob" {
				data, _, err := client.RepositoryFiles.GetRawFile(projectID, node.Path, &gitlab.GetRawFileOptions{
					Ref: gitlab.Ptr(revision),
				})
				if err != nil {
					return err
				}
				destFile := filepath.Join(destPath, node.Path)
				if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
					return err
				}
				if err := os.WriteFile(destFile, data, 0644); err != nil {
					return err
				}
			}
		}
		return nil
	}

	return err
}
