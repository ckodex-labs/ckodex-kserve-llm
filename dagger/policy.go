package main

import "fmt"

func coverageGateScript() string {
	return fmt.Sprintf(`
set -e
if [ ! -f coverage.out ]; then echo "FAIL: coverage.out not found" >&2; exit 1; fi
awk '
BEGIN {
  order = "controller gateway storage auth inference observability"
  pkgCount = split(order, pkgs, " ")
  min["controller"] = %d
  min["gateway"] = %d
  min["storage"] = %d
  min["auth"] = %d
  min["inference"] = %d
  min["observability"] = %d
}
NR == 1 && $1 ~ /^mode:/ { next }
{
  for (i = 1; i <= pkgCount; i++) {
    pkg = pkgs[i]
    if ($1 ~ "/internal/" pkg "/") {
      statements[pkg] += $2
      if ($3 > 0) {
        covered[pkg] += $2
      }
    }
  }
}
END {
  failed = 0
  for (i = 1; i <= pkgCount; i++) {
    pkg = pkgs[i]
    pct = statements[pkg] > 0 ? int((covered[pkg] * 100) / statements[pkg]) : 0
    printf("Coverage internal/%%s: %%d%%%% (min: %%d%%%%)\n", pkg, pct, min[pkg])
    if (pct < min[pkg]) {
      printf("FAIL: internal/%%s coverage %%d%%%% < %%d%%%% threshold\n", pkg, pct, min[pkg]) > "/dev/stderr"
      failed = 1
    }
  }
  exit failed
}
' coverage.out
`, coverageController, coverageGateway, coverageStorage, coverageAuth, coverageInference, coverageObs)
}

func coverageTestArgs() []string {
	return []string{"go", "test", "-coverprofile=coverage.out", "-covermode=atomic", "./..."}
}

func raceCoverageTestArgs() []string {
	return []string{"go", "test", "-race", "-p", "16", "-coverprofile=coverage.out", "-covermode=atomic", "./..."}
}

func testArgs() []string {
	return []string{"go", "test", "-short", "-p", "16", "./..."}
}
