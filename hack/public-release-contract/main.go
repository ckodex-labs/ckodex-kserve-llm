/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	ociIndexMediaType     = "application/vnd.oci.image.index.v1+json"
	dockerIndexMediaType  = "application/vnd.docker.distribution.manifest.list.v2+json"
	ociManifestMediaType  = "application/vnd.oci.image.manifest.v1+json"
	helmConfigMediaType   = "application/vnd.cncf.helm.config.v1+json"
	helmContentMediaType  = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
	defaultRequestTimeout = 2 * time.Minute
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type configuration struct {
	repository      string
	chartRepository string
	version         string
	operatorDigest  string
	initializerHash string
	plainHTTP       bool
}

type verifier struct {
	config configuration
}

type manifest struct {
	MediaType string       `json:"mediaType"`
	Manifests []descriptor `json:"manifests"`
	Config    descriptor   `json:"config"`
	Layers    []descriptor `json:"layers"`
}

type descriptor struct {
	MediaType string    `json:"mediaType"`
	Platform  *platform `json:"platform,omitempty"`
}

type platform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "public release contract:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	config, err := parseConfiguration(args)
	if err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, defaultRequestTimeout)
	defer cancel()
	checker := verifier{config: config}
	if err := checker.verifyContainer(requestCtx, config.repository, config.operatorDigest); err != nil {
		return fmt.Errorf("operator image: %w", err)
	}
	if err := checker.verifyChart(requestCtx); err != nil {
		return fmt.Errorf("helm chart: %w", err)
	}
	initializer := config.repository + "-huggingface-initializer"
	if err := checker.verifyContainer(requestCtx, initializer, config.initializerHash); err != nil {
		return fmt.Errorf("hugging face initializer image: %w", err)
	}
	if _, err := fmt.Fprintf(output, "public release contract passed for %s\n", config.version); err != nil {
		return fmt.Errorf("write success output: %w", err)
	}
	return nil
}

func parseConfiguration(args []string) (configuration, error) {
	var config configuration
	flags := flag.NewFlagSet("public-release-contract", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&config.repository, "repository", "", "operator OCI repository")
	flags.StringVar(&config.chartRepository, "chart-repository", "", "Helm chart OCI repository")
	flags.StringVar(&config.version, "version", "", "release tag")
	flags.StringVar(&config.operatorDigest, "operator-digest", "", "published operator digest")
	flags.StringVar(&config.initializerHash, "initializer-digest", "", "published initializer digest")
	flags.BoolVar(&config.plainHTTP, "plain-http", false, "use HTTP for a local test registry")
	if err := flags.Parse(args); err != nil {
		return configuration{}, err
	}
	if err := validateConfiguration(config); err != nil {
		return configuration{}, err
	}
	return config, nil
}

func validateConfiguration(config configuration) error {
	for name, repository := range map[string]string{
		"repository":       config.repository,
		"chart-repository": config.chartRepository,
	} {
		if !validRepository(repository) {
			return fmt.Errorf("%s %q must include a registry and repository path", name, repository)
		}
	}
	if !strings.HasPrefix(config.version, "v") || len(config.version) == 1 {
		return fmt.Errorf("version %q must start with v", config.version)
	}
	for name, digest := range map[string]string{
		"operator-digest":    config.operatorDigest,
		"initializer-digest": config.initializerHash,
	} {
		if !digestPattern.MatchString(digest) {
			return fmt.Errorf("%s %q is not a sha256 digest", name, digest)
		}
	}
	return nil
}

func validRepository(repository string) bool {
	parts := strings.Split(repository, "/")
	return len(parts) >= 3 &&
		strings.Contains(parts[0], ".") &&
		!strings.Contains(repository, "://")
}

func (v verifier) verifyContainer(ctx context.Context, repository, expectedDigest string) error {
	index, digest, err := v.fetchManifest(ctx, repository, v.config.version)
	if err != nil {
		return err
	}
	if digest != expectedDigest {
		return fmt.Errorf("tag resolves to %s, want %s", digest, expectedDigest)
	}
	if index.MediaType != ociIndexMediaType && index.MediaType != dockerIndexMediaType {
		return fmt.Errorf("media type %q is not a multi-platform index", index.MediaType)
	}
	for _, architecture := range []string{"amd64", "arm64"} {
		if !hasLinuxPlatform(index.Manifests, architecture) {
			return fmt.Errorf("multi-platform index has no linux/%s manifest", architecture)
		}
	}
	return nil
}

func (v verifier) verifyChart(ctx context.Context) error {
	version := strings.TrimPrefix(v.config.version, "v")
	chart, _, err := v.fetchManifest(ctx, v.config.chartRepository, version)
	if err != nil {
		return err
	}
	if chart.MediaType != ociManifestMediaType {
		return fmt.Errorf("media type %q is not an OCI manifest", chart.MediaType)
	}
	if chart.Config.MediaType != helmConfigMediaType {
		return fmt.Errorf("config media type %q is not a Helm config", chart.Config.MediaType)
	}
	for _, layer := range chart.Layers {
		if layer.MediaType == helmContentMediaType {
			return nil
		}
	}
	return errors.New("manifest has no Helm chart content layer")
}

func (v verifier) fetchManifest(
	ctx context.Context,
	repository string,
	reference string,
) (manifest, string, error) {
	remoteRepository, err := anonymousRepository(repository, v.config.plainHTTP)
	if err != nil {
		return manifest{}, "", err
	}
	resolved, err := remoteRepository.Resolve(ctx, reference)
	if err != nil {
		return manifest{}, "", fmt.Errorf("resolve %s anonymously: %w", reference, err)
	}
	content, err := remoteRepository.Fetch(ctx, resolved)
	if err != nil {
		return manifest{}, "", fmt.Errorf("fetch %s anonymously: %w", reference, err)
	}
	defer func() {
		_ = content.Close()
	}()
	var document manifest
	if err := json.NewDecoder(content).Decode(&document); err != nil {
		return manifest{}, "", fmt.Errorf("decode manifest: %w", err)
	}
	if document.MediaType == "" {
		document.MediaType = resolved.MediaType
	}
	return document, resolved.Digest.String(), nil
}

func anonymousRepository(reference string, plainHTTP bool) (*remote.Repository, error) {
	repository, err := remote.NewRepository(reference)
	if err != nil {
		return nil, fmt.Errorf("create OCI repository %s: %w", reference, err)
	}
	repository.PlainHTTP = plainHTTP
	repository.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Credential: auth.StaticCredential(repository.Reference.Registry, auth.EmptyCredential),
	}
	return repository, nil
}

func hasLinuxPlatform(manifests []descriptor, architecture string) bool {
	for _, item := range manifests {
		if item.Platform != nil &&
			item.Platform.OS == "linux" &&
			item.Platform.Architecture == architecture {
			return true
		}
	}
	return false
}
