package main

import (
	"context"
	"fmt"

	"dagger/ckodex-operator/internal/dagger"
	"golang.org/x/sync/errgroup"
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
	outputs := make([]string, len(platforms))
	g, groupCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return smokeTestHuggingFaceInitializer(groupCtx, source, "amd64")
	})
	for i, arch := range platforms {
		i, arch := i, arch
		g.Go(func() error {
			output, err := scanHuggingFaceInitializerArch(groupCtx, source, arch)
			if err != nil {
				return err
			}
			outputs[i] = fmt.Sprintf("%s:\n%s", arch, output)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return "", err
	}
	return outputs[0] + outputs[1], nil
}

// ScanHuggingFaceInitializerArch scans one initializer architecture natively.
//
// CI runs this function on matching amd64 and arm64 GitHub-hosted runners so
// each variant can exercise the initializer contract and import hf_xet natively.
func (m *CkodexOperator) ScanHuggingFaceInitializerArch(
	ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
	arch string,
) (string, error) {
	if arch != "amd64" && arch != "arm64" {
		return "", fmt.Errorf("unsupported Hugging Face initializer architecture %q", arch)
	}
	g, groupCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return smokeTestHuggingFaceInitializer(groupCtx, source, arch)
	})
	var output string
	g.Go(func() error {
		var err error
		output, err = scanHuggingFaceInitializerArch(groupCtx, source, arch)
		return err
	})
	if err := g.Wait(); err != nil {
		return "", err
	}
	return output, nil
}

func scanHuggingFaceInitializerArch(ctx context.Context, source *dagger.Directory, arch string) (string, error) {
	if arch != "amd64" && arch != "arm64" {
		return "", fmt.Errorf("unsupported Hugging Face initializer architecture %q", arch)
	}
	output, err := scanRootfs(buildHuggingFaceInitializerVariant(source, arch).Rootfs()).Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("scan %s Hugging Face initializer: %w", arch, err)
	}
	return output, nil
}

func smokeTestHuggingFaceInitializer(ctx context.Context, source *dagger.Directory, arch string) error {
	const destination = "/tmp/model"
	container := buildHuggingFaceInitializerVariant(source, arch).
		WithExec([]string{"python", "-c", "import hf_xet"}).
		WithMountedDirectory(destination, dag.Directory(), dagger.ContainerWithMountedDirectoryOpts{
			Owner: "65532:65532",
		}).
		WithExec(
			[]string{"hf://hf-internal-testing/tiny-random-gpt2", destination},
			dagger.ContainerWithExecOpts{UseEntrypoint: true},
		)
	if _, err := container.File(destination + "/config.json").Contents(ctx); err != nil {
		return fmt.Errorf("exercise KServe Hugging Face initializer contract: %w", err)
	}
	return nil
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
