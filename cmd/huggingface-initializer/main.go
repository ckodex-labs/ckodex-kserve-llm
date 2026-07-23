/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const defaultRevision = "main"

func main() {
	args, err := normalizeArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	hfPath, err := exec.LookPath("hf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "find hf executable: %v\n", err)
		os.Exit(1)
	}
	if err := syscall.Exec(hfPath, append([]string{hfPath}, args...), os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "execute hf: %v\n", err)
		os.Exit(1)
	}
}

// normalizeArgs accepts KServe's custom storage initializer contract:
//
//	<source-uri> <destination-path>
//
// Other argument forms pass through to the hf CLI for diagnostic use and
// compatibility with pods created by older operator releases.
func normalizeArgs(args []string) ([]string, error) {
	if len(args) != 2 {
		return args, nil
	}
	if !isHuggingFaceURI(args[0]) {
		if strings.Contains(args[0], "://") {
			return nil, fmt.Errorf("unsupported source URI %q; expected hf://", args[0])
		}
		return args, nil
	}

	repo, revision := parseHuggingFaceURI(args[0])
	if repo == "" {
		return nil, fmt.Errorf("Hugging Face source URI must include a repository")
	}
	if args[1] == "" {
		return nil, fmt.Errorf("Hugging Face destination path must not be empty")
	}

	return []string{
		"download", repo,
		"--revision", revision,
		"--local-dir", args[1],
	}, nil
}

func isHuggingFaceURI(uri string) bool {
	return strings.HasPrefix(uri, "hf://") ||
		strings.HasPrefix(uri, "hf-mirror://")
}

func parseHuggingFaceURI(uri string) (repo, revision string) {
	for _, prefix := range []string{"hf://", "hf-mirror://"} {
		if strings.HasPrefix(uri, prefix) {
			repo = strings.TrimPrefix(uri, prefix)
			break
		}
	}

	revision = defaultRevision
	if idx := strings.LastIndex(repo, "@"); idx >= 0 {
		repo, revision = repo[:idx], repo[idx+1:]
		if revision == "" {
			revision = defaultRevision
		}
	}
	return repo, revision
}
