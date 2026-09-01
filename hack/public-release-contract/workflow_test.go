/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type releaseWorkflow struct {
	Env  map[string]string      `yaml:"env"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Needs       []string          `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	Outputs     map[string]string `yaml:"outputs"`
	Steps       []workflowStep    `yaml:"steps"`
	Uses        string            `yaml:"uses"`
}

type workflowStep struct {
	Run  string            `yaml:"run"`
	Uses string            `yaml:"uses"`
	Env  map[string]string `yaml:"env"`
}

func TestReleaseWorkflowRunsAnonymousContractAfterPublishing(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var workflow releaseWorkflow
	require.NoError(t, yaml.Unmarshal(content, &workflow))
	assert.Equal(t, "${{ github.ref_name }}", workflow.Env["RELEASE_VERSION"])
	job, found := workflow.Jobs["public-release-contract"]
	require.True(t, found, "release workflow has no public-release-contract job")
	assert.ElementsMatch(t, []string{
		"image-release",
		"console-image-release",
		"image-provenance",
		"console-image-provenance",
		"hf-initializer-provenance",
		"helm-release",
	}, job.Needs)
	assert.Equal(t, map[string]string{"contents": "read"}, job.Permissions)
	consoleImageJob, found := workflow.Jobs["console-image-release"]
	require.True(t, found, "release workflow has no console-image-release job")
	assert.Equal(t, "ghcr.io/${{ github.repository_owner }}/ckodex-kserve-llm-console", consoleImageJob.Outputs["image-name"])
	assert.Contains(t, consoleImageJob.Steps[len(consoleImageJob.Steps)-1].Run, "COSIGN_REF")
	assert.Contains(t, workflow.Jobs["console-image-provenance"].Uses, "generator_container_slsa3.yml")
	assert.Equal(t, "ghcr.io/${{ github.repository }}", workflow.Jobs["image-release"].Outputs["image-name"])
	verifyJob, found := workflow.Jobs["verify"]
	require.True(t, found, "release workflow has no verify job")
	require.NotEmpty(t, stepContaining(verifyJob.Steps, "git ls-files --error-unmatch console/package.json"), "release verification must require ordinary console source files")
	exactHead := stepContaining(verifyJob.Steps, "commits/${GITHUB_SHA}/check-runs")
	require.NotEmpty(t, exactHead, "release verification must require hosted CI on the exact release head")
	assert.Contains(t, exactHead, "Lint + Build + Scan")
	assert.Contains(t, exactHead, "Hugging Face initializer (arm64)")
	versionContract := stepContaining(verifyJob.Steps, "manager-version-contract")
	require.NotEmpty(t, versionContract, "release verification must execute the version-injected manager")
	assert.Contains(t, versionContract, "internal/version.Version=${RELEASE_VERSION}")
	imagePublish := stepContaining(workflow.Jobs["image-release"].Steps, "dagger call publish")
	assert.Contains(t, imagePublish, "--version=\"$IMAGE_VERSION\"")
	helmJob, found := workflow.Jobs["helm-release"]
	require.True(t, found, "release workflow has no helm-release job")
	helmPackage := stepContaining(helmJob.Steps, "helm package deploy/helm")
	require.NotEmpty(t, helmPackage)
	assert.Contains(t, helmPackage, "--version \"${chart_version}\"")
	assert.Contains(t, helmPackage, "--app-version \"${RELEASE_VERSION}\"")
	assert.Contains(t, helmPackage, "helm template release \"$chart_package\"")
	assert.Contains(t, helmPackage, "ghcr.io/ckodex-labs/ckodex-kserve-llm-huggingface-initializer:${RELEASE_VERSION}")

	assertAnonymousContractSafety(t, job)
}

func assertAnonymousContractSafety(t *testing.T, job workflowJob) {
	t.Helper()
	command := contractCommand(job.Steps)
	require.NotEmpty(t, command)
	for _, argument := range []string{
		"--repository=",
		"--console-repository=",
		"--chart-repository=",
		"--version=",
		"--operator-digest=",
		"--console-digest=",
		"--initializer-digest=",
	} {
		assert.Contains(t, command, argument)
	}
	assert.NotContains(t, command, "GITHUB_TOKEN")
	for _, step := range job.Steps {
		assert.NotContains(t, step.Uses, "docker/login-action")
		assert.NotContains(t, step.Run, "docker login")
		assert.NotContains(t, step.Run, "helm registry login")
		assert.NotContains(t, step.Env, "GITHUB_TOKEN")
	}
}

func contractCommand(steps []workflowStep) string {
	return stepContaining(steps, "go run ./hack/public-release-contract")
}

func stepContaining(steps []workflowStep, needle string) string {
	for _, step := range steps {
		if strings.Contains(step.Run, needle) {
			return step.Run
		}
	}
	return ""
}
