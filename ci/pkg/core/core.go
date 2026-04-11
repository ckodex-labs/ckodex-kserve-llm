package core

import (
	"dagger.io/dagger"
)

// Pinned base image digests — never use floating tags for reproducible builds.
const (
	GoBuilderImage  = "golang:1.25-bookworm"
	DistrolessImage = "gcr.io/distroless/static:nonroot"

	GoVersion         = "1.25"
	GolangciLintVer   = "v1.64.6"
	SyftVersion       = "v1.42.4"
	CosignVersion     = "v3.0.4"
	TrivyVersion      = "0.69.3"
	LulaVersion       = "v0.9.5"
	LulaImage         = "ghcr.io/defenseunicorns/lula:" + LulaVersion
	GolangciLintImage = "golangci/golangci-lint:" + GolangciLintVer

	// Coverage thresholds.
	CoverageController    = 72
	CoverageGateway       = 80
	CoverageStorage       = 80
	CoverageAuth          = 80
	CoverageInference     = 80
	CoverageObservability = 80
)

type Config struct {
	ImageRef   string
	Registry   string // Contract: Push target
	Version    string // Contract: Override git version
	Push       bool
	Sign       bool
	Attest     bool
	SkipTests  bool
	SkipScan   bool
	GitCommit  string
	GitRepoURL string

	// Supply Chain Verification Paths (Contract)
	CosignBundlePath    string
	CosignImagePath     string
	SLSAArtifactPath    string
	SLSAProvenancePath  string
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
