package build

import (
	"context"
	"strings"

	"dagger.io/dagger"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
)

func Build(ctx context.Context, p *core.Pipeline) (string, error) {
	platforms := []dagger.Platform{"linux/amd64", "linux/arm64"}
	ref := p.Cfg.ImageRef
	if ref == "" {
		ref = "ghcr.io/ckodex-labs/ckodex-kserve-llm:dev"
	}

	// Build one container per platform.
	variants := make([]*dagger.Container, 0, len(platforms))
	for _, platform := range platforms {
		arch := strings.Split(string(platform), "/")[1] // amd64 or arm64
		binary := p.GoBase().
			WithEnvVariable("CGO_ENABLED", "0").
			WithEnvVariable("GOOS", "linux").
			WithEnvVariable("GOARCH", arch).
			WithExec([]string{
				"go", "build",
				"-a",
				"-ldflags=-s -w -extldflags '-static'",
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

	// Publish multi-platform index if pushing; otherwise just describe locally.
	if p.Cfg.Push {
		digest, err := p.Client.Container().
			Publish(ctx, ref, dagger.ContainerPublishOpts{
				PlatformVariants: variants,
			})
		return digest, err
	}

	// Local: materialize to catch build errors
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
