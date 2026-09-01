# Orchestrated remediation waves — 2026-08-28

## Run identity

Two remediation waves were coordinated through Orca Run
`run_688bd1913295`. Workers used disjoint scopes in the registered integration
worktree `/Users/mchorfa/Documents/projects/runbase/ckodex-kserve-llm`.
The earlier dispatch attempt was rejected and stopped because Orca resolved the
wrong `tensor-prime` worktree; no worker edits were accepted from that attempt.

## Worker outcomes

| Workstream | Task | Outcome | Material result |
|---|---|---|---|
| Multi-engine contract | `task_43690402b24a` + `task_57c563a049f8` | completed | Added immutable runtime registry and bidirectional registry/schema tests; admitted verified vLLM and SGLang served-tier adapters; unregistered Quant-CPP/GGUF/llama.cpp fails closed |
| Access plane | `task_a6c9d6c66566` + `task_87f29d6121ff` | completed | Added bounded, non-mutating tenant/model admission evaluator and an optional RequestPipeline policy gate; no non-test production caller exists |
| Cache/P-D/recovery | `task_f8bf01fdc5a0` | completed | Added fail-closed cache quantity validation, stale-node cleanup, terminal session release, endpoint rebinding recovery, and paired tests |
| Observability/proof | `task_b38691e0b1ce` | completed | Added content-free Ed25519 receipt verification, chain commitments, producer binding, sticky evidence readiness, and paired tests; Rekor/SVID acquisition remains open |
| CI/release acceptance | `task_631dccdd61ac` | blocked/abandoned | Static workflow/KIND/release changes were preserved; final Go verification was blocked by the host toolchain cache/approval boundary, so no CI claim is attached |
| Integration review | `task_cc6c4ba4b7f2` | completed | Confirmed ownership overlaps, required integration order, missing vectors, and hosted/live release gates |

## Coordinator corrections

- Fixed three receipt error-wrapping lint findings.
- Reconciled the stable v1 engine enum and regenerated CRD/deepcopy artifacts.
- Removed dead Quant-CPP image configuration from operator defaults and both
  Helm release surfaces.
- Refreshed the SGLang adapter from `v0.5.16` to current upstream `v0.5.18`,
  verified its manifest digest, and aligned the image allowlists.
- Corrected generated YAML/Markdown indentation defects found by Helm and lint.

## Verification record

| Gate | Result |
|---|---|
| Full Go race suite | pass: `go test -race ./...` |
| Full Go vet | pass: `go vet -a ./...` |
| Go lint | pass: `make lint`, `0 issues` |
| Engine/conformance stress | pass: focused vLLM/SGLang registry/conformance/runtime tests repeated 10 times |
| Workflow syntax | pass: `actionlint` on CI, Nightly, Pages, and release workflows |
| Helm | pass: both chart lint targets; icon recommendation only |
| Markdown | pass: 21 modified documentation surfaces |
| Release rehearsal | pass: `make release-readiness`, six archives, CRD checksum, chart package, image-tag contract |
| Hosted exact-head | partial: prior CI/CodeQL pass recorded; latest Nightly failed before E2E and aligned fix remains unhosted |
| Real GPU/LMCache | unverified: no suitable live hardware/cluster evidence |

## Current boundary

These waves improve correctness and evidence density. They do not complete
llama.cpp/TensorRT-LLM adapters, a non-test production request-plane caller for
cross-model routing, tenant-fair queue execution, durable execution of the
residency FSM effects, live P/D or LMCache failover, or hosted/public release
acceptance.
The worktree remains intentionally uncommitted: `HEAD` is
`2ef53bd55156f21b9abb4f446d7d3409e239ecac`, `origin/main` is
`c3f6b83d932ca0779339e73df67b8866e0157806`, and the coordinator observed 150
modified tracked files plus 27 untracked files. No `git add -A`, commit, push,
tag, or publish was performed.
