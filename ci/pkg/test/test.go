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

// coverageGateScript returns a shell script that parses the existing coverage.out 
// and fails if any measured package falls below its threshold.
func coverageGateScript(ctrlMin, gwMin, storeMin, authMin, inferMin, obsMin int) string {
	return fmt.Sprintf(`
set -e
if [ ! -f coverage.out ]; then
  echo "FAIL: coverage.out not found" >&2; exit 1
fi

check() {
  pkg=$1; min=$2
  # Extract total coverage for the specific internal package from the profile.
  # go tool cover -func output format: <file>:<line>: <func> <percent>
  pct=$(go tool cover -func=coverage.out | grep "internal/${pkg}/" | awk '
    { sum += $NF; count++ }
    END { if (count > 0) print int(sum/count); else print 0 }
  ')
  
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
