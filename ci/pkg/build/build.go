// Package build contains the Dagger image build stage.
package build

import (
	"context"
	"fmt"
	"os"
	"strings"

	"dagger.io/dagger"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
)

// Build builds the operator image for the configured platforms.
func Build(ctx context.Context, p *core.Pipeline) (string, error) {
	platforms := []dagger.Platform{"linux/amd64", "linux/arm64"}

	// 1. Determine image reference (Contract: REGISTRY)
	ref := p.Cfg.ImageRef
	if ref == "" {
		if p.Cfg.Registry != "" {
			tag := p.Cfg.Version
			if tag == "" {
				tag = "dev"
			}
			ref = fmt.Sprintf("%s/ckodex-kserve-llm:%s", p.Cfg.Registry, tag)
		} else {
			ref = "ckodex-kserve-llm:local"
		}
	}

	// 2. Prepare build ldflags (Contract: VERSION)
	version := p.Cfg.Version
	if version == "" {
		version = "dev"
	}
	ldflags := fmt.Sprintf("-s -w -extldflags '-static' -X github.com/ckodex-labs/kserve-llm-operator/internal/version.Version=%s", version)

	// Build one container per platform.
	variants := make([]*dagger.Container, 0, len(platforms))
	for _, platform := range platforms {
		arch := strings.Split(string(platform), "/")[1]
		binary := p.GoBase().
			WithEnvVariable("CGO_ENABLED", "0").
			WithEnvVariable("GOOS", "linux").
			WithEnvVariable("GOARCH", arch).
			WithExec([]string{
				"go", "build",
				"-a",
				"-ldflags", ldflags,
				"-o", "/out/manager",
				"cmd/manager/main.go",
			}).
			File("/out/manager")

		variant := p.Client.Container(dagger.ContainerOpts{Platform: platform}).
			From(core.DistrolessImage).
			WithFile("/manager", binary).
			WithUser("65532:65532").
			WithEntrypoint([]string{"/manager"})

		variants = append(variants, variant)
	}

	// 3. Publish or Export (Contract: REGISTRY)
	if p.Cfg.Push && p.Cfg.Registry != "" {
		digest, err := p.Client.Container().
			Publish(ctx, ref, dagger.ContainerPublishOpts{
				PlatformVariants: variants,
			})
		if err != nil {
			return digest, err
		}
		if mkErr := os.MkdirAll("bin", 0o750); mkErr != nil {
			return digest, fmt.Errorf("prepare bin directory: %w", mkErr)
		}
		if writeErr := os.WriteFile("bin/image-digest.txt", []byte(digest), 0o600); writeErr != nil {
			return digest, fmt.Errorf("write image digest: %w", writeErr)
		}
		return digest, err
	}

	// Export tarball if no registry (Contract)
	if p.Cfg.Registry == "" && !p.Cfg.Push {
		tarPath := "bin/ckodex-kserve-llm.tar"
		if _, err := p.Client.Container().
			Export(ctx, tarPath, dagger.ContainerExportOpts{
				PlatformVariants: variants,
			}); err != nil {
			return "", fmt.Errorf("export tarball: %w", err)
		}
		return tarPath, nil
	}

	// Default: sync to catch errors
	_, err := variants[0].Sync(ctx)
	return ref, err
}

// DockerImageLocal builds an amd64 image for local scanning (no push).
func DockerImageLocal(p *core.Pipeline) *dagger.Container {
	binary := p.GoBase().
		WithEnvVariable("CGO_ENABLED", "0").
		WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", "amd64").
		WithExec([]string{
			"go", "build", "-a",
			"-ldflags=-s -w -extldflags '-static'",
			"-o", "/out/manager",
			"cmd/manager/main.go",
		}).
		File("/out/manager")

	return p.Client.Container().
		From(core.DistrolessImage).
		WithFile("/manager", binary).
		WithUser("65532:65532").
		WithEntrypoint([]string{"/manager"})
}
