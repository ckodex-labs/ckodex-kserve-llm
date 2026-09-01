# Deep QA and stabilization evidence — 2026-08-27

## Scope

This record covers the current dirty working tree after the dependency refresh,
llm-d Router migration, and audit OTLP implementation. Existing user changes
and untracked files were preserved. No commit, reset, cleanup, or hosted
mutation was performed.

## Findings and remediations

| Finding | Remediation | Verification |
|---|---|---|
| `golangci-lint` preinstalled on the host was built with Go 1.26 and rejected the Go 1.27 module target | Built pinned `v2.13.1` from source with Go 1.27; aligned Makefile, Dagger, and release workflow to the same source-built path | Source-built `golangci-lint run ./...` returned `0 issues` |
| Repeated controller registration test reused the global `llmloraadapter` name | Kept the production controller name stable and added a unique test-only registration seam | `go test -race -count=20 ./internal/controller` passed |
| Storage transaction helper triggered path-inclusion and directory-mode findings | Used `os.OpenInRoot` for state/payload reads and changed staging permissions to `0750` | `go test -race ./cmd/storage-initializer` and source-built linter passed |
| API packages imported deprecated controller-runtime scheme builders | Added a local runtime-scheme adapter over `k8s.io/apimachinery/pkg/runtime.SchemeBuilder` | API package tests and source-built linter passed |
| Configuration and open-loop documentation exceeded repository size limits | Moved `AuditSinkConfig` to `internal/config/audit_config.go` and compressed only the affected ledger entries | `operator_config.go` is 491 LOC; `docs/open-loops.md` is 499 LOC |
| Circuit breaker timeout returned a probe without changing state | Added a lock-protected half-open transition with one concurrent probe and explicit reopen-on-failure behavior | Gateway race stress and state-transition tests passed |
| Coalescer maintenance returned empty successful results without an executor | Removed the unsafe pipeline maintenance path and documented the coalescer as a standalone primitive until a real executor exists | Inference race tests passed |
| Saga compensation errors were discarded while status was `compensated` | Added `compensation-failed` status, activity-log errors, missing-compensator detection, and wrapped error chains | Dapr tests and `make lint` passed |
| Structured audit logs omitted event details | Included redacted details in the structured sink and added a regression test | Observability race tests passed |
| Envoy AI Gateway rate-limit path was a silent no-op | Removed the dead formatting shim and added the required explicit implementation TODO | Gateway tests and lint passed |
| Release-readiness cleanup raced a concurrent Go package walk over `dist/` | Serialized artifact-producing release verification from source-tree test walks | Serialized `go test -race ./...` and `make release-readiness` both passed |

## Verification matrix

| Gate | Result | Command or artifact |
|---|---|---|
| Full Go behavior | pass | `go test -race ./...` |
| Full Go static checks | pass | `go vet -a ./...` |
| Repeated observability behavior | pass | `go test -race -count=20 ./internal/observability` |
| Repeated gateway/controller behavior | pass | gateway stress and controller `-race -count=20` runs |
| Go lint | pass | Go 1.27-built pinned `golangci-lint v2.13.1`, `0 issues` |
| Dagger module compile | pass | `go test ./...` in `dagger/` |
| Console behavior and types | pass | `npm test` 43/43, lint, TypeScript check |
| Helm contracts | pass | both chart lint runs and explicit OTLP endpoint render |
| Documentation formatting | pass | Markdown lint on 10 modified Markdown surfaces |
| Release artifact rehearsal | pass | `make release-readiness`; archives, CRD checksum, Helm package, and image tags validated |
| OTLP audit delivery | pass | `httptest` receiver accepted `/v1/logs`; `503` receiver propagated failure |
| Repeated/shuffled QA | pass | Controller, gateway, observability, storage, and Dapr runs passed with race detection; controller/observability boundary runs repeated 20/50 times |
| Makefile lint lane | pass | `make lint` uses the pinned Go-built linter path and returned `0 issues` |

## Hosted corroboration

The live Nightly run [33081386331](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33081386331) ran from `origin/main` at `c3f6b83` on 2026-08-27. KIND bootstrap failed before E2E: kubeadm could not decode `etcd.local.extraArgs` because the checked-in main configuration serialized it as an array rather than the required map. The run collected failure artifacts and tore down the cluster; no runtime acceptance was established. The pending checkout contains the map-shaped configuration, but that fix has not yet been committed or hosted-verified.

## Remaining boundary

The local result does not close hosted acceptance, fresh KIND acceptance,
actual Dagger engine execution, external collector retention/alerting, signed
audit-chain verification, or GPU/LMCache failover evidence. The host's
pre-existing Dagger process was preserved; it was not killed or reclassified as
a failed product run.
