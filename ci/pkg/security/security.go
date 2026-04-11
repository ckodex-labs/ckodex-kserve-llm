package security

import (
	"context"
	"fmt"

	"dagger.io/dagger"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/build"
	"github.com/ckodex-labs/kserve-llm-operator/ci/pkg/core"
)

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

func Lula(ctx context.Context, p *core.Pipeline) (*dagger.File, error) {
	// Lula validates security controls and generates OSCAL assessment results.
	assessment := p.Client.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).
		From(core.LulaImage).
		WithMountedDirectory("/src", p.Source).
		WithWorkdir("/src").
		WithExec([]string{"lula", "validate", "-f", "lula/lula-component.yaml", "-o", "assessment-results.yaml"}).
		File("assessment-results.yaml")

	return assessment, nil
}
