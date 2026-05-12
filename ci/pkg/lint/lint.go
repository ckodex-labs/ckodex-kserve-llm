// Package lint contains the Dagger lint stage.
package lint

import (
	"context"
	"fmt"

	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
)

// Lint runs vet and golangci-lint for the pipeline inputs.
func Lint(ctx context.Context, p *core.Pipeline) (string, error) {
	// 1. go vet (fast, built-in)
	goVetCtr := p.GoBase().WithExec([]string{"go", "vet", "./..."})
	if _, err := goVetCtr.Sync(ctx); err != nil {
		out, _ := goVetCtr.Stdout(ctx)
		return out, fmt.Errorf("go vet: %w", err)
	}

	// 2. golangci-lint (using optimized image)
	lintCtr := p.LintBase().
		WithExec([]string{
			"golangci-lint", "run", "-v",
			"--timeout", "10m",
			"./ci/...",
		})
	if _, err := lintCtr.Sync(ctx); err != nil {
		out, _ := lintCtr.Stdout(ctx)
		return out, fmt.Errorf("golangci-lint: %w", err)
	}

	return "lint passed", nil
}
