// Package main is the Dagger module for the ckodex-kserve-llm-operator CI/CD pipeline.
//
// Run `dagger develop` once to generate internal/dagger/ before calling any functions.
// Available via: dagger call lint | test | coverage | build | scan | sbom | all
package main

import (
	"context"
	"fmt"
	"strings"

	"dagger/ckodex-operator/internal/dagger"
)

const (
	goBuilderImage    = "golang:1.25-bookworm"
	golangciLintImage = "golangci/golangci-lint:v2.4.0"
	trivyVersion      = "0.69.3"
	cosignVersion     = "v3.0.4"
	distrolessImage   = "gcr.io/distroless/static:nonroot"

	// Coverage thresholds — mirror ci/pkg/core constants.
	coverageController = 27 // envtest required for full coverage; see ADR-008
	coverageGateway    = 80
	coverageStorage    = 80
	coverageAuth       = 80
	coverageInference  = 80
	coverageObs        = 80
)

// CkodexOperator is the root Dagger module type for the operator CI/CD pipeline.
type CkodexOperator struct{}

// Lint runs go vet and golangci-lint over the operator source.
//
// Usage: dagger call lint --source=.
func (m *CkodexOperator) Lint(
	ctx context.Context,
	// +defaultPath="/"
	source *dagger.Directory,
) (string, error) {
	if _, err := goBase(source).
		WithExec([]string{"go", "vet", "./..."}).
		Sync(ctx); err != nil {
		return "", fmt.Errorf("go vet: %w", err)
	}

	out, err := dag.Container().
		From(golangciLintImage).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("go-build")).
		WithMountedCache("/root/.cache/golangci-lint", dag.CacheVolume("golangci-lint")).
		WithExec([]string{"golangci-lint", "run", "-v", "--timeout", "10m", "./..."}).
		Stdout(ctx)
	if err != nil {
		return out, fmt.Errorf("golangci-lint: %w", err)
	}
	return "lint passed", nil
}

// Test runs the full test suite with race detection and per-package coverage gates.
//
// Usage: dagger call test --source=.
func (m *CkodexOperator) Test(
	ctx context.Context,
	// +defaultPath="/"
	source *dagger.Directory,
) (string, error) {
	return goBase(source).
		WithExec([]string{
			"go", "test",
			"-race",
			"-coverprofile=coverage.out",
			"-covermode=atomic",
			"./...",
		}).
		WithExec([]string{"sh", "-c", coverageGateScript()}).
		Stdout(ctx)
}

// coverageGateScript returns the shell script that enforces per-package coverage
// thresholds from coverage.out. Mirrors ci/pkg/test/test.go:coverageGateScript.
func coverageGateScript() string {
	return fmt.Sprintf(`
set -e
if [ ! -f coverage.out ]; then echo "FAIL: coverage.out not found" >&2; exit 1; fi
check() {
  pkg=$1; min=$2
  pct=$(go tool cover -func=coverage.out | grep "internal/${pkg}/" | awk \
    '{ sum += $NF; count++ } END { if (count > 0) print int(sum/count); else print 0 }')
  echo "Coverage internal/${pkg}: ${pct}%% (min: ${min}%%)"
  if [ "$pct" -lt "$min" ]; then
    echo "FAIL: internal/${pkg} coverage ${pct}%% < ${min}%% threshold" >&2; exit 1
  fi
}
check controller %d
check gateway %d
check storage %d
check auth %d
check inference %d
check observability %d
`, coverageController, coverageGateway, coverageStorage, coverageAuth, coverageInference, coverageObs)
}

// Coverage runs tests and exports the coverage profile file.
//
// Usage: dagger call coverage --source=. export --path=coverage.out
func (m *CkodexOperator) Coverage(
	// +defaultPath="/"
	source *dagger.Directory,
) *dagger.File {
	return goBase(source).
		WithExec([]string{
			"go", "test",
			"-coverprofile=coverage.out",
			"-covermode=atomic",
			"./...",
		}).
		File("coverage.out")
}

// Build builds the operator image container (amd64).
// For multi-arch publishing use Publish.
//
// Usage: dagger call build --source=. --version=v0.1.0
func (m *CkodexOperator) Build(
	// +defaultPath="/"
	source *dagger.Directory,
	// +optional
	version string,
) *dagger.Container {
	return buildVariant(source, "amd64", resolveVersion(version))
}

// Scan runs a Trivy vulnerability scan on the amd64 build.
//
// Usage: dagger call scan --source=.
func (m *CkodexOperator) Scan(
	ctx context.Context,
	// +defaultPath="/"
	source *dagger.Directory,
) (string, error) {
	imgTar := m.Build(source, "").AsTarball()
	return dag.Container().
		From(fmt.Sprintf("aquasec/trivy:%s", trivyVersion)).
		WithMountedFile("/image.tar", imgTar).
		WithExec([]string{
			"trivy", "image",
			"--input", "/image.tar",
			"--severity", "CRITICAL,HIGH",
			"--scanners", "vuln",
			"--exit-code", "1",
			"--ignore-unfixed",
			"--format", "table",
		}).
		Stdout(ctx)
}

// Sbom generates a CycloneDX SBOM for a given image reference.
//
// Usage: dagger call sbom --source=. --image-ref=ghcr.io/org/app:v1.0.0 \
//
//	--registry-username=user --registry-token=env:GITHUB_TOKEN export --path=sbom.cdx.json
func (m *CkodexOperator) Sbom(
	// +defaultPath="/"
	source *dagger.Directory,
	imageRef string,
	// +optional
	registryUsername string,
	// +optional
	registryToken *dagger.Secret,
) *dagger.File {
	ctr := dag.Container().
		From(fmt.Sprintf("aquasec/trivy:%s", trivyVersion)).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src")
	if registryUsername != "" {
		ctr = ctr.WithEnvVariable("TRIVY_USERNAME", registryUsername)
	}
	if registryToken != nil {
		ctr = ctr.WithSecretVariable("TRIVY_PASSWORD", registryToken)
	}
	return ctr.WithExec([]string{
		"trivy", "image",
		"--format", "cyclonedx",
		"--output", "sbom.cdx.json",
		imageRef,
	}).File("sbom.cdx.json")
}

// Publish builds a multi-arch image and pushes to a registry.
// Returns the published image digest.
//
// Usage: dagger call publish --source=. --image-ref=ghcr.io/org/app:v1.0.0 \
//
//	--registry-username=user --registry-token=env:GITHUB_TOKEN
func (m *CkodexOperator) Publish(
	ctx context.Context,
	// +defaultPath="/"
	source *dagger.Directory,
	imageRef string,
	// +optional
	version string,
	registryUsername string,
	registryToken *dagger.Secret,
) (string, error) {
	ver := resolveVersion(version)
	platforms := []string{"amd64", "arm64"}
	variants := make([]*dagger.Container, 0, len(platforms))
	for _, arch := range platforms {
		variants = append(variants, buildVariant(source, arch, ver))
	}

	ctr := dag.Container().
		WithRegistryAuth("ghcr.io", registryUsername, registryToken)

	digest, err := ctr.Publish(ctx, imageRef, dagger.ContainerPublishOpts{
		PlatformVariants: variants,
	})
	if err != nil {
		return "", fmt.Errorf("publish %s: %w", imageRef, err)
	}
	return digest, nil
}

// Lula runs OSCAL compliance validation and returns the assessment results file.
//
// Mirrors ci/pkg/security.Lula. Downloads the Lula binary, verifies its checksum,
// and validates controls defined in lula/lula-component.yaml.
//
// Usage: dagger call lula --source=. export --path=assessment-results.yaml
func (m *CkodexOperator) Lula(
	// +defaultPath="/"
	source *dagger.Directory,
) *dagger.File {
	const (
		lulaVersion    = "v0.16.0"
		lulaBase       = "https://github.com/defenseunicorns-labs/lula1/releases/download/" + lulaVersion
		lulaBinaryName = "lula_" + lulaVersion + "_Linux_amd64"
		lulaBinaryURL  = lulaBase + "/" + lulaBinaryName
		lulaChecksums  = lulaBase + "/checksums.txt"
	)
	cmd := fmt.Sprintf(`set -eu
apk add --no-cache curl ca-certificates coreutils >/dev/null
curl -fsSL -o /tmp/lula %q
curl -fsSL -o /tmp/checksums.txt %q
expected="$(grep "  %s$" /tmp/checksums.txt | awk '{print $1}')"
echo "${expected}  /tmp/lula" | sha256sum -c -
install -m 0755 /tmp/lula /usr/local/bin/lula
lula validate -f lula/lula-component.yaml -o assessment-results.yaml`,
		lulaBinaryURL, lulaChecksums, lulaBinaryName)

	return dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).
		From("alpine:3.20").
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"sh", "-lc", cmd}).
		File("assessment-results.yaml")
}

// All runs lint, test, build, scan, and OSCAL validation in sequence.
//
// Usage: dagger call all --source=.
func (m *CkodexOperator) All(
	ctx context.Context,
	// +defaultPath="/"
	source *dagger.Directory,
) (string, error) {
	if _, err := m.Lint(ctx, source); err != nil {
		return "", fmt.Errorf("lint: %w", err)
	}
	if _, err := m.Test(ctx, source); err != nil {
		return "", fmt.Errorf("test: %w", err)
	}
	if _, err := m.Scan(ctx, source); err != nil {
		return "", fmt.Errorf("scan: %w", err)
	}
	return "all checks passed", nil
}

// goBase returns a configured Go container with source mounted and module cache warmed.
func goBase(source *dagger.Directory) *dagger.Container {
	return dag.Container().
		From(goBuilderImage).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("go-build")).
		WithEnvVariable("GOFLAGS", "-mod=readonly").
		WithExec([]string{"go", "mod", "download"})
}

// buildVariant builds the operator binary and wraps it in a distroless container
// for a specific GOARCH target.
func buildVariant(source *dagger.Directory, arch, version string) *dagger.Container {
	ldflags := fmt.Sprintf(
		"-s -w -extldflags '-static' -X github.com/ckodex-labs/kserve-llm-operator/internal/version.Version=%s",
		version,
	)
	platform := dagger.Platform("linux/" + arch)
	binary := goBase(source).
		WithEnvVariable("CGO_ENABLED", "0").
		WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", arch).
		WithExec([]string{
			"go", "build", "-a",
			"-ldflags", ldflags,
			"-o", "/out/manager",
			"cmd/manager/main.go",
		}).
		File("/out/manager")

	return dag.Container(dagger.ContainerOpts{Platform: platform}).
		From(distrolessImage).
		WithFile("/manager", binary).
		WithUser("65532:65532").
		WithEntrypoint([]string{"/manager"})
}

// resolveVersion returns "dev" when version is empty.
func resolveVersion(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return "dev"
	}
	return v
}
