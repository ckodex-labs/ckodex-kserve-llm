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
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Needs       []string          `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	Steps       []workflowStep    `yaml:"steps"`
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
	job, found := workflow.Jobs["public-release-contract"]
	require.True(t, found, "release workflow has no public-release-contract job")
	assert.ElementsMatch(t, []string{"image-release", "helm-release"}, job.Needs)
	assert.Equal(t, map[string]string{"contents": "read"}, job.Permissions)

	command := contractCommand(job.Steps)
	require.NotEmpty(t, command)
	for _, argument := range []string{
		"--repository=",
		"--chart-repository=",
		"--version=",
		"--operator-digest=",
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
	for _, step := range steps {
		if strings.Contains(step.Run, "go run ./hack/public-release-contract") {
			return step.Run
		}
	}
	return ""
}
