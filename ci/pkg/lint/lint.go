package lint

import (
	"context"
	"fmt"

	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
)

func Lint(ctx context.Context, p *core.Pipeline) (string, error) {
	// Two-step lint: go vet (fast, built-in) + golangci-lint (comprehensive)
	goVet, err := p.GoBase().
		WithExec([]string{"go", "vet", "./..."}).
		Stdout(ctx)
	if err != nil {
		return goVet, fmt.Errorf("go vet: %w", err)
	}

	return p.GoBase().
		WithExec([]string{"go", "install", "github.com/golangci/golangci-lint/cmd/golangci-lint@" + core.GolangciLintVer}).
		WithExec([]string{
			"golangci-lint", "run", "-v",
			"--timeout", "5m",
			"--out-format", "line-number",
		}).
		Stdout(ctx)
}
