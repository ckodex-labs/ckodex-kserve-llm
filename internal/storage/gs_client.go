/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// GSClient pulls models from Google Cloud Storage.
type GSClient struct {
	client *storage.Client
}

// NewGSClient creates a new GSClient.
func NewGSClient(ctx context.Context) (*GSClient, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &GSClient{client: client}, nil
}

func init() {
	RegisterClient(&GSClient{})
}

func (c *GSClient) Schemes() []string {
	return []string{"gs"}
}

// Pull downloads artifacts from a GS bucket.
// URI format: gs://[bucket]/[path]
func (c *GSClient) Pull(ctx context.Context, uri string, destPath string) error {
	ref := strings.TrimPrefix(uri, "gs://")
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return fmt.Errorf("invalid GS URI format, expected gs://bucket/path: %s", uri)
	}
	bucketName, prefix := parts[0], parts[1]

	client := c.client
	if client == nil {
		var err error
		client, err = storage.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create GS client: %w", err)
		}
	}

	bucket := client.Bucket(bucketName)
	it := bucket.Objects(ctx, &storage.Query{Prefix: prefix})

	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to list GS objects: %w", err)
		}

		// Relative path within the destination
		relPath := strings.TrimPrefix(attrs.Name, prefix)
		if relPath == "" && !strings.HasSuffix(prefix, "/") {
			// Single file download
			relPath = filepath.Base(prefix)
		}

		destFile := filepath.Join(destPath, relPath)
		if strings.HasSuffix(attrs.Name, "/") {
			// It's a directory (simulated in GS)
			if err := os.MkdirAll(destFile, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
			return err
		}

		fmt.Printf("Downloading gs://%s/%s to %s\n", bucketName, attrs.Name, destFile)
		if err := c.downloadObject(ctx, bucket, attrs.Name, destFile); err != nil {
			return err
		}
	}

	return nil
}

func (c *GSClient) downloadObject(ctx context.Context, bucket *storage.BucketHandle, name, destFile string) error {
	rc, err := bucket.Object(name).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to create GS reader for %s: %w", name, err)
	}
	defer func() { _ = rc.Close() }()

	f, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, rc)
	return err
}
