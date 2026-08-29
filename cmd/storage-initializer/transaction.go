/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	stagingDirectory = ".ckodex-staging"
	stagingStateFile = ".ckodex-staging-state.json"
)

type stagingState struct {
	Subject string   `json:"subject"`
	Phase   string   `json:"phase"`
	Files   []string `json:"files,omitempty"`
}

func prepareStagingDirectory(destPath, uri string) (string, func(), error) {
	entries, err := os.ReadDir(destPath)
	if err != nil {
		return "", nil, fmt.Errorf("inspect destination %s: %w", destPath, err)
	}
	if len(entries) > 0 {
		if err := recoverStagingTransaction(destPath, uri, entries); err != nil {
			return "", nil, err
		}
		entries, err = os.ReadDir(destPath)
		if err != nil {
			return "", nil, fmt.Errorf("recheck destination %s: %w", destPath, err)
		}
	}
	if len(entries) > 0 {
		return "", nil, fmt.Errorf("destination %s already contains files, but no matching verified cache record was found; refusing to reuse unverified content", destPath)
	}

	stagingPath := filepath.Join(destPath, stagingDirectory)
	if err := os.Mkdir(stagingPath, 0o750); err != nil {
		return "", nil, fmt.Errorf("create staging directory: %w", err)
	}
	if err := writeStagingState(stagingPath, stagingState{Subject: uri, Phase: "downloading"}); err != nil {
		_ = os.RemoveAll(stagingPath)
		return "", nil, fmt.Errorf("record staging transaction: %w", err)
	}
	return stagingPath, func() { _ = os.RemoveAll(stagingPath) }, nil
}

func recoverStagingTransaction(destPath, uri string, entries []os.DirEntry) error {
	var stagingEntry os.DirEntry
	for _, entry := range entries {
		if entry.Name() == stagingDirectory {
			stagingEntry = entry
		}
	}
	if stagingEntry == nil || !stagingEntry.IsDir() {
		return nil
	}

	stagingPath := filepath.Join(destPath, stagingDirectory)
	state, err := loadStagingState(stagingPath)
	if err != nil || state.Subject != uri {
		return fmt.Errorf("destination %s contains an unrecognized staging transaction; refusing recovery", destPath)
	}
	if state.Phase != "downloading" && state.Phase != "committing" {
		return fmt.Errorf("destination %s contains a staging transaction with an invalid phase; refusing recovery", destPath)
	}
	allowed := make(map[string]struct{}, len(state.Files))
	for _, name := range state.Files {
		allowed[name] = struct{}{}
	}
	for _, entry := range entries {
		if entry.Name() == stagingDirectory {
			continue
		}
		if state.Phase != "committing" {
			return fmt.Errorf("destination %s contains files alongside an active staging transaction; refusing recovery", destPath)
		}
		if _, ok := allowed[entry.Name()]; !ok {
			return fmt.Errorf("destination %s contains an unexpected file during staging recovery; refusing recovery", destPath)
		}
		if err := os.RemoveAll(filepath.Join(destPath, entry.Name())); err != nil {
			return fmt.Errorf("remove incomplete staged payload %s: %w", entry.Name(), err)
		}
	}
	if err := os.RemoveAll(stagingPath); err != nil {
		return fmt.Errorf("remove stale staging transaction: %w", err)
	}
	return nil
}

func writeStagingState(stagingPath string, state stagingState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stagingPath, stagingStateFile), append(data, '\n'), 0o600)
}

func loadStagingState(stagingPath string) (stagingState, error) {
	file, err := os.OpenInRoot(stagingPath, stagingStateFile)
	if err != nil {
		return stagingState{}, err
	}
	data, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil {
		return stagingState{}, err
	}
	if closeErr != nil {
		return stagingState{}, closeErr
	}
	var state stagingState
	if err := json.Unmarshal(data, &state); err != nil {
		return stagingState{}, err
	}
	return state, nil
}

func commitStagingDirectory(stagingPath, destPath, uri string) error {
	entries, err := os.ReadDir(stagingPath)
	if err != nil {
		return err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() != stagingStateFile {
			files = append(files, entry.Name())
		}
	}
	if err := writeStagingState(stagingPath, stagingState{Subject: uri, Phase: "committing", Files: files}); err != nil {
		return err
	}
	for _, name := range files {
		if err := os.Rename(filepath.Join(stagingPath, name), filepath.Join(destPath, name)); err != nil {
			return fmt.Errorf("move %s: %w", name, err)
		}
	}
	return nil
}

func directoryDigest(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() && entry.Name() == verificationStateDir {
			return fs.SkipDir
		}
		if entry.Name() == stagingStateFile {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed in model payload: %s", rel)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if _, err := io.WriteString(hash, filepath.ToSlash(rel)+"\x00"); err != nil {
			return err
		}
		file, err := os.OpenInRoot(root, rel)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		_, err = io.WriteString(hash, "\x00")
		return err
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
