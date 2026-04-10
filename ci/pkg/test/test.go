package test

import (
	"context"
	"fmt"

	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
)

func Test(ctx context.Context, p *core.Pipeline) (string, error) {
	out, err := p.GoBase().
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
				core.CoverageController, core.CoverageGateway, core.CoverageStorage,
				core.CoverageAuth, core.CoverageInference, core.CoverageObservability,
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
  # Run go test -cover for the specific package to get weighted package-level coverage.
  pct=$(go test -cover "./internal/${pkg}" | grep -oE "coverage: [0-9.]+" | awk '{print int($2)}')
  if [ -z "$pct" ]; then pct=0; fi
  echo "Coverage internal/${pkg}: ${pct}%% (min: ${min}%%)"
  if [ "$pct" -lt "$min" ]; then
    echo "FAIL: internal/${pkg} coverage ${pct}%% < ${min}%% threshold" >&2; exit 1
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
