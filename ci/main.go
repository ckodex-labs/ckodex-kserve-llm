// CKodex KServe LLM Operator — Dagger CI/CD Pipeline
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
		Exclude: []string{".git", "bin", "ci"},
	})

	p := &core.Pipeline{Client: client, Source: source, Cfg: cfg}

	// Lint — always run
	if _, err := lint.Lint(ctx, p); err != nil {
		fatal("lint", err)
	}
	log("lint passed")

	// Test + coverage gate
	if !cfg.SkipTests {
		if _, err := test.Test(ctx, p); err != nil {
			fatal("test", err)
		}
		log("tests passed (coverage gates met)")
	}

	// Build multi-arch image
	imageRef, err := build.Build(ctx, p)
	if err != nil {
		fatal("build", err)
	}
	log("build passed → %s", imageRef)

	// Vulnerability scan
	if !cfg.SkipScan {
		if _, err := security.Scan(ctx, p); err != nil {
			fatal("security scan", err)
		}
		log("trivy scan passed (no CRITICAL/HIGH unfixed CVEs)")
	}

	// Lula OSCAL Validation
	if _, err := security.Lula(ctx, p); err != nil {
		fatal("lula validation", err)
	}
	log("lula oscal validation passed")

	// Supply Chain Security (SBOM + Sign + Attest)
	if !cfg.SkipScan {
		sbomFile, err := supplychain.SBOM(ctx, p)
		if err != nil {
			fatal("sbom generation", err)
		}
		log("sbom generated (cyclonedx-json)")

		if cfg.Sign {
			if err := supplychain.Sign(ctx, p, imageRef); err != nil {
				fatal("signing", err)
			}
			log("image signed (cosign keyless)")
		}

		if cfg.Attest {
			if err := supplychain.Attest(ctx, p, imageRef, sbomFile); err != nil {
				fatal("attestation", err)
			}
			log("sbom and slsa provenance attached")
		}
	}
	log("pipeline complete")
}

func parseFlags() *core.Config {
	cfg := &core.Config{}
	flag.StringVar(&cfg.ImageRef, "image", "", "Image reference to build/push (e.g. ghcr.io/org/app:v1.0.0)")
	flag.BoolVar(&cfg.Push, "push", false, "Push image to registry after build")
	flag.BoolVar(&cfg.Sign, "sign", false, "Sign image with cosign keyless (requires OIDC env)")
	flag.BoolVar(&cfg.Attest, "attest", false, "Attach SBOM + SLSA provenance attestations")
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

func log(msg string, args ...any) {
	fmt.Printf("✅ "+msg+"\n", args...)
}

func fatal(stage string, err error) {
	fmt.Fprintf(os.Stderr, "❌ %s failed: %v\n", stage, err)
	os.Exit(1)
}
