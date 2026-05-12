// Package security contains the Dagger security and compliance stages.
package security

import (
	"context"
	"fmt"

	"dagger.io/dagger"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/build"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
)

// Scan runs the vulnerability scan over the built image.
func Scan(ctx context.Context, p *core.Pipeline) (string, error) {
	img := build.DockerImageLocal(p)
	imgTar := img.AsTarball()

	return p.Client.Container().
		From(fmt.Sprintf("aquasec/trivy:%s", core.TrivyVersion)).
		WithMountedFile("/image.tar", imgTar).
		WithExec([]string{
			"trivy", "image",
			"--input", "/image.tar",
			"--severity", "CRITICAL,HIGH",
			"--scanners", "vuln",
			"--exit-code", "1",
			"--ignore-unfixed",
			"--format", "table",
		}).
		Stdout(ctx)
}

// Lula runs the OSCAL validation step and returns the assessment file.
func Lula(_ context.Context, p *core.Pipeline) (*dagger.File, error) {
	// Lula validates security controls and generates OSCAL assessment results.
	cmd := fmt.Sprintf(`set -eu
apk add --no-cache curl ca-certificates coreutils >/dev/null
curl -fsSL -o /tmp/lula %q
curl -fsSL -o /tmp/checksums.txt %q
expected="$(grep "  %s$" /tmp/checksums.txt | awk '{print $1}')"
echo "${expected}  /tmp/lula" | sha256sum -c -
install -m 0755 /tmp/lula /usr/local/bin/lula
lula validate -f lula/lula-component.yaml -o assessment-results.yaml`, core.LulaBinaryURL, core.LulaChecksumsURL, core.LulaBinaryName)

	assessment := p.Client.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).
		From("alpine:3.20").
		WithMountedDirectory("/src", p.Source).
		WithWorkdir("/src").
		WithExec([]string{"sh", "-lc", cmd}).
		File("assessment-results.yaml")

	return assessment, nil
}
