// Package main runs the Dagger-backed CI/CD pipeline for the operator.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"dagger.io/dagger"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/build"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/lint"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/security"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/supplychain"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/test"
)

func main() {
	cfg := parseFlags()

	ctx := context.Background()
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		fatal("connect to Dagger", err)
	}
	defer func() { _ = client.Close() }()

	source := client.Host().Directory(".", dagger.HostDirectoryOpts{
		Exclude: []string{".git", "bin", "node_modules"},
	})

	p := &core.Pipeline{Client: client, Source: source, Cfg: cfg}

	if err := runPipeline(ctx, p); err != nil {
		fatal("pipeline", err)
	}
}

func parseFlags() *core.Config {
	cfg := &core.Config{}
	flag.StringVar(&cfg.ImageRef, "image", "", "Image reference to build/push (e.g. ghcr.io/org/app:v1.0.0)")
	flag.BoolVar(&cfg.Push, "push", false, "Push image to registry after build")
	flag.BoolVar(&cfg.Sign, "sign", false, "Sign image with cosign keyless (requires OIDC env)")
	flag.BoolVar(&cfg.Attest, "attest", false, "Attach dev-grade SBOM + SLSA provenance attestations (L2)")
	flag.BoolVar(&cfg.SkipTests, "skip-tests", false, "Skip test stage")
	flag.BoolVar(&cfg.SkipScan, "skip-scan", false, "Skip Trivy vulnerability scan")
	flag.StringVar(&cfg.GitCommit, "git-commit", "", "Git commit SHA for SLSA provenance (default: $GITHUB_SHA)")
	flag.StringVar(&cfg.GitRepoURL, "git-repo", "", "Git repo URL for SLSA provenance (default: $GITHUB_SERVER_URL/$GITHUB_REPOSITORY)")
	flag.Parse()

	// Environment variable contract overrides
	if v := os.Getenv("REGISTRY"); v != "" {
		cfg.Registry = v
		cfg.Push = true
	}
	if v := os.Getenv("VERSION"); v != "" {
		cfg.Version = v
	}
	cfg.CosignBundlePath = os.Getenv("COSIGN_BUNDLE_PATH")
	cfg.CosignImagePath = os.Getenv("COSIGN_IMAGE")
	if cfg.CosignImagePath == "" {
		cfg.CosignImagePath = os.Getenv("COSIGN_ARTIFACT_PATH")
	}
	cfg.SLSAArtifactPath = os.Getenv("SLSA_ARTIFACT_PATH")
	cfg.SLSAProvenancePath = os.Getenv("SLSA_PROVENANCE_PATH")

	return cfg
}

func runPipeline(ctx context.Context, p *core.Pipeline) error {
	if err := runLint(ctx, p); err != nil {
		return err
	}
	log("lint passed")

	if !p.Cfg.SkipTests {
		if err := runTests(ctx, p); err != nil {
			return err
		}
		log("tests passed (coverage gates met)")
	}

	imageRef, err := runBuild(ctx, p)
	if err != nil {
		return err
	}
	log("build passed → %s", imageRef)

	if !p.Cfg.SkipScan {
		if err := runSecurityScan(ctx, p); err != nil {
			return err
		}
		log("trivy scan passed (no CRITICAL/HIGH unfixed CVEs)")
	}

	if err := runLula(ctx, p); err != nil {
		return err
	}
	log("lula oscal validation passed → bin/oscal-assessment-results.yaml")

	if !p.Cfg.SkipScan {
		if err := runSupplyChain(ctx, p, imageRef); err != nil {
			return err
		}
	}

	if err := runVerificationContracts(ctx, p); err != nil {
		return err
	}

	log("pipeline complete")
	return nil
}

func runLint(ctx context.Context, p *core.Pipeline) error {
	// Lint is always run.
	out, err := lint.Lint(ctx, p)
	if err != nil {
		fmt.Fprintln(os.Stderr, out)
		return fmt.Errorf("lint: %w", err)
	}
	return nil
}

func runTests(ctx context.Context, p *core.Pipeline) error {
	if _, err := test.Test(ctx, p); err != nil {
		return fmt.Errorf("test: %w", err)
	}
	return nil
}

func runBuild(ctx context.Context, p *core.Pipeline) (string, error) {
	imageRef, err := build.Build(ctx, p)
	if err != nil {
		return "", fmt.Errorf("build: %w", err)
	}
	return imageRef, nil
}

func runSecurityScan(ctx context.Context, p *core.Pipeline) error {
	if _, err := security.Scan(ctx, p); err != nil {
		return fmt.Errorf("security scan: %w", err)
	}
	return nil
}

func runLula(ctx context.Context, p *core.Pipeline) error {
	lulaResults, err := security.Lula(ctx, p)
	if err != nil {
		return fmt.Errorf("lula validation: %w", err)
	}
	if _, err := lulaResults.Export(ctx, "bin/oscal-assessment-results.yaml"); err != nil {
		return fmt.Errorf("export lula results: %w", err)
	}
	return nil
}

func runSupplyChain(ctx context.Context, p *core.Pipeline, imageRef string) error {
	sbomFile, err := supplychain.SBOM(ctx, p, imageRef)
	if err != nil {
		return fmt.Errorf("sbom generation: %w", err)
	}
	log("sbom generated (cyclonedx-json)")

	if p.Cfg.Sign {
		if err := supplychain.Sign(ctx, p, imageRef); err != nil {
			return fmt.Errorf("signing: %w", err)
		}
		log("image signed (cosign keyless)")
	}

	if p.Cfg.Attest {
		if err := supplychain.Attest(ctx, p, imageRef, sbomFile); err != nil {
			return fmt.Errorf("attestation: %w", err)
		}
		log("sbom and slsa provenance attached")
	}

	return nil
}

func runVerificationContracts(ctx context.Context, p *core.Pipeline) error {
	if err := supplychain.VerifySignature(ctx, p); err != nil {
		return fmt.Errorf("signature verification contract: %w", err)
	}
	if p.Cfg.CosignImagePath != "" {
		log("signature verification contract passed")
	}

	if err := supplychain.VerifyProvenance(ctx, p); err != nil {
		return fmt.Errorf("provenance verification contract: %w", err)
	}
	if p.Cfg.SLSAProvenancePath != "" && p.Cfg.SLSAArtifactPath != "" {
		log("provenance verification contract passed")
	}

	return nil
}

func log(msg string, args ...any) {
	fmt.Printf("✅ "+msg+"\n", args...)
}

func fatal(stage string, err error) {
	fmt.Fprintf(os.Stderr, "❌ %s failed: %v\n", stage, err)
	os.Exit(1)
}
