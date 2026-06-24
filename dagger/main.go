// Package main is the Dagger module for the ckodex-kserve-llm-operator CI/CD pipeline.
//
// Run `dagger develop` once to generate internal/dagger/ before calling any functions.
// Available via: dagger call lint | test | coverage | build | build-check | scan | sbom | all
package main

import (
	"context"
	"fmt"
	"strings"

	"dagger/ckodex-operator/internal/dagger"
	"golang.org/x/sync/errgroup"
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

var sourceExcludes = []string{
	".git/",
	".dagger/",
	".cache/",
	".cocoindex_code/",
	".tmp/",
	"bin/",
	"console/.next/",
	"console/node_modules/",
	"dist/",
	"scratch/bin/",
	"target/",
	"node_modules/",
	"*.log",
	"*.out",
}

// CkodexOperator is the root Dagger module type for the operator CI/CD pipeline.
type CkodexOperator struct{}

// Lint runs golangci-lint over the operator source.
//
// Usage: dagger call lint --source=.
func (m *CkodexOperator) Lint(
	ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	out, err := golangciBase(source).
		WithExec([]string{"golangci-lint", "run", "-v", "--fast-only", "--timeout", "2m", "./..."}).
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
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	return testBase(source, raceCoverageTestArgs()).
		WithExec([]string{"sh", "-c", coverageGateScript()}).
		Stdout(ctx)
}

// coverageGateScript returns the shell script that enforces per-package coverage
// thresholds from coverage.out. Mirrors ci/pkg/test/test.go:coverageGateScript.
func coverageGateScript() string {
	return fmt.Sprintf(`
set -e
if [ ! -f coverage.out ]; then echo "FAIL: coverage.out not found" >&2; exit 1; fi
go tool cover -func=coverage.out > coverage.func
awk '
BEGIN {
  order = "controller gateway storage auth inference observability"
  split(order, pkgs, " ")
  min["controller"] = %d
  min["gateway"] = %d
  min["storage"] = %d
  min["auth"] = %d
  min["inference"] = %d
  min["observability"] = %d
}
{
  for (i = 1; i <= length(pkgs); i++) {
    pkg = pkgs[i]
    if ($1 ~ "internal/" pkg "/") {
      pct = $NF
      sub("%%", "", pct)
      sum[pkg] += pct
      count[pkg]++
    }
  }
}
END {
  failed = 0
  for (i = 1; i <= length(pkgs); i++) {
    pkg = pkgs[i]
    pct = count[pkg] > 0 ? int(sum[pkg] / count[pkg]) : 0
    printf("Coverage internal/%%s: %%d%%%% (min: %%d%%%%)\n", pkg, pct, min[pkg])
    if (pct < min[pkg]) {
      printf("FAIL: internal/%%s coverage %%d%%%% < %%d%%%% threshold\n", pkg, pct, min[pkg]) > "/dev/stderr"
      failed = 1
    }
  }
  exit failed
}
' coverage.func
`, coverageController, coverageGateway, coverageStorage, coverageAuth, coverageInference, coverageObs)
}

func coverageTestArgs() []string {
	return []string{
		"go", "test",
		"-coverprofile=coverage.out",
		"-covermode=atomic",
		"./...",
	}
}

func raceCoverageTestArgs() []string {
	return []string{
		"go", "test",
		"-race",
		"-p", "16",
		"-coverprofile=coverage.out",
		"-covermode=atomic",
		"./...",
	}
}

func testArgs() []string {
	return []string{"go", "test", "-short", "-p", "16", "./..."}
}

// Coverage runs tests and exports the coverage profile file.
//
// Usage: dagger call coverage --source=. export --path=coverage.out
func (m *CkodexOperator) Coverage(
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) *dagger.File {
	return testBase(source, coverageTestArgs()).
		File("coverage.out")
}

// Build builds the operator image container (amd64).
// For multi-arch publishing use Publish.
//
// Usage: dagger call build --source=. --version=v0.1.0
func (m *CkodexOperator) Build(
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
	// +optional
	version string,
) *dagger.Container {
	return buildVariant(source, "amd64", resolveVersion(version))
}

// BuildCheck materializes the operator image without exporting it.
//
// Usage: dagger call build-check --source=.
func (m *CkodexOperator) BuildCheck(
	ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
	// +optional
	version string,
) (string, error) {
	if _, err := buildCheck(source, resolveVersion(version)).Sync(ctx); err != nil {
		return "", err
	}
	return "build passed", nil
}

// Scan runs a Trivy vulnerability scan on the amd64 build.
//
// Usage: dagger call scan --source=.
func (m *CkodexOperator) Scan(
	ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	return scanRootfs(m.Build(source, "").Rootfs()).Stdout(ctx)
}

// Sbom generates a CycloneDX SBOM for a given image reference.
//
// Usage: dagger call sbom --source=. --image-ref=ghcr.io/org/app:v1.0.0 \
//
//	--registry-username=user --registry-token=env:GITHUB_TOKEN export --path=sbom.cdx.json
func (m *CkodexOperator) Sbom(
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
	imageRef string,
	// +optional
	registryUsername string,
	// +optional
	registryToken *dagger.Secret,
) *dagger.File {
	ctr := trivyBase().
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
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
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
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
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

// All runs the hosted fast operational gate in bounded parallel branches.
//
// Usage: dagger call all --source=.
func (m *CkodexOperator) All(
	ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	g, groupCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if _, err := m.Lint(groupCtx, source); err != nil {
			return fmt.Errorf("lint: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if _, err := m.BuildCheck(groupCtx, source, ""); err != nil {
			return fmt.Errorf("build: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return "", err
	}
	return "all checks passed", nil
}

// goModBase warms dependency caches from only go.mod/go.sum so source edits do
// not invalidate module downloads.
func goModBase(source *dagger.Directory) *dagger.Container {
	return dag.Container().
		From(goBuilderImage).
		WithWorkdir("/src").
		WithMountedFile("/src/go.mod", source.File("go.mod")).
		WithMountedFile("/src/go.sum", source.File("go.sum")).
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("go-build")).
		WithExec([]string{"mkdir", "-p", "/root/.cache/go-build/tmp"}).
		WithEnvVariable("GOCACHE", "/root/.cache/go-build").
		WithEnvVariable("GOTMPDIR", "/root/.cache/go-build/tmp").
		WithEnvVariable("GOFLAGS", "-mod=readonly").
		WithExec([]string{"go", "mod", "download"})
}

// goBase returns a configured Go container with source mounted on a warmed module base.
func goBase(source *dagger.Directory) *dagger.Container {
	return goModBase(source).
		WithMountedDirectory("/src", filteredSource(source))
}

func testBase(source *dagger.Directory, args []string) *dagger.Container {
	return goBase(source).WithExec(args)
}

// golangciBase mirrors goBase for the golangci-lint image so dependency
// resolution stays cacheable independently of source changes.
func golangciBase(source *dagger.Directory) *dagger.Container {
	return dag.Container().
		From(golangciLintImage).
		WithWorkdir("/src").
		WithMountedFile("/src/go.mod", source.File("go.mod")).
		WithMountedFile("/src/go.sum", source.File("go.sum")).
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("go-build")).
		WithMountedCache("/root/.cache/golangci-lint", dag.CacheVolume("golangci-lint")).
		WithExec([]string{"mkdir", "-p", "/root/.cache/go-build/tmp"}).
		WithEnvVariable("GOCACHE", "/root/.cache/go-build").
		WithEnvVariable("GOLANGCI_LINT_CACHE", "/root/.cache/golangci-lint").
		WithEnvVariable("GOTMPDIR", "/root/.cache/go-build/tmp").
		WithEnvVariable("GOFLAGS", "-mod=readonly").
		WithExec([]string{"go", "mod", "download"}).
		WithMountedDirectory("/src", filteredSource(source))
}

func trivyBase() *dagger.Container {
	return dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).
		From(fmt.Sprintf("aquasec/trivy:%s", trivyVersion)).
		WithMountedCache("/root/.cache/trivy", dag.CacheVolume("trivy-db")).
		WithEnvVariable("TRIVY_CACHE_DIR", "/root/.cache/trivy")
}

func scanRootfs(rootfs *dagger.Directory) *dagger.Container {
	return trivyBase().
		WithMountedDirectory("/rootfs", rootfs).
		WithExec([]string{
			"trivy", "rootfs",
			"--severity", "CRITICAL,HIGH",
			"--scanners", "vuln",
			"--exit-code", "1",
			"--ignore-unfixed",
			"--format", "table",
			"/rootfs",
		})
}

func filteredSource(source *dagger.Directory) *dagger.Directory {
	return source.Filter(dagger.DirectoryFilterOpts{
		Exclude:   sourceExcludes,
		Gitignore: true,
	})
}

// buildCheck compiles the manager binary without release-image assembly. The
// full static/distroless path remains covered by Build, Scan, and Publish.
func buildCheck(source *dagger.Directory, version string) *dagger.Container {
	ldflags := fmt.Sprintf(
		"-X github.com/ckodex-labs/kserve-llm-operator/internal/version.Version=%s",
		version,
	)
	return goBase(source).
		WithExec([]string{"mkdir", "-p", "/root/.cache/go-build/tmp", "/tmp/ckodex-build"}).
		WithEnvVariable("CGO_ENABLED", "0").
		WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", "amd64").
		WithEnvVariable("GOTMPDIR", "/root/.cache/go-build/tmp").
		WithExec([]string{
			"go", "build",
			"-trimpath",
			"-ldflags", ldflags,
			"-o", "/tmp/ckodex-build/manager",
			"./cmd/manager",
		})
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
		WithExec([]string{"mkdir", "-p", "/root/.cache/go-build/tmp", "/tmp/ckodex-build"}).
		WithEnvVariable("CGO_ENABLED", "0").
		WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", arch).
		WithEnvVariable("GOTMPDIR", "/root/.cache/go-build/tmp").
		WithExec([]string{
			"go", "build",
			"-ldflags", ldflags,
			"-o", "/tmp/ckodex-build/manager",
			"cmd/manager/main.go",
		}).
		File("/tmp/ckodex-build/manager")

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
