package supplychain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"dagger.io/dagger"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
)

func SBOM(ctx context.Context, p *core.Pipeline) (*dagger.File, error) {
	ctr := p.Client.Container().
		From(fmt.Sprintf("aquasec/trivy:%s", core.TrivyVersion)).
		WithMountedDirectory("/src", p.Source).
		WithWorkdir("/src").
		WithExec([]string{
			"trivy", "image",
			"--format", "cyclonedx",
			"--output", "sbom.cdx.json",
			"ghcr.io/ckodex-labs/ckodex-kserve-llm:dev",
		})

	return ctr.File("sbom.cdx.json"), nil
}

func Sign(ctx context.Context, p *core.Pipeline, imageRef string) error {
	idToken := os.Getenv("SIGSTORE_ID_TOKEN")
	if idToken == "" {
		idToken = os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	}

	ctr := p.Client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", core.CosignVersion)).
		WithEnvVariable("COSIGN_YES", "true")

	if idToken != "" {
		ctr = ctr.WithSecretVariable("SIGSTORE_ID_TOKEN",
			p.Client.SetSecret("sigstore-id-token", idToken))
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

func Attest(ctx context.Context, p *core.Pipeline, imageRef string, sbomFile *dagger.File) error {
	idToken := os.Getenv("SIGSTORE_ID_TOKEN")

	// 1. Attach SBOM attestation.
	ctr := p.Client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", core.CosignVersion)).
		WithEnvVariable("COSIGN_YES", "true").
		WithMountedFile("/sbom.cdx.json", sbomFile)

	if idToken != "" {
		ctr = ctr.WithSecretVariable("SIGSTORE_ID_TOKEN",
			p.Client.SetSecret("sigstore-id-token-attest", idToken))
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
	provenance := slsaProvenance(imageRef, p.Cfg.GitCommit, p.Cfg.GitRepoURL)
	provJSON, err := json.Marshal(provenance)
	if err != nil {
		return fmt.Errorf("marshal slsa provenance: %w", err)
	}

	if err := os.WriteFile("slsa-provenance.json", provJSON, 0o600); err != nil {
		return fmt.Errorf("write slsa provenance: %w", err)
	}

	provFile := p.Client.Host().File("slsa-provenance.json")
	ctr2 := p.Client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", core.CosignVersion)).
		WithEnvVariable("COSIGN_YES", "true").
		WithMountedFile("/provenance.json", provFile)

	if idToken != "" {
		ctr2 = ctr2.WithSecretVariable("SIGSTORE_ID_TOKEN",
			p.Client.SetSecret("sigstore-id-token-prov", idToken))
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
