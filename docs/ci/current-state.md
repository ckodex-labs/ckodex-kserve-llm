# CI/CD Current State — ckodex-kserve-llm-operator

Last updated: 2026-09-01

---

## Pipeline Overview

The CI/CD pipeline has one implementation:

| Layer | Entry | Pattern | Status |
|-------|-------|---------|--------|
| Dagger module (current GHA) | `dagger call <func>` | Generated Dagger Module SDK (`dag` global) | Active |

The CI implementation is active, but beta acceptance is not fully closed.
**C — hosted evidence:** CI run [33455614072](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33455614072)
passed on merge commit `eccb4d71d0229fad6abb7740738af16926e466ac`.
**C — hosted failure:** Nightly run [33081386331](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33081386331)
failed on the same commit during kubeadm bootstrap. **S — acceptance pending:**
**C — release evidence:** tagged release [v0.18.0-rc.7](https://github.com/ckodex-labs/ckodex-kserve-llm/releases/tag/v0.18.0-rc.7)
passed verification, publication, provenance, chart packaging, and anonymous
artifact acceptance in [run 33457020052](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33457020052).
**S — runtime acceptance pending:** the Nightly KIND and live model-profile gates
remain separate from release publication. See [the evidence record](hosted-exact-head-2026-08-28.md).

---

## Triggers

| Workflow | File | Trigger |
|----------|------|---------|
| CI | `.github/workflows/ci.yml` | push to main/release/**, PR to main |
| Release | `.github/workflows/release.yml` | push tag `v*` |

---

## CI Pipeline Stages (`ci.yml`)

```
Checkout
→ Setup Go (go-version-file: go.mod)
→ Require tracked console source and Dockerfile
→ Setup Node.js (Node 22)
→ Console test + lint + typecheck + webpack build
→ Console SSR + populated-state verification
→ Console container build
→ Console container HIGH/CRITICAL vulnerability scan
→ Setup Helm
→ Install GoReleaser v2.18.0
→ Pre-pull Dagger engine v0.21.9
→ Run CI Pipeline (dagger call all --source=. ; lint + non-release compile check)
   ├── lint (golangci-lint v2.13.1 source-built with the pinned Go builder, full configured surface including tests)
   └── build-check (operator compile check, linux/amd64)
→ Run race-enabled tests + statement-weighted 80% package-family coverage gates
→ Run vulnerability scan (dagger call scan --source=. ; full image rootfs + Trivy)
→ Scan the Hugging Face initializer image (amd64)
→ Run bidirectional conformance vectors
→ Run the envtest integration suite
→ Rehearse release (make release-readiness → bin/release-readiness.json)
→ Upload dist/ + bin/release-readiness.json (artifact, 30d)
```

## Release Pipeline Stages (`release.yml`)

```
Tag push (v*)
→ verify release ref/checkout identity
→ require successful hosted CI checks on the exact tag SHA
→ verify (lint + go test ./... + make release-readiness + manager version injection)
→ image-release
   ├── Install Dagger CLI v0.21.9
   ├── Install Cosign v3.1.3
   ├── Log in to GHCR
   ├── Generate Dagger SDK (dagger develop)
   ├── Build, scan, and publish image (dagger call publish → digest)
   ├── Sign image with cosign OIDC (cosign sign --yes <ref>@<digest>)
   ├── Generate SBOM (dagger call sbom → sbom/sbom.cdx.json)
   └── Upload SBOM artifact (90d retention)
→ console-image-release
   ├── Build multi-architecture standalone Next.js image
   ├── Attach BuildKit provenance and SBOM
   ├── Scan published image for unfixed HIGH/CRITICAL vulnerabilities
   └── Sign console image with cosign OIDC
→ binary-release (GoReleaser v2.18.0 → GitHub release assets and checksums)
→ image-provenance (slsa-framework/slsa-github-generator container@v2.1.0)
→ console-image-provenance (slsa-framework/slsa-github-generator container@v2.1.0)
→ binary-provenance (slsa-framework/slsa-github-generator generic@v2.1.0)
→ helm-release (helm push oci://ghcr.io/<owner>/charts)
→ public-release-contract (anonymous operator, console, initializer, and chart retrieval)
```

---

## Tool Versions (pinned)

| Tool | Version | Source |
|------|---------|--------|
| Go | `go.mod` (`go 1.26.0`, `toolchain go1.26.7`) | go.mod |
| Dagger CLI | v0.21.9 | installed |
| golangci-lint | v2.13.1 (source-built in the Go 1.27 Dagger builder) | `dagger/constants.go`, `dagger/bases.go` |
| Trivy | 0.72.0 | `dagger/constants.go` |
| Cosign | v3.1.3 | `dagger/constants.go` |
| Lula | v0.16.0 | `dagger/module.go` |
| GoReleaser | v2.18.0 | `.github/workflows/ci.yml:53` |
| Helm | v5.0.1 action | workflows |

---

## Dagger Module Functions (dagger call)

Implemented across responsibility-named files in `dagger/` (`module.go`, `build.go`,
`policy.go`, and related helpers). Requires `dagger develop` for first-time setup.

| Function | Invocation | Output |
|----------|-----------|--------|
| `lint` | `dagger call lint --source=.` | string (pass/fail) |
| `test` | `dagger call test --source=.` | string (test output + coverage gates) |
| `coverage` | `dagger call coverage --source=. export --path=coverage.out` | File |
| `build` | `dagger call build --source=. --version=dev` | Container |
| `scan` | `dagger call scan --source=.` | string (trivy output) |
| `sbom` | `dagger call sbom --source=. --image-ref=<ref> export --path=sbom.cdx.json` | File |
| `lula` | `dagger call lula --source=. export --path=assessment-results.yaml` | File |
| `publish` | `dagger call publish --source=. --image-ref=... --version=... --registry-username=... --registry-token=env:GITHUB_TOKEN` | string (digest) |
| `all` | `dagger call all --source=.` | string (hosted fast lint + build-check pass/fail) |

`lula` always lints the linked validation definitions and runs offline positive
and negative IA-9 fixtures. Its exported OSCAL assessment evaluates the live
cluster when Kubernetes credentials are available; without them, controls are
reported as `not-satisfied`.

---

## Required Integration Evidence

`dagger call integration --source=.` installs the pinned envtest helper, obtains
Kubernetes 1.35 test assets, and runs `test/integration/...` with
`REQUIRE_ENVTEST=1`. The gate fails when those assets cannot be obtained; it does
not convert a skipped envtest suite into a successful CI result.

## Nightly KIND Chaos

`.github/workflows/nightly-chaos.yml` runs daily and on demand in a disposable
KIND cluster. It uses `run/e2e.sh`, executes the E2E lifecycle suite, deletes the
operator pod, and proves the Deployment supplies a new ready pod. Failed runs retain
cluster resources, events, and controller logs for 14 days; the cluster is always torn down.
KIND v0.33.0 uses the digest-pinned Kubernetes v1.36.4 image from
`deploy/kind/acceptance-node-image.txt`; local setup and Make consume the same file.

| Package | Threshold | Rationale |
|---------|-----------|-----------|
| `internal/controller` | 80% | statement-weighted across controller packages |
| `internal/gateway` | 80% | standard |
| `internal/storage` | 80% | standard |
| `internal/auth` | 80% | standard |
| `internal/inference` | 80% | standard |
| `internal/observability` | 80% | standard |

---

## Known Issues

The historical IPv6 test failures were confirmed as sandbox-imposed socket
restrictions and are not current code defects. They must not be used to infer
that every beta gate is green: the current release assessment still has open
hosted and live-environment gates. See `docs/beta/readiness-ledger.md` and
`docs/open-loops.md` for the authoritative evidence state.

---

## Open Loops

See `docs/open-loops.md` for tracked deferrals. The CI implementation and supply
chain wiring are in place, while exact-head hosted verification, Nightly KIND
acceptance, public artifact alignment, live runtime acceptance, and provenance
verification remain tracked beta gates.

Done: L-CI-001..004, L-SC-001..002, L-OP-001..005.

---

## Architecture Reference

- `dagger/module.go`, `dagger/policy.go`, `dagger/build.go` — Dagger Module functions (ADR-008)
- `dagger.json` — Dagger Module manifest
