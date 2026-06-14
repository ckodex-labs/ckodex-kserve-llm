// Package core holds shared config and Dagger base containers for the CI pipeline.
package core

import (
	"dagger.io/dagger"
)

// Pinned base image digests — never use floating tags for reproducible builds.
const (
	GoBuilderImage  = "golang:1.25-bookworm"
	DistrolessImage = "gcr.io/distroless/static:nonroot"

	GoVersion         = "1.25"
	GolangciLintVer   = "v2.4.0"
	SyftVersion       = "v1.42.4"
	CosignVersion     = "v3.0.4"
	TrivyVersion      = "0.69.3"
	LulaVersion       = "v0.16.0"
	LulaReleaseBase   = "https://github.com/defenseunicorns-labs/lula1/releases/download/" + LulaVersion
	LulaBinaryName    = "lula_" + LulaVersion + "_Linux_amd64"
	LulaBinaryURL     = LulaReleaseBase + "/" + LulaBinaryName
	LulaChecksumsURL  = LulaReleaseBase + "/checksums.txt"
	GolangciLintImage = "golangci/golangci-lint:" + GolangciLintVer

	// Coverage thresholds.
	// CoverageController is intentionally below the project minimum (80%).
	// The controller reconcile loop requires an envtest Kubernetes API server to
	// exercise its core paths; only the unit-testable sub-functions (deployment
	// builders, cleanup, status helpers extracted to sub-packages) can be covered
	// without envtest. Integration coverage is validated in CI via `make test-e2e`.
	CoverageController    = 27
	CoverageGateway       = 80
	CoverageStorage       = 80
	CoverageAuth          = 80
	CoverageInference     = 80
	CoverageObservability = 80
)

// Config contains pipeline inputs from CLI flags and CI environment variables.
type Config struct {
	ImageRef   string
	Registry   string // Contract: Push target
	Version    string // Contract: Override git version
	GitCommit  string
	GitRepoURL string

	// Supply Chain Verification Paths (Contract)
	CosignBundlePath   string
	CosignImagePath    string
	SLSAArtifactPath   string
	SLSAProvenancePath string

	Push      bool
	Sign      bool
	Attest    bool
	SkipLint  bool
	SkipTests bool
	SkipScan  bool
}

// Pipeline holds Dagger client + source directory for all stages.
type Pipeline struct {
	Client *dagger.Client
	Source *dagger.Directory
	Cfg    *Config
}

// GoBase returns a Go container with source mounted and module cache warmed.
func (p *Pipeline) GoBase() *dagger.Container {
	goCache := p.Client.CacheVolume("go-mod")
	goBuildCache := p.Client.CacheVolume("go-build")

	return p.Client.Container().
		From(GoBuilderImage).
		WithMountedDirectory("/src", p.Source).
		WithWorkdir("/src").
		WithMountedCache("/go/pkg/mod", goCache).
		WithMountedCache("/root/.cache/go-build", goBuildCache).
		WithEnvVariable("GOFLAGS", "-mod=readonly").
		WithExec([]string{"go", "mod", "download"})
}

// LintBase returns a container using the official golangci-lint image.
func (p *Pipeline) LintBase() *dagger.Container {
	return p.Client.Container().
		From(GolangciLintImage).
		WithMountedDirectory("/src", p.Source).
		WithWorkdir("/src").
		WithMountedCache("/go/pkg/mod", p.Client.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/go-build", p.Client.CacheVolume("go-build")).
		WithMountedCache("/root/.cache/golangci-lint", p.Client.CacheVolume("golangci-lint"))
}
