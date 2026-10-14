# CI/CD Current State — ckodex-kserve-llm-operator

Last updated: 2026-10-14

---

## Pipeline Overview

The CI/CD pipeline has one implementation:

| Layer | Entry | Pattern | Status |
|-------|-------|---------|--------|
| Dagger module (current GHA) | `dagger call <func>` | Generated Dagger Module SDK (`dag` global) | Active |

The CI implementation is active, but beta acceptance is not fully closed.
**C — hosted evidence:** the latest CI run checked here,
[38093178306](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/38093178306),
passed on current main commit `845bd53063769815c578ae7412319c2fda66c619`
on 2026-10-10. It does not cover uncommitted checkout changes.
**C — hosted failure:** the latest Nightly KIND run checked here,
[38385225450](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/38385225450),
failed on that commit on 2026-10-13 after cluster creation, Gateway CRD setup,
image builds/loads, and operator rollout succeeded. The GPT-2 CPU smoke
workload (resource name `llama3-8b`) reached `CrashLoopBackOff` with five
vLLM restarts and never became ready. E2E lifecycle and chaos tests did not run.
The old evidence bundle lacks inference logs and container exit status, so the
exception and root cause remain unknown. The local diagnostic change collects
those details before teardown; hosted validation of that change remains pending.
See [the failure record](../evidence/nightly-kind-2026-10-13.md).
**C — release evidence:** tagged release [v0.19.0](https://github.com/ckodex-labs/ckodex-kserve-llm/releases/tag/v0.19.0)
passed verification, publication, provenance, chart packaging, and anonymous
artifact acceptance.
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
The local collector update additionally retains current/previous vLLM logs,
redacted pod describe and allowlisted termination status, and full controller
logs (`--tail=-1`) from each discovered pod/container. Every command uses the
explicit disposable KIND context and bounded timeouts. `collection.json`
reports unavailable commands, missing previous logs, and exhausted budgets.
Raw stdout/stderr, pod specs, Secrets, ConfigMaps, and kubeconfigs are not saved.
Describe spec sections are removed; known literal pod env values and common
credential patterns are masked in free text. Arbitrary unlabelled or encoded
secrets in logs cannot be guaranteed detectable; review evidence before sharing.
If pod discovery fails, only the safe collection manifest is retained.
The offline fixtures run through the existing Dagger `Test` gate; the local
command is `python3 -m unittest discover -s test/diagnostics -v`.
KIND v0.33.0 uses the digest-pinned Kubernetes v1.35.8 image from
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
chain wiring are in place, while Nightly KIND acceptance, live runtime
acceptance, downstream provenance verification, and human-access qualification
remain tracked beta gates.

Done: L-CI-001..005, L-SC-001..002, L-OP-001..005, L-REL-004.

---

## Architecture Reference

- `dagger/module.go`, `dagger/policy.go`, `dagger/build.go` — Dagger Module functions (ADR-008)
- `dagger.json` — Dagger Module manifest
