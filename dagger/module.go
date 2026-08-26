package main

import (
	"context"
	"fmt"
	"time"

	"dagger/ckodex-operator/internal/dagger"

	"golang.org/x/sync/errgroup"
)

// CkodexOperator is the root Dagger module type for the operator CI/CD pipeline.
type CkodexOperator struct{}

// Lint runs golangci-lint over the operator source.
func (m *CkodexOperator) Lint(ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	out, err := golangciBase(source).WithExec([]string{"golangci-lint", "run", "-v", "--timeout", "2m", "./..."}).Stdout(ctx)
	if err != nil {
		return out, fmt.Errorf("golangci-lint: %w", err)
	}
	return "lint passed", nil
}

// Test runs the full test suite with race detection and per-package coverage gates.
func (m *CkodexOperator) Test(ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	return testBase(source, raceCoverageTestArgs()).WithExec([]string{"sh", "-c", coverageGateScript()}).Stdout(ctx)
}

// Coverage runs tests and exports the coverage profile file.
func (m *CkodexOperator) Coverage(
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) *dagger.File {
	return testBase(source, coverageTestArgs()).File("coverage.out")
}

// Conformance runs the specification-backed tests without requiring a Kubernetes cluster.
func (m *CkodexOperator) Conformance(ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	out, err := testBase(source, []string{"go", "test", "-race", "./test/conformance/..."}).Stdout(ctx)
	if err != nil {
		return out, fmt.Errorf("conformance: %w", err)
	}
	return "conformance passed", nil
}

// Integration runs the controller integration suite against a real envtest API server.
func (m *CkodexOperator) Integration(ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	script := `set -eu
case "$(uname -m)" in
  x86_64)
    asset="setup-envtest-linux-amd64"
    expected="` + envtestToolSHA256AMD64 + `"
    envtest_asset="envtest-v` + envtestK8sVersion + `-linux-amd64.tar.gz"
    envtest_expected="` + envtestAssetsSHA512AMD64 + `"
    ;;
  aarch64|arm64)
    asset="setup-envtest-linux-arm64"
    expected="` + envtestToolSHA256ARM64 + `"
    envtest_asset="envtest-v` + envtestK8sVersion + `-linux-arm64.tar.gz"
    envtest_expected="` + envtestAssetsSHA512ARM64 + `"
    ;;
  *)
    echo "unsupported envtest tool architecture: $(uname -m)" >&2
    exit 1
    ;;
esac
curl -fsSL --retry 3 --retry-delay 2 --connect-timeout 10 --max-time 120 \
  -o /tmp/setup-envtest "` + envtestToolBaseURL + `/${asset}"
echo "${expected}  /tmp/setup-envtest" | sha256sum --check --strict -
install -m 0755 /tmp/setup-envtest /usr/local/bin/setup-envtest
curl -fsSL --retry 3 --retry-delay 2 --connect-timeout 10 --max-time 180 \
  -o /tmp/envtest.tar.gz "` + envtestAssetsBaseURL + `/${envtest_asset}"
echo "${envtest_expected}  /tmp/envtest.tar.gz" | sha512sum --check --strict -
setup-envtest sideload "` + envtestK8sVersion + `" < /tmp/envtest.tar.gz
assets="$(setup-envtest use -i -p path "` + envtestK8sVersion + `")"
REQUIRE_ENVTEST=1 KUBEBUILDER_ASSETS="$assets" go test -race -count=1 -p 1 ./test/integration/...`
	out, err := goBase(source).
		WithMountedCache("/root/.local/share/kubebuilder-envtest", dag.CacheVolume("envtest-assets")).
		WithExec([]string{"sh", "-ec", script}).Stdout(ctx)
	if err != nil {
		return out, fmt.Errorf("integration: %w", err)
	}
	return "integration passed", nil
}

// Build builds the operator image container (amd64).
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
func (m *CkodexOperator) BuildCheck(ctx context.Context,
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
func (m *CkodexOperator) Scan(ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	return scanRootfs(m.Build(source, "").Rootfs()).Stdout(ctx)
}

// Sbom generates a CycloneDX SBOM for a given image reference.
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
	ctr := trivyBase().WithMountedDirectory("/src", source).WithWorkdir("/src")
	if registryUsername != "" {
		ctr = ctr.WithEnvVariable("TRIVY_USERNAME", registryUsername)
	}
	if registryToken != nil {
		ctr = ctr.WithSecretVariable("TRIVY_PASSWORD", registryToken)
	}
	return ctr.WithExec([]string{"trivy", "image", "--format", "cyclonedx", "--output", "sbom.cdx.json", imageRef}).File("sbom.cdx.json")
}

// Publish builds a multi-arch image and pushes it to a registry.
func (m *CkodexOperator) Publish(ctx context.Context,
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

	ctr := dag.Container().WithRegistryAuth("ghcr.io", registryUsername, registryToken)
	digest, err := ctr.Publish(ctx, imageRef, dagger.ContainerPublishOpts{PlatformVariants: variants})
	if err != nil {
		return "", fmt.Errorf("publish %s: %w", imageRef, err)
	}
	return digest, nil
}

// Lula verifies the linked validation definitions, runs offline IA-9 policy checks,
// then evaluates the OSCAL component against the available cluster.
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
	installCmd := fmt.Sprintf(`set -eu
apk add --no-cache curl ca-certificates coreutils >/dev/null
curl -fsSL -o /tmp/lula %q
curl -fsSL -o /tmp/checksums.txt %q
expected="$$(grep "  %s$$" /tmp/checksums.txt | awk '{print $1}')"
echo "$${expected}  /tmp/lula" | sha256sum -c -
install -m 0755 /tmp/lula /usr/local/bin/lula`, lulaBinaryURL, lulaChecksums, lulaBinaryName)
	validateCmd := `set -eu
for validation in \
  lula/network-policy-validation.yaml \
  lula/governance-validation.yaml \
  lula/supply-chain-validation.yaml \
  lula/ois-validation.yaml \
  lula/spire-identity-validation.yaml
do
  lula dev lint -f "$${validation}"
done
lula dev validate -f lula/spire-identity-validation.yaml \
  -r lula/testdata/spire-identity-valid.json
lula dev validate -f lula/spire-identity-validation.yaml \
  -r lula/testdata/spire-identity-missing-registration.json -e=false
lula validate -f lula/lula-component.yaml -o assessment-results.yaml`

	return dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From("alpine:3.20").
		WithExec([]string{"sh", "-lc", installCmd}).WithMountedDirectory("/src", source).
		WithWorkdir("/src").WithExec([]string{"sh", "-lc", validateCmd}).File("assessment-results.yaml")
}

// All runs the hosted fast operational gate in bounded parallel branches.
func (m *CkodexOperator) All(ctx context.Context,
	// +defaultPath="/"
	// +ignore=[".git", ".dagger", ".cache", ".cocoindex_code", ".tmp", "bin", "console/.next", "console/node_modules", "dist", "scratch/bin", "target", "**/node_modules", "*.log", "*.out"]
	source *dagger.Directory,
) (string, error) {
	deadlineCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	g, groupCtx := errgroup.WithContext(deadlineCtx)
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
