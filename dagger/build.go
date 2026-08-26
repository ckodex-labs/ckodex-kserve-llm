package main

import (
	"fmt"
	"strings"

	"dagger/ckodex-operator/internal/dagger"
)

func buildCheck(source *dagger.Directory, version string) *dagger.Container {
	ldflags := fmt.Sprintf("-X github.com/ckodex-labs/kserve-llm-operator/internal/version.Version=%s", version)
	return goBase(source).WithExec([]string{"mkdir", "-p", "/root/.cache/go-build/tmp", "/tmp/ckodex-build"}).
		WithEnvVariable("CGO_ENABLED", "0").WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", "amd64").WithEnvVariable("GOTMPDIR", "/root/.cache/go-build/tmp").
		WithExec([]string{"go", "build", "-trimpath", "-ldflags", ldflags, "-o", "/tmp/ckodex-build/manager", "./cmd/manager"})
}

func buildVariant(source *dagger.Directory, arch, version string) *dagger.Container {
	ldflags := fmt.Sprintf("-s -w -extldflags '-static' -X github.com/ckodex-labs/kserve-llm-operator/internal/version.Version=%s", version)
	platform := dagger.Platform("linux/" + arch)
	binary := goBase(source).WithExec([]string{"mkdir", "-p", "/root/.cache/go-build/tmp", "/tmp/ckodex-build"}).
		WithEnvVariable("CGO_ENABLED", "0").WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", arch).WithEnvVariable("GOTMPDIR", "/root/.cache/go-build/tmp").
		WithExec([]string{"go", "build", "-ldflags", ldflags, "-o", "/tmp/ckodex-build/manager", "./cmd/manager"}).
		File("/tmp/ckodex-build/manager")

	return dag.Container(dagger.ContainerOpts{Platform: platform}).From(distrolessImage).
		WithFile("/manager", binary).WithUser("65532:65532").WithEntrypoint([]string{"/manager"})
}

func resolveVersion(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return "dev"
	}
	return v
}
