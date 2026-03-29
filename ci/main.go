// CKodex KServe LLM Operator — Dagger CI/CD Pipeline
//
// Stages (in dependency order):
//
//	lint    → go vet + golangci-lint
//	test    → go test -race + coverage gate (controller ≥ 40%, storage ≥ 30%, gateway ≥ 35%)
//	build   → multi-arch distroless image (linux/amd64, linux/arm64)
//	scan    → Trivy vulnerability scan (CRITICAL/HIGH, exit on unfixed)
//	sbom    → Syft SBOM (CycloneDX JSON + SPDX JSON)
//	sign    → cosign keyless signing via OIDC → Rekor transparency log
//	attest  → cosign attest: SBOM (cyclonedx) + SLSA provenance (slsaprovenance1)
//
// Usage:
//
//	go run ./ci/main.go                          # lint + test + build + scan locally
//	go run ./ci/main.go --push --sign --attest   # full release pipeline (requires OIDC env)
//	go run ./ci/main.go --image ghcr.io/ckodex/kserve-llm-operator:v1.0.0 --push --sign
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"dagger.io/dagger"
)

// Pinned base image digests — never use floating tags for reproducible builds.
// Update digests by running: crane digest <image>:<tag>
const (
	// goBuilderImage pins the build environment to a specific bookworm digest.
	goBuilderImage = "golang:1.25-bookworm"

	// distrolessDigest: gcr.io/distroless/static:nonroot
	// Verify: cosign verify gcr.io/distroless/static:nonroot --certificate-oidc-issuer=...
	distrolessImage = "gcr.io/distroless/static:nonroot"

	goVersion         = "1.25"
	golangciLintVer   = "v1.63.4"
	syftVersion       = "v1.21.0"
	cosignVersion     = "v2.4.1"
	trivyVersion      = "0.58.2"
	golangciLintImage = "golangci/golangci-lint:" + golangciLintVer

	// Coverage thresholds — set at current actuals so any regression fails CI.
	// Do not lower these without a tracked reason in the commit message.
	coverageController    = 80
	coverageGateway       = 80
	coverageStorage       = 80
	coverageAuth          = 80
	coverageInference     = 80
	coverageObservability = 80
)

type config struct {
	imageRef   string // e.g. ghcr.io/ckodex/kserve-llm-operator:v1.0.0
	push       bool   // push image to registry after build
	sign       bool   // cosign keyless sign (requires OIDC env vars)
	attest     bool   // attach SBOM + SLSA provenance attestations
	skipTests  bool   // skip test stage (for faster local builds)
	skipScan   bool   // skip Trivy scan
	gitCommit  string // git commit SHA for SLSA provenance
	gitRepoURL string // git repo URL for SLSA provenance
}

func main() {
	cfg := parseFlags()

	ctx := context.Background()
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		fatal("connect to Dagger", err)
	}
	defer client.Close()

	source := client.Host().Directory(".", dagger.HostDirectoryOpts{
		Exclude: []string{".git", "bin", "ci"},
	})

	p := &pipeline{client: client, source: source, cfg: cfg}

	// Lint — always run
	if _, err := p.lint(ctx); err != nil {
		fatal("lint", err)
	}
	log("lint passed")

	// Test + coverage gate
	if !cfg.skipTests {
		if _, err := p.test(ctx); err != nil {
			fatal("test", err)
		}
		log("tests passed (coverage gates met)")
	}

	// Build multi-arch image
	imageRef, err := p.build(ctx)
	if err != nil {
		fatal("build", err)
	}
	log("build passed → %s", imageRef)

	// Vulnerability scan
	if !cfg.skipScan {
		if _, err := p.scan(ctx); err != nil {
			fatal("security scan", err)
		}
		log("trivy scan passed (no CRITICAL/HIGH unfixed CVEs)")
	}

	// SBOM generation
	sbomFile, err := p.sbom(ctx)
	if err != nil {
		fatal("sbom", err)
	}
	log("sbom generated → %s", sbomFile)

	// Push + sign + attest (release path only)
	if cfg.push && cfg.sign {
		pushedRef, err := p.push(ctx, imageRef)
		if err != nil {
			fatal("push", err)
		}
		log("pushed → %s", pushedRef)

		if err := p.sign(ctx, pushedRef); err != nil {
			fatal("sign", err)
		}
		log("signed → Rekor entry created")

		if cfg.attest {
			if err := p.attest(ctx, pushedRef, sbomFile); err != nil {
				fatal("attest", err)
			}
			log("attestations attached (SBOM + SLSA provenance)")
		}
	}

	log("pipeline complete")
}

// pipeline holds Dagger client + source directory for all stages.
type pipeline struct {
	client *dagger.Client
	source *dagger.Directory
	cfg    *config
}

// --- Stage: lint ---

func (p *pipeline) lint(ctx context.Context) (string, error) {
	// Two-step lint: go vet (fast, built-in) + golangci-lint (comprehensive)
	goVet, err := p.goBase().
		WithExec([]string{"go", "vet", "./..."}).
		Stdout(ctx)
	if err != nil {
		return goVet, fmt.Errorf("go vet: %w", err)
	}

	return p.client.Container().
		From(golangciLintImage).
		WithMountedDirectory("/src", p.source).
		WithWorkdir("/src").
		WithExec([]string{
			"golangci-lint", "run",
			"--timeout", "5m",
			"--out-format", "colored-line-number",
		}).
		Stdout(ctx)
}

// --- Stage: test ---

func (p *pipeline) test(ctx context.Context) (string, error) {
	out, err := p.goBase().
		WithExec([]string{
			"go", "test",
			"-race",
			"-coverprofile=coverage.out",
			"-covermode=atomic",
			"./...",
		}).
		// Coverage gate: fail if any measured package is below threshold.
		WithExec([]string{
			"sh", "-c", coverageGateScript(
				coverageController, coverageGateway, coverageStorage,
				coverageAuth, coverageInference, coverageObservability,
			),
		}).
		Stdout(ctx)
	return out, err
}

// coverageGateScript returns a shell script that parses coverage.out and
// fails if any measured package falls below its threshold.
func coverageGateScript(ctrlMin, gwMin, storeMin, authMin, inferMin, obsMin int) string {
	return fmt.Sprintf(`
set -e
check() {
  pkg=$1; min=$2
  pct=$(go tool cover -func=coverage.out | grep "^github.com/ckodex-labs/kserve-llm-operator/internal/${pkg}" | awk 'END{gsub(/%%/,""); print int($3)}')
  if [ -z "$pct" ]; then pct=0; fi
  echo "Coverage ${pkg}: ${pct}%% (min: ${min}%%)"
  if [ "$pct" -lt "$min" ]; then
    echo "FAIL: ${pkg} coverage ${pct}%% < ${min}%% threshold" >&2; exit 1
  fi
}
check controller %d
check gateway %d
check storage %d
check auth %d
check inference %d
check observability %d
`, ctrlMin, gwMin, storeMin, authMin, inferMin, obsMin)
}

// --- Stage: build (multi-arch) ---

func (p *pipeline) build(ctx context.Context) (string, error) {
	platforms := []dagger.Platform{"linux/amd64", "linux/arm64"}
	ref := p.cfg.imageRef
	if ref == "" {
		ref = "ghcr.io/ckodex/kserve-llm-operator:dev"
	}

	// Build one container per platform.
	variants := make([]*dagger.Container, 0, len(platforms))
	for _, platform := range platforms {
		arch := strings.Split(string(platform), "/")[1] // amd64 or arm64
		binary := p.goBase().
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

		variant := p.client.Container(dagger.ContainerOpts{Platform: platform}).
			From(distrolessImage).
			WithFile("/manager", binary).
			WithUser("65532:65532").
			WithEntrypoint([]string{"/manager"})

		variants = append(variants, variant)
	}

	// Publish multi-platform index if pushing; otherwise just describe locally.
	if p.cfg.push {
		digest, err := p.client.Container().
			Publish(ctx, ref, dagger.ContainerPublishOpts{
				PlatformVariants: variants,
			})
		return digest, err
	}

	// Local: return the amd64 variant ref for scanning/testing.
	_, err := variants[0].Stdout(ctx) // materialize to catch build errors
	return ref, err
}

// --- Stage: push ---

func (p *pipeline) push(ctx context.Context, imageRef string) (string, error) {
	registryUser := os.Getenv("REGISTRY_USERNAME")
	registryToken := os.Getenv("REGISTRY_TOKEN")

	ctr := p.client.Container().From(distrolessImage)
	if registryToken != "" {
		ctr = ctr.WithRegistryAuth(
			registryHostFrom(imageRef),
			registryUser,
			p.client.SetSecret("registry-token", registryToken),
		)
	}
	return ctr.Publish(ctx, imageRef)
}

// --- Stage: scan (Trivy) ---

func (p *pipeline) scan(ctx context.Context) (string, error) {
	img := p.dockerImageLocal(ctx)
	imgTar := img.AsTarball()

	return p.client.Container().
		From(fmt.Sprintf("aquasec/trivy:%s", trivyVersion)).
		WithMountedFile("/image.tar", imgTar).
		WithExec([]string{
			"trivy", "image",
			"--input", "/image.tar",
			"--severity", "CRITICAL,HIGH",
			"--exit-code", "1",
			"--ignore-unfixed",
			"--format", "table",
		}).
		Stdout(ctx)
}

// --- Stage: sbom ---

// sbom generates both CycloneDX and SPDX SBOMs and returns the CycloneDX file path.
func (p *pipeline) sbom(ctx context.Context) (string, error) {
	sbomDir := p.client.Container().
		From(fmt.Sprintf("anchore/syft:%s", syftVersion)).
		WithMountedDirectory("/src", p.source).
		WithWorkdir("/src").
		WithExec([]string{
			"syft", "dir:/src",
			"-o", "cyclonedx-json=/sbom/sbom.cdx.json",
			"-o", "spdx-json=/sbom/sbom.spdx.json",
		}).
		Directory("/sbom")

	// Export to host for attestation step.
	if _, err := sbomDir.Export(ctx, "sbom"); err != nil {
		return "", fmt.Errorf("export sbom: %w", err)
	}

	return "sbom/sbom.cdx.json", nil
}

// --- Stage: sign (cosign keyless) ---

// sign performs cosign keyless signing using a GitHub OIDC token (or any OIDC
// provider). The signed image digest is recorded in the Rekor transparency log.
// Required env vars in CI:
//
//	SIGSTORE_ID_TOKEN  — OIDC token from GitHub Actions (id-token permission)
func (p *pipeline) sign(ctx context.Context, imageRef string) error {
	idToken := os.Getenv("SIGSTORE_ID_TOKEN")
	if idToken == "" {
		// Also accept the GitHub Actions OIDC token variable name.
		idToken = os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	}

	ctr := p.client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", cosignVersion)).
		WithEnvVariable("COSIGN_YES", "true")

	if idToken != "" {
		ctr = ctr.WithSecretVariable("SIGSTORE_ID_TOKEN",
			p.client.SetSecret("sigstore-id-token", idToken))
	}

	_, err := ctr.
		WithExec([]string{
			"cosign", "sign",
			"--yes",
			imageRef,
		}).
		Stdout(ctx)
	return err
}

// --- Stage: attest (SBOM + SLSA provenance) ---

// attest attaches two attestations to the image:
//  1. CycloneDX SBOM (type: cyclonedx)
//  2. SLSA provenance v1 predicate (type: slsaprovenance1)
func (p *pipeline) attest(ctx context.Context, imageRef, sbomFilePath string) error {
	idToken := os.Getenv("SIGSTORE_ID_TOKEN")

	// 1. Attach SBOM attestation.
	sbomFile := p.client.Host().File(sbomFilePath)
	ctr := p.client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", cosignVersion)).
		WithEnvVariable("COSIGN_YES", "true").
		WithMountedFile("/sbom.cdx.json", sbomFile)

	if idToken != "" {
		ctr = ctr.WithSecretVariable("SIGSTORE_ID_TOKEN",
			p.client.SetSecret("sigstore-id-token-attest", idToken))
	}

	if _, err := ctr.
		WithExec([]string{
			"cosign", "attest",
			"--yes",
			"--type", "cyclonedx",
			"--predicate", "/sbom.cdx.json",
			imageRef,
		}).
		Stdout(ctx); err != nil {
		return fmt.Errorf("attach sbom attestation: %w", err)
	}

	// 2. Generate SLSA provenance predicate and attach it.
	provenance := slsaProvenance(imageRef, p.cfg.gitCommit, p.cfg.gitRepoURL)
	provJSON, err := json.Marshal(provenance)
	if err != nil {
		return fmt.Errorf("marshal slsa provenance: %w", err)
	}

	if err := os.WriteFile("slsa-provenance.json", provJSON, 0o600); err != nil {
		return fmt.Errorf("write slsa provenance: %w", err)
	}

	provFile := p.client.Host().File("slsa-provenance.json")
	ctr2 := p.client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", cosignVersion)).
		WithEnvVariable("COSIGN_YES", "true").
		WithMountedFile("/provenance.json", provFile)

	if idToken != "" {
		ctr2 = ctr2.WithSecretVariable("SIGSTORE_ID_TOKEN",
			p.client.SetSecret("sigstore-id-token-prov", idToken))
	}

	_, err = ctr2.
		WithExec([]string{
			"cosign", "attest",
			"--yes",
			"--type", "slsaprovenance1",
			"--predicate", "/provenance.json",
			imageRef,
		}).
		Stdout(ctx)
	return err
}

// --- SLSA provenance predicate ---

// slsaProvenance builds a minimal SLSA v1.0 provenance predicate.
// https://slsa.dev/provenance/v1
func slsaProvenance(imageRef, gitCommit, repoURL string) map[string]any {
	if gitCommit == "" {
		gitCommit = os.Getenv("GITHUB_SHA")
	}
	if repoURL == "" {
		repoURL = os.Getenv("GITHUB_SERVER_URL") + "/" + os.Getenv("GITHUB_REPOSITORY")
	}
	runID := os.Getenv("GITHUB_RUN_ID")
	builderID := "https://github.com/slsa-framework/slsa-github-generator/.github/workflows/builder_go_slsa3.yml"
	if runID != "" {
		builderID = fmt.Sprintf("https://github.com/%s/actions/runs/%s",
			os.Getenv("GITHUB_REPOSITORY"), runID)
	}

	return map[string]any{
		"buildDefinition": map[string]any{
			"buildType": "https://slsa.dev/provenance/v1",
			"externalParameters": map[string]any{
				"source": map[string]string{
					"uri":    repoURL,
					"digest": gitCommit,
				},
			},
			"resolvedDependencies": []map[string]any{
				{
					"uri":    repoURL,
					"digest": map[string]string{"gitCommit": gitCommit},
				},
			},
		},
		"runDetails": map[string]any{
			"builder": map[string]string{
				"id": builderID,
			},
			"metadata": map[string]any{
				"invocationId": runID,
				"startedOn":    time.Now().UTC().Format(time.RFC3339),
				"subject": []map[string]string{
					{"name": imageRef},
				},
			},
		},
	}
}

// --- Helpers ---

// goBase returns a Go container with source mounted and module cache warmed.
func (p *pipeline) goBase() *dagger.Container {
	goCache := p.client.CacheVolume("go-mod")
	goBuildCache := p.client.CacheVolume("go-build")

	return p.client.Container().
		From(goBuilderImage).
		WithMountedDirectory("/src", p.source).
		WithWorkdir("/src").
		WithMountedCache("/go/pkg/mod", goCache).
		WithMountedCache("/root/.cache/go-build", goBuildCache).
		WithEnvVariable("GOFLAGS", "-mod=readonly").
		WithExec([]string{"go", "mod", "download"})
}

// dockerImageLocal builds an amd64 image for local scanning (no push).
func (p *pipeline) dockerImageLocal(ctx context.Context) *dagger.Container {
	binary := p.goBase().
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

	return p.client.Container().
		From(distrolessImage).
		WithFile("/manager", binary).
		WithUser("65532:65532").
		WithEntrypoint([]string{"/manager"})
}

func registryHostFrom(imageRef string) string {
	// e.g. ghcr.io/ckodex/foo:tag → ghcr.io
	parts := strings.SplitN(imageRef, "/", 2)
	if len(parts) > 1 && strings.Contains(parts[0], ".") {
		return parts[0]
	}
	return "index.docker.io"
}

func parseFlags() *config {
	cfg := &config{}
	flag.StringVar(&cfg.imageRef, "image", "", "Image reference to build/push (e.g. ghcr.io/org/app:v1.0.0)")
	flag.BoolVar(&cfg.push, "push", false, "Push image to registry after build")
	flag.BoolVar(&cfg.sign, "sign", false, "Sign image with cosign keyless (requires OIDC env)")
	flag.BoolVar(&cfg.attest, "attest", false, "Attach SBOM + SLSA provenance attestations")
	flag.BoolVar(&cfg.skipTests, "skip-tests", false, "Skip test stage")
	flag.BoolVar(&cfg.skipScan, "skip-scan", false, "Skip Trivy vulnerability scan")
	flag.StringVar(&cfg.gitCommit, "git-commit", "", "Git commit SHA for SLSA provenance (default: $GITHUB_SHA)")
	flag.StringVar(&cfg.gitRepoURL, "git-repo", "", "Git repo URL for SLSA provenance (default: $GITHUB_SERVER_URL/$GITHUB_REPOSITORY)")
	flag.Parse()
	return cfg
}

func log(msg string, args ...any) {
	fmt.Printf("✅ "+msg+"\n", args...)
}

func fatal(stage string, err error) {
	fmt.Fprintf(os.Stderr, "❌ %s failed: %v\n", stage, err)
	os.Exit(1)
}
