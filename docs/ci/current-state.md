# CI/CD Current State — ckodex-kserve-llm-operator

Last updated: 2026-07-09

---

## Pipeline Overview

The CI/CD pipeline has one implementation:

| Layer | Entry | Pattern | Status |
|-------|-------|---------|--------|
| Dagger module (current GHA) | `dagger call <func>` | Generated Dagger Module SDK (`dag` global) | Active |

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
→ Detect console (skipped: gitlink without .gitmodules)
→ Setup Helm
→ Install GoReleaser v2.15.4
→ Pre-pull Dagger engine v0.21.7
→ Run CI Pipeline (dagger call all --source=. ; lint + non-release compile check)
   ├── lint (golangci-lint v2.12.2 fast-only)
   └── build-check (operator compile check, linux/amd64)
→ Run vulnerability scan (dagger call scan --source=. ; full image rootfs + Trivy)
→ Rehearse release (make release-readiness → bin/release-readiness.json)
→ Upload dist/ + bin/release-readiness.json (artifact, 30d)
```

## Release Pipeline Stages (`release.yml`)

```
Tag push (v*)
→ verify (lint + go test ./... + make release-readiness)
→ image-release
   ├── Install Dagger CLI v0.21.7
   ├── Install Cosign v3.1.1
   ├── Log in to GHCR
   ├── Generate Dagger SDK (dagger develop)
   ├── Build, scan, and publish image (dagger call publish → digest)
   ├── Sign image with cosign OIDC (cosign sign --yes <ref>@<digest>)
   ├── Generate SBOM (dagger call sbom → sbom/sbom.cdx.json)
   └── Upload SBOM artifact (90d retention)
→ binary-release (GoReleaser v2.15.4 → GitHub release assets and checksums)
→ image-provenance (slsa-framework/slsa-github-generator container@v2.0.0)
→ binary-provenance (slsa-framework/slsa-github-generator generic@v2.0.0)
→ helm-release (helm push oci://ghcr.io/<owner>/charts)
```

---

## Tool Versions (pinned)

| Tool | Version | Source |
|------|---------|--------|
| Go | `go.mod` (1.26.5) | go.mod |
| Dagger CLI | v0.21.7 | installed |
| golangci-lint | v2.12.2 | `dagger/main.go` |
| Trivy | 0.72.0 | `dagger/main.go` |
| Cosign | v3.1.1 | `dagger/main.go` |
| Lula | v0.16.0 | `dagger/main.go` |
| GoReleaser | v2.15.4 | `.github/workflows/ci.yml:53` |
| Helm | latest (azure/setup-helm@v4) | workflows |

---

## Dagger Module Functions (dagger call)

Implemented in `dagger/main.go`. Requires `dagger develop` for first-time setup.

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

## Coverage Thresholds

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

| Package | Threshold | Rationale |
|---------|-----------|-----------|
| `internal/controller` | 27% | envtest (Kubernetes API) required for full path coverage |
| `internal/gateway` | 80% | standard |
| `internal/storage` | 80% | standard |
| `internal/auth` | 80% | standard |
| `internal/inference` | 80% | standard |
| `internal/observability` | 80% | standard |

---

## Known Issues

| ID | Issue | Package | Root Cause |
|----|-------|---------|------------|
No known failing tests. All 5 previously listed failures were sandbox-imposed socket
restrictions, not code defects. Confirmed passing outside sandbox (2026-06-14).
CI (linux/amd64) has always been clean.

---

## Open Loops

See `docs/open-loops.md` for tracked deferrals. All CI/supply-chain loops are closed.

Done: L-CI-001..004, L-SC-001..002, L-OP-001..005.

---

## Architecture Reference

- `dagger/main.go` — Dagger Module functions (ADR-008)
- `dagger.json` — Dagger Module manifest
