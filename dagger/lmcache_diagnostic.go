package main

import (
	"context"
	"fmt"
	"regexp"

	"dagger/ckodex-operator/internal/dagger"
)

const (
	lmcacheDiagnosticWheelName = "lmcache-0.0.0+pr4467.e05c2ea-cp312-cp312-linux_x86_64.whl"
	lmcacheDiagnosticWheelSHA  = "4c936485327e1152667290fe83cd27f08379c7edbf628272a6d011d5fcbbd21b"
	lmcacheBackportWheelName   = "lmcache-0.5.6+ckodex.pr4467-cp312-cp312-linux_x86_64.whl"
	lmcacheBackportWheelSHA    = "b00f42303684cf7d572cc356b39d82c9120686ded916c27820106585a517a6d2"
	lmcacheDiagnosticBase      = "vllm/vllm-openai@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e"
)

// BuildLMCachePR4467Diagnostic installs the exact locally tested development
// wheel into the pinned amd64 vLLM image. This unsigned image is diagnostic
// only: it is not a v0.5.6 backport or a production-qualified release. The
// supplied wheel was built for CUDA compute capability 8.9.
func (m *CkodexOperator) BuildLMCachePR4467Diagnostic(ctx context.Context, wheel *dagger.File) (*dagger.Container, error) {
	return buildLMCacheDiagnostic(ctx, wheel, lmcacheDiagnosticWheelName, lmcacheDiagnosticWheelSHA)
}

// BuildLMCacheV056BackportDiagnostic installs the locally built v0.5.6
// backport wheel into the pinned amd64 image. This unsigned candidate is for
// testing on CUDA compute capability 8.9, not a qualified upstream release.
func (m *CkodexOperator) BuildLMCacheV056BackportDiagnostic(ctx context.Context, wheel *dagger.File) (*dagger.Container, error) {
	return buildLMCacheDiagnostic(ctx, wheel, lmcacheBackportWheelName, lmcacheBackportWheelSHA)
}

// BuildLMCacheV056BackportCandidate installs a caller-identified local
// backport wheel. The supplied digest identifies the input, not its source,
// provenance, or qualification; the resulting image requires fresh testing.
func (m *CkodexOperator) BuildLMCacheV056BackportCandidate(ctx context.Context, wheel *dagger.File, wheelSHA256 string) (*dagger.Container, error) {
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(wheelSHA256) {
		return nil, fmt.Errorf("wheel SHA-256 must be 64 lowercase hexadecimal digits")
	}
	return buildLMCacheDiagnostic(ctx, wheel, lmcacheBackportWheelName, wheelSHA256)
}

func buildLMCacheDiagnostic(ctx context.Context, wheel *dagger.File, filename, digest string) (*dagger.Container, error) {
	// Check before resolving the large runtime image. The Dagger File is immutable,
	// so the same content is mounted in the installation step below.
	check := dag.Container().From(goBuilderImage).
		WithMountedFile("/tmp/lmcache-diagnostic.whl", wheel).
		WithExec([]string{"sh", "-ec", "printf '%s  %s\\n' \"$1\" /tmp/lmcache-diagnostic.whl | sha256sum --check --strict -", "verify-wheel", digest})
	if _, err := check.Sync(ctx); err != nil {
		return nil, fmt.Errorf("verify LMCache diagnostic wheel SHA-256: %w", err)
	}

	wheelPath := "/tmp/" + filename
	return dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From(lmcacheDiagnosticBase).
		WithEnvVariable("LMCACHE_TRACK_USAGE", "false").
		WithEnvVariable("DO_NOT_TRACK", "1").
		WithMountedFile(wheelPath, wheel).
		WithExec([]string{"sh", "-ec", "PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1 python3 -m pip install --no-index --no-deps \"$1\"", "install-wheel", wheelPath}).
		WithoutMount(wheelPath).
		WithEntrypoint([]string{"vllm", "serve"}), nil
}
