// Package supplychain contains the Dagger supply-chain security stages.
package supplychain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"dagger.io/dagger"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
)

const (
	cosignCommand           = "cosign"
	cosignYesFlag           = "--yes"
	cosignTypeFlag          = "--type"
	githubOIDCTokenEnv      = "ACTIONS_ID_TOKEN_REQUEST_TOKEN"
	githubOIDCRequestURLEnv = "ACTIONS_ID_TOKEN_REQUEST_URL"
	sigstoreIDTokenEnv      = "SIGSTORE_ID_TOKEN"
)

// SBOM generates a CycloneDX SBOM for the built image.
func SBOM(_ context.Context, p *core.Pipeline, imageRef string) (*dagger.File, error) {
	// Generate SBOM from the built image to ensure we capture the final state.
	ctr := p.Client.Container().
		From(fmt.Sprintf("aquasec/trivy:%s", core.TrivyVersion)).
		WithMountedDirectory("/src", p.Source).
		WithWorkdir("/src").
		WithExec([]string{
			"trivy", "image",
			"--format", "cyclonedx",
			"--output", "sbom.cdx.json",
			imageRef,
		})

	return ctr.File("sbom.cdx.json"), nil
}

// Sign signs the image with cosign when an OIDC token is available.
func Sign(ctx context.Context, p *core.Pipeline, imageRef string) error {
	ctr := p.Client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", core.CosignVersion)).
		WithEnvVariable("COSIGN_YES", "true")
	ctr, mode := withCosignIdentity(p, ctr, "sign")

	output, err := ctr.
		WithExec([]string{
			cosignCommand, "sign",
			cosignYesFlag,
			imageRef,
		}).
		Stdout(ctx)
	if err != nil {
		if trimmed := strings.TrimSpace(output); trimmed != "" {
			return fmt.Errorf("cosign sign (%s): %w\n%s", mode, err, trimmed)
		}
		return fmt.Errorf("cosign sign (%s): %w", mode, err)
	}
	return nil
}

// Attest attaches SBOM and SLSA provenance attestations to the image.
func Attest(ctx context.Context, p *core.Pipeline, imageRef string, sbomFile *dagger.File) error {
	// 1. Attach SBOM attestation.
	ctr := p.Client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", core.CosignVersion)).
		WithEnvVariable("COSIGN_YES", "true").
		WithMountedFile("/sbom.cdx.json", sbomFile)
	ctr, mode := withCosignIdentity(p, ctr, "attest-sbom")

	output, err := ctr.
		WithExec([]string{
			cosignCommand, "attest",
			cosignYesFlag,
			cosignTypeFlag, "cyclonedx",
			"--predicate", "/sbom.cdx.json",
			imageRef,
		}).
		Stdout(ctx)
	if err != nil {
		if trimmed := strings.TrimSpace(output); trimmed != "" {
			return fmt.Errorf("attach sbom attestation (%s): %w\n%s", mode, err, trimmed)
		}
		return fmt.Errorf("attach sbom attestation (%s): %w", mode, err)
	}

	// 2. Generate SLSA provenance predicate and attach it.
	provenance := slsaProvenance(imageRef, p.Cfg.GitCommit, p.Cfg.GitRepoURL)
	provJSON, err := json.Marshal(provenance)
	if err != nil {
		return fmt.Errorf("marshal slsa provenance: %w", err)
	}

	if writeErr := os.WriteFile("slsa-provenance.json", provJSON, 0o600); writeErr != nil {
		return fmt.Errorf("write slsa provenance: %w", writeErr)
	}

	provFile := p.Client.Host().File("slsa-provenance.json")
	ctr2 := p.Client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", core.CosignVersion)).
		WithEnvVariable("COSIGN_YES", "true").
		WithMountedFile("/provenance.json", provFile)
	ctr2, mode = withCosignIdentity(p, ctr2, "attest-provenance")

	// NOTE: This generates the SLSA v1.0 predicate for 'Manual' or 'Local' attestation.
	// In production (GHA), this is wrapped or superseded by the non-forgeable L3
	// envelope provided by the slsa-framework/slsa-github-generator.
	output, err = ctr2.
		WithExec([]string{
			cosignCommand, "attest",
			cosignYesFlag,
			cosignTypeFlag, "slsaprovenance1",
			"--predicate", "/provenance.json",
			imageRef,
		}).
		Stdout(ctx)
	if err != nil {
		if trimmed := strings.TrimSpace(output); trimmed != "" {
			return fmt.Errorf("attach slsa provenance attestation (%s): %w\n%s", mode, err, trimmed)
		}
		return fmt.Errorf("attach slsa provenance attestation (%s): %w", mode, err)
	}
	return nil
}

func withCosignIdentity(p *core.Pipeline, ctr *dagger.Container, secretNameSuffix string) (*dagger.Container, string) {
	mode, env := cosignIdentityEnv()
	switch mode {
	case sigstoreIDTokenEnv:
		idToken := env[sigstoreIDTokenEnv]
		return ctr.WithSecretVariable(sigstoreIDTokenEnv,
			p.Client.SetSecret("sigstore-id-token-"+secretNameSuffix, idToken)), mode
	case "github-actions-oidc":
		requestToken := env[githubOIDCTokenEnv]
		requestURL := env[githubOIDCRequestURLEnv]
		ctr = ctr.
			WithSecretVariable(githubOIDCTokenEnv,
				p.Client.SetSecret("github-oidc-token-"+secretNameSuffix, requestToken)).
			WithEnvVariable(githubOIDCRequestURLEnv, requestURL)
		return ctr, mode
	default:
		return ctr, mode
	}
}

func cosignIdentityEnv() (string, map[string]string) {
	if idToken := os.Getenv(sigstoreIDTokenEnv); idToken != "" {
		return sigstoreIDTokenEnv, map[string]string{
			sigstoreIDTokenEnv: idToken,
		}
	}

	requestToken := os.Getenv(githubOIDCTokenEnv)
	requestURL := os.Getenv(githubOIDCRequestURLEnv)
	if requestToken != "" && requestURL != "" {
		return "github-actions-oidc", map[string]string{
			githubOIDCTokenEnv:      requestToken,
			githubOIDCRequestURLEnv: requestURL,
		}
	}

	return "ambient", nil
}

// slsaProvenance generates a SLSA v1.0 predicate for local/development use.
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

// VerifySignature implements the COSIGN_IMAGE/COSIGN_ARTIFACT_PATH contract (Section 31.4).
func VerifySignature(ctx context.Context, p *core.Pipeline) error {
	image := p.Cfg.CosignImagePath
	if image == "" {
		return nil
	}

	ctr := p.Client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", core.CosignVersion))

	args := []string{cosignCommand, "verify", cosignYesFlag}
	if p.Cfg.CosignBundlePath != "" {
		args = append(args, "--bundle", p.Cfg.CosignBundlePath)
	}
	args = append(args, image)

	_, err := ctr.WithExec(args).Stdout(ctx)
	return err
}

// VerifyProvenance implements the SLSA_PROVENANCE_PATH contract (Section 32.1).
func VerifyProvenance(ctx context.Context, p *core.Pipeline) error {
	if p.Cfg.SLSAProvenancePath == "" || p.Cfg.SLSAArtifactPath == "" {
		return nil
	}

	ctr := p.Client.Container().
		From(fmt.Sprintf("gcr.io/projectsigstore/cosign:%s", core.CosignVersion))

	_, err := ctr.
		WithExec([]string{
			cosignCommand, "verify-attestation",
			cosignYesFlag,
			cosignTypeFlag, "slsaprovenance1",
			"--bundle", p.Cfg.SLSAProvenancePath,
			p.Cfg.SLSAArtifactPath,
		}).
		Stdout(ctx)
	return err
}
