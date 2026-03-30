/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hupe1980/go-huggingface"
)

const (
	// defaultHFBaseURL is the public HuggingFace Hub base URL.
	defaultHFBaseURL = "https://huggingface.co"
)

// HuggingFaceClient pulls models from the Hugging Face Hub.
// Set baseURL to point at an internal mirror for air-gapped environments.
type HuggingFaceClient struct {
	client  *huggingface.InferenceClient
	baseURL string // e.g. "https://hf-mirror.corp.internal", defaults to defaultHFBaseURL
}

// NewHuggingFaceClient creates a client pointing at the public HuggingFace Hub.
func NewHuggingFaceClient(token string) *HuggingFaceClient {
	return NewHuggingFaceClientWithMirror(token, "")
}

// NewHuggingFaceClientWithMirror creates a client with an optional internal mirror URL.
// When mirrorURL is empty the public https://huggingface.co is used.
// mirrorURL must include scheme, e.g. "https://hf-mirror.corp.internal".
func NewHuggingFaceClientWithMirror(token, mirrorURL string) *HuggingFaceClient {
	base := strings.TrimRight(defaultHFBaseURL, "/")
	if mirrorURL != "" {
		base = strings.TrimRight(mirrorURL, "/")
	}
	return &HuggingFaceClient{
		client:  huggingface.NewInferenceClient(token),
		baseURL: base,
	}
}

func init() {
	// Automatically register if we have a default client or via a factory.
	RegisterClient(&HuggingFaceClient{baseURL: defaultHFBaseURL})
}

func (c *HuggingFaceClient) Schemes() []string {
	return []string{"hf", "huggingface"}
}

// Pull downloads a model repository from Hugging Face.
// URI format: hf://[org]/[repo](@branch)
//
// Features:
//   - Fetches the full file tree from the HF API
//   - Downloads all model files (parallel chunked for files > 1 GB)
//   - Verifies SHA256 checksums from HF LFS metadata (disable with --skip-checksum or SKIP_CHECKSUM=1)
func (c *HuggingFaceClient) Pull(ctx context.Context, uri string, destPath string) error {
	ref := strings.TrimPrefix(strings.TrimPrefix(uri, "hf://"), "huggingface://")

	token := os.Getenv("HF_TOKEN")

	// Split repo from branch/revision
	revision := "main"
	repo := ref
	if idx := strings.Index(ref, "@"); idx >= 0 {
		repo = ref[:idx]
		revision = ref[idx+1:]
	}

	fmt.Printf("Pulling Hugging Face model %s (revision: %s) to %s\n", repo, revision, destPath)

	skipChecksum := os.Getenv("SKIP_CHECKSUM") == "1" || os.Getenv("SKIP_CHECKSUM") == "true"

	// Fetch the full file list from the HF API.
	files, err := c.listRepoFiles(ctx, repo, revision, token)
	if err != nil {
		return fmt.Errorf("failed to list repo files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no files found in repo %s at revision %s", repo, revision)
	}

	// Fetch checksums for verification.
	var checksums map[string]string
	if !skipChecksum {
		verifier := NewChecksumVerifierWithMirror(token, c.baseURL)
		checksums, err = verifier.FetchFileChecksums(ctx, repo, revision)
		if err != nil {
			fmt.Printf("Warning: could not fetch checksums, skipping verification: %v\n", err)
			checksums = nil
		}
	} else {
		fmt.Printf("Checksum verification disabled (SKIP_CHECKSUM)\n")
	}

	// Create the parallel downloader.
	downloader := NewParallelDownloader(token)

	// Download each file.
	for _, file := range files {
		fileURL := fmt.Sprintf("%s/%s/resolve/%s/%s", c.baseURL, repo, revision, file)
		destFile := filepath.Join(destPath, file)

		fmt.Printf("Downloading: %s\n", file)

		expectedHash := ""
		if checksums != nil {
			expectedHash = checksums[file]
		}

		if err := downloader.DownloadFile(ctx, fileURL, destFile, expectedHash); err != nil {
			return fmt.Errorf("failed to download %s: %w", file, err)
		}
	}

	// Final directory-level verification for any files that had checksums
	// but were downloaded via the simple (non-parallel) path without inline
	// verification (i.e., no LFS hash available at download time).
	if checksums != nil && !skipChecksum {
		if err := VerifyDirectory(destPath, checksums); err != nil {
			return err
		}
		fmt.Printf("All checksums verified successfully\n")
	}

	fmt.Printf("Successfully downloaded %d files from %s\n", len(files), repo)
	return nil
}

// HFTreeEntry represents one entry from the HF API tree endpoint.
type HFTreeEntry struct {
	Type string `json:"type"` // "file" or "directory"
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// listRepoFiles queries the HF API to enumerate all files in a repository.
func (c *HuggingFaceClient) listRepoFiles(ctx context.Context, repo, revision, token string) ([]string, error) {
	url := fmt.Sprintf("%s/api/models/%s/tree/%s", c.baseURL, repo, revision)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HF API tree returned %s for %s", resp.Status, url)
	}

	var entries []HFTreeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode HF tree response: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.Type == "file" {
			files = append(files, e.Path)
		}
	}

	return files, nil
}
