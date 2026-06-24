/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jfrog/jfrog-client-go/artifactory"
	artifactoryauth "github.com/jfrog/jfrog-client-go/artifactory/auth"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/config"
)

// ArtifactoryClient pulls models from JFrog Artifactory.
type ArtifactoryClient struct {
	manager artifactory.ArtifactoryServicesManager
}

// NewArtifactoryClient creates a new ArtifactoryClient.
func NewArtifactoryClient(url, user, password string) (*ArtifactoryClient, error) {
	// In some versions of jfrog-client-go, it's NewArtifactoryDetails, in others it might change.
	// We'll use the common setters approach if available or a similar factory.
	rtDetails := artifactoryauth.NewArtifactoryDetails()
	rtDetails.SetUrl(url)
	rtDetails.SetUser(user)
	rtDetails.SetPassword(password)

	cfg, err := config.NewConfigBuilder().
		SetServiceDetails(rtDetails).
		Build()
	if err != nil {
		return nil, err
	}

	m, err := artifactory.New(cfg)
	if err != nil {
		return nil, err
	}

	return &ArtifactoryClient{manager: m}, nil
}

func init() {
	// Register with a dummy manager for scheme detection.
	RegisterClient(&ArtifactoryClient{})
}

func (c *ArtifactoryClient) Schemes() []string {
	return []string{"artifactory"}
}

// Pull downloads artifacts from Artifactory.
// URI format: artifactory://[host]/[repo-key]/[path]
func (c *ArtifactoryClient) Pull(ctx context.Context, uri string, destPath string) error {
	ref := strings.TrimPrefix(uri, "artifactory://")

	// Split host, repo and path
	parts := strings.SplitN(ref, "/", 3)
	if len(parts) < 3 {
		return fmt.Errorf("invalid Artifactory URI format, expected artifactory://host/repo/path: %s", uri)
	}
	host, repo, path := parts[0], parts[1], parts[2]

	manager := c.manager
	if manager == nil {
		user := os.Getenv("ARTIFACTORY_USER")
		password := os.Getenv("ARTIFACTORY_PASSWORD")
		url := fmt.Sprintf("https://%s/artifactory", host)

		rtDetails := artifactoryauth.NewArtifactoryDetails()
		rtDetails.SetUrl(url)
		rtDetails.SetUser(user)
		rtDetails.SetPassword(password)

		cfg, err := config.NewConfigBuilder().
			SetServiceDetails(rtDetails).
			Build()
		if err != nil {
			return err
		}

		manager, err = artifactory.New(cfg)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Pulling Artifactory artifact from %s/%s to %s\n", repo, path, destPath)

	params := services.NewDownloadParams()
	params.Pattern = fmt.Sprintf("%s/%s", repo, path)
	params.Target = destPath
	params.Flat = true

	_, _, err := manager.DownloadFiles(params)
	return err
}
