/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client pulls models from Amazon S3.
type S3Client struct {
	client *s3.Client
}

// NewS3Client creates a new S3Client.
// Reads S3_ENDPOINT (or AWS_ENDPOINT_URL) and AWS_NO_SIGN_REQUEST from the environment,
// enabling anonymous path-style access to SeaweedFS without code changes.
func NewS3Client(ctx context.Context, region string) (*S3Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, s3ConfigOptions(region)...)
	if err != nil {
		return nil, err
	}
	return &S3Client{client: s3.NewFromConfig(cfg, s3PathStyleOption())}, nil
}

// s3ConfigOptions returns aws config load options derived from environment variables.
// Precedence: AWS_ENDPOINT_URL > S3_ENDPOINT (KServe convention).
func s3ConfigOptions(region string) []func(*config.LoadOptions) error {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	// Anonymous credentials when AWS_NO_SIGN_REQUEST is set (SeaweedFS anonymous mode).
	if v := os.Getenv("AWS_NO_SIGN_REQUEST"); v == "yes" || v == "1" || v == "true" {
		opts = append(opts, config.WithCredentialsProvider(aws.AnonymousCredentials{}))
	}

	// Custom endpoint: AWS_ENDPOINT_URL takes precedence, then S3_ENDPOINT.
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		endpoint = os.Getenv("S3_ENDPOINT")
	}
	if endpoint != "" {
		opts = append(opts, config.WithBaseEndpoint(endpoint))
	}

	return opts
}

// s3PathStyleOption forces path-style addressing, required for SeaweedFS and
// any S3-compatible server that does not support virtual-hosted buckets.
func s3PathStyleOption() func(*s3.Options) {
	return func(o *s3.Options) {
		o.UsePathStyle = true
	}
}

func init() {
	RegisterClient(&S3Client{})
}

func (c *S3Client) Schemes() []string {
	return []string{"s3"}
}

// Pull downloads artifacts from an S3 bucket.
// URI format: s3://[bucket]/[path]
func (c *S3Client) Pull(ctx context.Context, uri string, destPath string) error {
	ref := strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return fmt.Errorf("invalid S3 URI format, expected s3://bucket/path: %s", uri)
	}
	bucket, key := parts[0], parts[1]

	client := c.client
	if client == nil {
		// Lazy-init: read region, endpoint, and signing settings from environment.
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-east-1"
		}
		cfg, err := config.LoadDefaultConfig(ctx, s3ConfigOptions(region)...)
		if err != nil {
			return fmt.Errorf("failed to load S3 config: %w", err)
		}
		client = s3.NewFromConfig(cfg, s3PathStyleOption())
	}

	downloader := manager.NewDownloader(client) // nolint:staticcheck

	// For S3 "folder" download, we need to list objects first
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(key),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list S3 objects: %w", err)
		}

		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}

			// Relative path within the destination
			relPath := strings.TrimPrefix(*obj.Key, key)
			if relPath == "" && !strings.HasSuffix(key, "/") {
				// Single file download
				relPath = filepath.Base(key)
			}

			destFile := filepath.Join(destPath, relPath)
			if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
				return err
			}

			f, err := os.Create(destFile)
			if err != nil {
				return err
			}

			fmt.Printf("Downloading s3://%s/%s to %s\n", bucket, *obj.Key, destFile)
			_, err = downloader.Download(ctx, f, &s3.GetObjectInput{ // nolint:staticcheck
				Bucket: aws.String(bucket),
				Key:    obj.Key,
			})
			_ = f.Close()
			if err != nil {
				return fmt.Errorf("failed to download s3://%s/%s: %w", bucket, *obj.Key, err)
			}
		}
	}

	return nil
}
