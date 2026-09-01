# Changelog

## v0.18.0-rc.7 — 2026-09-01

### Added

- An opt-in, revision-pinned tiny `glm5_next` fixture for small-machine
  architecture and operator-contract testing.
- A weight-free `glm5_next` preflight and a local CPU evidence record.

### Fixed

- Aligned the root and Dagger Go modules with the Go 1.26.7 contract supported
  by Dagger v0.21.9.
- Tidied the Go module graph so hosted Dagger verification runs with
  `-mod=readonly`.
- Removed the duplicate `crd-bundle` Makefile target.

### Verification

- Hosted release workflow [33457020052](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33457020052)
  passed release verification, signed image and binary publication, chart
  publication, provenance generation, and anonymous artifact acceptance.
- Local evidence: [tiny GLM CPU smoke](docs/evidence/glm5-next-tiny-local-2026-08-31.md).

### Boundaries

The tiny GLM fixture preserves the `glm5_next` architecture family at reduced
dimensions. It does not establish full GLM-5.3 model quality, NVFP4 execution,
GPU performance, long-context behavior, or distributed inference.
