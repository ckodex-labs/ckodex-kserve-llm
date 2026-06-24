# CI/CD Current State — ckodex-kserve-llm-operator

Last updated: 2026-06-13

---

## Pipeline Overview

The CI/CD pipeline has two layers:

| Layer | Entry | Pattern | Status |
|-------|-------|---------|--------|
| Standalone (current GHA) | `go run ./ci/main.go` | Dagger standalone SDK (`dagger.Connect`) | Active |
| Module (dagger call) | `dagger call <func>` | Dagger Module SDK (`dag` global) | Added (ADR-008) |

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
→ Pre-pull Dagger engine v0.21.4
→ Run CI Pipeline (dagger call all --source=. ; lint + non-release compile check)
   ├── lint (golangci-lint v2.4.0 fast-only)
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
   ├── Install Dagger CLI v0.21.4
   ├── Install Cosign v3.0.4
   ├── Log in to GHCR
   ├── Generate Dagger SDK (dagger develop)
   ├── Build, scan, and publish image (dagger call publish → digest)
   ├── Sign image with cosign OIDC (cosign sign --yes <ref>@<digest>)
   ├── Generate SBOM (dagger call sbom → sbom/sbom.cdx.json)
   └── Upload SBOM artifact (90d retention)
→ binary-release (GoReleaser v2.15.4 → GitHub Release draft)
→ image-provenance (slsa-framework/slsa-github-generator container@v2.0.0)
→ binary-provenance (slsa-framework/slsa-github-generator generic@v2.0.0)
→ helm-release (helm push oci://ghcr.io/<owner>/charts)
```

---

## Tool Versions (pinned)

| Tool | Version | Source |
|------|---------|--------|
| Go | `go.mod` (1.25.0) | go.mod |
| Dagger SDK | v0.20.2 | go.mod |
| Dagger CLI | v0.21.4 | installed |
| golangci-lint | v2.4.0 | `ci/pkg/core.go` |
| Trivy | 0.69.3 | `ci/pkg/core.go` |
| Syft | v1.42.4 | `ci/pkg/core.go` |
| Cosign | v3.0.4 | `ci/pkg/core.go` |
| Lula | v0.16.0 | `ci/pkg/core.go` |
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

---

## Coverage Thresholds

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

Next: **L-CI-002** (P3) — retire `ci/main.go` after one stable release cycle.

Done (2026-06-13/14): L-CI-001, L-CI-003, L-CI-004, L-SC-001, L-SC-002, L-OP-001..004

---

## Architecture Reference

- `ci/main.go` — standalone pipeline entrypoint (flags → Dagger stages)
- `ci/pkg/core/core.go` — Pipeline struct, base containers, tool version constants
- `ci/pkg/lint/lint.go` — go vet + golangci-lint
- `ci/pkg/test/test.go` — go test + coverage gate
- `ci/pkg/build/build.go` — multi-arch image build + export/publish
- `ci/pkg/security/security.go` — Trivy scan + Lula OSCAL
- `ci/pkg/supplychain/supplychain.go` — SBOM, sign (cosign), attest, verify
- `dagger/main.go` — Dagger Module functions (ADR-008)
- `dagger.json` — Dagger Module manifest
