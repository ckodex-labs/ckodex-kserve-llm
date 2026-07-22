package main

import (
	"context"
	"fmt"

	"dagger/ckodex-operator/internal/dagger"
)

// BuildHuggingFaceInitializer builds the amd64 Xet-aware hf:// initializer.
//
// Usage: dagger call build-hugging-face-initializer --source=.
func (m *CkodexOperator) BuildHuggingFaceInitializer(
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) *dagger.Container {
	return buildHuggingFaceInitializerVariant(source, "amd64")
}

// ScanHuggingFaceInitializer fails on fixed HIGH or CRITICAL vulnerabilities.
//
// Usage: dagger call scan-hugging-face-initializer --source=.
func (m *CkodexOperator) ScanHuggingFaceInitializer(
	ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	platforms := []string{"amd64", "arm64"}
	result := ""
	for _, arch := range platforms {
		output, err := scanRootfs(buildHuggingFaceInitializerVariant(source, arch).Rootfs()).Stdout(ctx)
		if err != nil {
			return result, fmt.Errorf("scan %s Hugging Face initializer: %w", arch, err)
		}
		result += fmt.Sprintf("%s:\n%s", arch, output)
	}
	return result, nil
}

// PublishHuggingFaceInitializer builds and publishes amd64 and arm64 images.
//
// Usage: dagger call publish-hugging-face-initializer --source=. \
//
//	--image-ref=ghcr.io/org/app-huggingface-initializer:v1.0.0 \
//	--registry-username=user --registry-token=env:GITHUB_TOKEN
func (m *CkodexOperator) PublishHuggingFaceInitializer(
	ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
	imageRef string,
	registryUsername string,
	registryToken *dagger.Secret,
) (string, error) {
	platforms := []string{"amd64", "arm64"}
	variants := make([]*dagger.Container, 0, len(platforms))
	for _, arch := range platforms {
		variants = append(variants, buildHuggingFaceInitializerVariant(source, arch))
	}

	digest, err := dag.Container().
		WithRegistryAuth("ghcr.io", registryUsername, registryToken).
		Publish(ctx, imageRef, dagger.ContainerPublishOpts{PlatformVariants: variants})
	if err != nil {
		return "", fmt.Errorf("publish Hugging Face initializer %s: %w", imageRef, err)
	}
	return digest, nil
}

func buildHuggingFaceInitializerVariant(source *dagger.Directory, arch string) *dagger.Container {
	return filteredSource(source).DockerBuild(dagger.DirectoryDockerBuildOpts{
		Platform: dagger.Platform("linux/" + arch),
		Target:   "huggingface-initializer",
	})
}
