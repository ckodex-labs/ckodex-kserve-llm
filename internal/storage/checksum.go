/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// HFFileInfo represents a single file entry from the HuggingFace API.
type HFFileInfo struct {
	Filename string     `json:"rfilename"`
	LFS      *HFLFSInfo `json:"lfs,omitempty"`
	OID      string     `json:"oid,omitempty"`
	Size     int64      `json:"size"`
}

// HFLFSInfo holds LFS pointer metadata returned by the HF API.
type HFLFSInfo struct {
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	PointerSize int64  `json:"pointerSize"`
}

// HFModelInfo represents the top-level model info response from the HF API.
type HFModelInfo struct {
	Siblings []HFFileInfo `json:"siblings"`
}

// ChecksumVerifier performs SHA256 verification of downloaded files against
// metadata from the HuggingFace Hub API.
type ChecksumVerifier struct {
	Token      string
	HTTPClient *http.Client
	BaseURL    string // defaults to "https://huggingface.co" when empty
}

// NewChecksumVerifier creates a verifier using the given HF token.
func NewChecksumVerifier(token string) *ChecksumVerifier {
	return NewChecksumVerifierWithMirror(token, "")
}

// NewChecksumVerifierWithMirror creates a verifier pointing at an optional mirror URL.
func NewChecksumVerifierWithMirror(token, mirrorURL string) *ChecksumVerifier {
	base := defaultHFBaseURL
	if mirrorURL != "" {
		base = strings.TrimRight(mirrorURL, "/")
	}
	return &ChecksumVerifier{
		Token:      token,
		HTTPClient: http.DefaultClient,
		BaseURL:    base,
	}
}

// FetchFileChecksums retrieves the SHA256 checksums for all files in a HF repo.
// It queries {baseURL}/api/models/{repo}?revision={revision}.
// Returns a map of filename -> sha256 hex string. Files without LFS metadata
// are omitted (they are small text files whose hash is not tracked by HF).
func (v *ChecksumVerifier) FetchFileChecksums(ctx context.Context, repo, revision string) (map[string]string, error) {
	url := fmt.Sprintf("%s/api/models/%s?revision=%s", v.BaseURL, repo, revision)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("checksum: failed to build request: %w", err)
	}
	if v.Token != "" {
		req.Header.Set("Authorization", "Bearer "+v.Token)
	}

	resp, err := v.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("checksum: failed to fetch model info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksum: HF API returned %s for %s", resp.Status, url)
	}

	var info HFModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("checksum: failed to decode model info: %w", err)
	}

	checksums := make(map[string]string)
	for _, sibling := range info.Siblings {
		if sibling.LFS != nil && sibling.LFS.SHA256 != "" {
			checksums[sibling.Filename] = strings.ToLower(sibling.LFS.SHA256)
		}
	}

	return checksums, nil
}

// VerifyFile computes the SHA256 of the file at filePath and compares it
// against the expected hex digest. Returns nil on match.
func VerifyFile(filePath, expectedSHA256 string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("checksum: cannot open %s: %w", filePath, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("checksum: failed to hash %s: %w", filePath, err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != strings.ToLower(expectedSHA256) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filePath, expectedSHA256, actual)
	}

	return nil
}

// VerifyDirectory walks destDir and verifies every file that has a known
// checksum in the provided map. Files not present in the map are skipped.
func VerifyDirectory(destDir string, checksums map[string]string) error {
	var mismatches []string

	for relPath, expected := range checksums {
		absPath := filepath.Join(destDir, relPath)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			// File not downloaded (maybe filtered); skip.
			continue
		}

		if err := VerifyFile(absPath, expected); err != nil {
			mismatches = append(mismatches, err.Error())
		}
	}

	if len(mismatches) > 0 {
		return fmt.Errorf("checksum verification failed:\n  %s", strings.Join(mismatches, "\n  "))
	}

	return nil
}

// ComputeSHA256 returns the hex-encoded SHA256 of the file at the given path.
func ComputeSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
