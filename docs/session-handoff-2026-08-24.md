# Session Handoff — Runtime Contract and Repository Hardening

Date: 2026-08-24
Workspace: `/Users/mchorfa/Documents/projects/runbase/ckodex-kserve-llm`
Branch: `ckodex/issue-hardening-lmcache`
HEAD at handoff: `cdcbc9c` (`fix: update x/text past CVE-2026-56852`)
Upstream state: configured branch upstream is gone
Next-session rule: inspect before editing; do not reset, clean, stage, commit, or
push the mixed checkout without an explicit integration decision.

## 1. Executive state

The bounded local remediation is complete and the final local gates pass.
This does **not** establish hosted, Nightly, public-release, GPU, LMCache,
browser-assistive-technology, or provenance acceptance.

Claim classification used below:

- **C** — implemented and backed by a current local command, test, or artifact.
- **S** — design is recorded or implementation is partial; additional work or
  acceptance evidence is required.
- **A** — future direction without a complete implementation.

### C — locally established

- Go tests, uncached tests, race tests, vet, module verification, and the full
  configured Go lint surface pass.
- Dagger `all` and race/coverage `test` pass.
- Every governed coverage family is at or above 80% using statement-weighted
  coverage rather than an average of function percentages.
- Console tests, ESLint, and TypeScript checks pass.
- Helm install contracts, actionlint, JSON parsing, Markdown lint, and
  `git diff --check` pass.
- Exact AST inspection found no changed/new non-generated Go file over 500 LOC
  and no `FuncDecl` or `FuncLit` over 50 source lines.
- The sidebar split is below 500 LOC per file.
- No production `panic` call or unchecked Helm map assertion remains in the
  changed surface.
- All sub-agents observed during the session were closed. A final audit checked
  38 known IDs; every ID was already shut down or unregistered.

### S — partial or acceptance pending

- CKC-ENG is a proposed contract with a partial vLLM tier-1 adapter.
- Stable v1 conversion/admission is implemented locally; hosted exact-head
  conversion evidence remains open.
- Evidence-health evaluation exists but is not the complete CKC-OBS evidence
  plane.
- AIPack verification fails closed but does not yet perform cosign/Rekor
  cryptographic verification.
- Model admission does not yet implement the full synchronous/progressive
  two-plane design.
- Local Dagger ran with client/engine v0.21.8, while `dagger.json` and hosted
  workflows request v0.21.7. The current local traces are not proof of exact
  hosted Dagger-version parity.

### A — future work

- SGLang, llama.cpp, and TensorRT-LLM adapters.
- Governed runtime tiers above the current served-tier seam.
- Full evidence signatures, sequence/hash chains, transparency references, and
  deterministic replay bundles.

## 2. Checkout preservation boundary

The checkout was dirty before this session and remains a mixed worktree.
Do not infer that every staged, unstaged, deleted, or untracked path belongs to
this remediation.

Snapshot at handoff:

- 425 paths reported by `git status --porcelain=v1`.
- 179 paths have an index-side change.
- 91 paths have a worktree-side change.
- 184 paths are untracked.
- 11 status entries include deletion.
- `git diff --stat` covers tracked worktree differences only and reported
  91 files, 1,721 insertions, and 15,436 deletions. It does not count the full
  untracked replacement surface.

The large deletions are predominantly monolithic files that now have
responsibility-named replacements. Exact historical test-name comparison found
no behavior-preserving split loss. Two tests were intentionally renamed because
their semantics changed:

- `TestExecuteStage_UnknownType_NoOp` became a fail-closed assertion.
- `TestAIPackGovernance_FullyAttestedPack` became a predicate-present but
  cryptographically-unverified assertion.

Never use these commands as a first action:

```text
git reset --hard
git clean -fd
git checkout -- .
git add -A
git commit -am ...
```

Before any commit work, inspect all three views separately:

```bash
git status --short --branch
git diff --cached --name-status
git diff --name-status
git ls-files --others --exclude-standard
```

If the next objective is delivery through Git, first obtain an operator choice
for commit decomposition. The current index already contains extensive work and
must not be treated as a fresh staging area.

## 3. Documentation imported and reconciled

Commit `e324dd3` from
`origin/claude/codebase-health-inference-frameworks-mcoc2n` was available in the
local object database but was not part of this branch. Its additive contract
files were imported through patch application, not cherry-pick, to avoid
overwriting local documentation changes.

Added:

- `docs/engine-contract.md`
- `docs/adr/010-runtime-engine-contract.md`
- `docs/adr/011-canonical-observability-planes.md`
- `docs/adr/012-model-admission-planes.md`
- `docs/remediation-plan.md`

Merged and reconciled:

- `docs/open-loops.md`
- `docs/ci/current-state.md`
- `README.md`
- `.cspell.json`

The contract, ADRs, and remediation plan are explicitly classified **S**.
The remediation plan is marked as a historical snapshot from `4b596ef`; current
status belongs in `docs/open-loops.md` and current command evidence.

`docs/open-loops.md` is exactly 500 LOC. Do not add to it without compacting or
splitting it first.

## 4. Implemented code slices

### 4.1 CI and toolchain — C

- `.github/workflows/ci.yml` invokes the Dagger race/coverage test gate.
- `.golangci.yml` analyzes tests and enables the selected gosec families.
- Coverage calculation reads raw Go coverage statements and weights by statement
  count. It no longer averages function percentages.
- All governed package-family thresholds are 80%.
- Manager builds target `./cmd/manager`; building only `main.go` no longer drops
  the split manager files.
- GoReleaser, golangci-lint, and Dagger workflow downloads use pinned SHA-256
  values.
- Makefile tool installers are version-pinned.
- `go.mod` separates `go 1.26` from `toolchain go1.26.5`.
- The Dagger module was split into responsibility-named files under `dagger/`.

Important version evidence:

- Host Go: 1.26.7 darwin/arm64.
- Host golangci-lint: 2.13.0.
- Dagger containers use Go 1.26.5 and golangci-lint 2.12.2.
- Host Dagger client/engine: 0.21.8.
- Repository/workflow Dagger request: 0.21.7.

### 4.2 API conversion and admission — C locally, S hosted

- v1/v1alpha2 conversion preserves the divergent runtime fields.
- Conversion tests and the round-trip fuzz target pass.
- The stable v1 validator and defaulter are registered through dedicated paths.
- CRD conversion configuration and CA injection are present.
- Unknown inference engines are rejected by shared validation before workload
  reconciliation.
- CRD engine fields carry the `vllm;quant-cpp` enum.
- Webhook validation/default logic was split so all functions remain under
  50 LOC.

Hosted create/default/convert evidence is still required before closing
`L-API-001`/`L-OP-010`.

### 4.3 Runtime adapter seam — S

- `internal/runtime` defines the engine-neutral adapter shape.
- `internal/runtime/vllm` renders the current vLLM argument contract.
- The single-node deployment builder calls the vLLM adapter.
- User-supplied arguments retain precedence over adapter defaults.
- Unsupported engines no longer silently deploy as vLLM.
- Capability support uses named fields and a named conformance-tier type.

Rendered vLLM areas include:

- tensor/data/local-data/pipeline parallelism;
- expert parallelism and EPLB;
- KV-cache dtype and CPU offload;
- speculative method/token/model settings;
- quantization;
- model, host, and port defaults.

The adapter remains tier 1. The following are not behind the seam yet:

- image selection;
- metrics contract;
- receipt contract;
- health contract;
- workload-builder ownership;
- cross-engine registry and shared conformance suite.

`L-RT-001`, `L-RT-002`, `L-RT-003`, and `L-RT-004` remain the runtime roadmap.

### 4.4 AIPack and governance — C fail-closed, S cryptography

- Predicate presence alone no longer yields a positive cryptographic verdict.
- Missing cryptographic verification returns `Verified: false`.
- Pattern, VAD, lineage, and air-gap incomplete paths return explicit errors.
- Policy evaluation covers family, artifact kind, required predicate, and risk
  band constraints.
- Unknown risk bands fail closed.
- AC-4, AU-2, and SI-4 no longer report unconditional compliant status; they
  default to `Unknown/EvidenceUnavailable` when proof is absent.
- SI-7 and SR-2 remain evidence-driven and fail closed.

Still open:

- cosign verification;
- Rekor inclusion proof;
- artifact-digest binding;
- predicate payload-schema validation;
- remaining AIPACK-SPEC section implementations;
- schema admission wiring.

See `L-PACK-001` through `L-PACK-004`.

### 4.5 Observability and evidence — C helpers, S plane

- Evidence-health evaluation detects missing, unexpected, duplicate, unsigned,
  and out-of-order receipts.
- Kubernetes Event write failures are logged with action/resource context.
- Events use the audited namespace and involved-object identity.
- Configured direct OTLP export reports an explicit unavailable error; it does
  not claim delivery.
- Audit file-close and JSON-encoding failures are visible.
- Full model URIs are not sent to audit sinks. Audit details retain only a
  sanitized scheme plus `sha256:<64-hex>` reference.
- Vector sink and sidecar construction were split under the 50-line function
  limit.

Still open:

- integrity signatures and producer binding;
- sequence/hash/Merkle structures;
- content-bearing receipt-type removal;
- canonical metric/event namespace migration;
- runtime wiring for evidence-health signals;
- direct OTLP log export and profile-specific failure policy.

See `L-OBS-001` through `L-OBS-005`.

### 4.6 Controller and workload safety — C

- Primary LLM, LoRA, LocalModelCache, ModelOnboarding, manager, deployment
  builder, API types, Dagger, and large test files were split into
  responsibility-named files.
- Unknown onboarding stages fail closed.
- Status-condition write errors propagate.
- Intentional best-effort deletes/unloads log failures with context.
- LoRA breakers are synchronized and keyed by operation, namespace, target, and
  adapter. Load and unload state no longer contaminate each other.
- LocalModelCache rejects invalid node selectors.
- Direct cluster-scoped caches use the fixed default workload namespace.
- LoRA-owned cache namespace use requires matching owner namespace/name/UID and
  ownership metadata.
- Cross-namespace storage references are rejected.
- Warmup workloads run non-root with restricted security context, dropped
  capabilities, runtime-default seccomp, read-only root, and explicit writable
  mounts.
- Helm manifest validation no longer uses unchecked map assertions.

Remaining authority concern:

- The controller still has cluster-wide mutation rights for dynamic EPP
  ServiceAccounts, Roles, and RoleBindings. Reconciliation scopes objects to the
  workload namespace, but controller compromise retains a broad blast radius.
  This is tracked as `L-OP-015`.

### 4.7 Charts, release contract, and console — C locally

- Both chart install contracts derive release images from chart `appVersion`.
- The legacy beta.7 operator/initializer defaults were moved to beta.8.
- Legacy console and initializer image tags default from `Chart.appVersion`.
- Helm contract reads chart metadata dynamically instead of hardcoding beta.8.
- A byte-for-byte chart source detector was rejected and removed because the
  chart layouts intentionally differ. The executable semantic contract remains.
- Full chart consolidation is still open as `L-OP-013`.
- The 723-line sidebar was split into context, layout, menu, and public export
  modules. The design conformance test reads the implementation modules rather
  than a sentinel comment.

## 5. Final verification ledger

### Dagger authoritative local gates

- `dagger call all --source=.` — passed.
  Trace: <https://dagger.cloud/MChorfa/traces/7217666ab529a234a4f4f018156b3744>
- `dagger call test --source=.` — passed with race detection and coverage gate.
  Trace: <https://dagger.cloud/MChorfa/traces/0cabb7cd231d375abedb788475a377df>

Coverage printed by the final Dagger test:

| Package family | Coverage | Floor |
|---|---:|---:|
| `internal/controller` | 85% | 80% |
| `internal/gateway` | 85% | 80% |
| `internal/storage` | 81% | 80% |
| `internal/auth` | 90% | 80% |
| `internal/inference` | 91% | 80% |
| `internal/observability` | 84% | 80% |

The local Dagger command emitted a warning that the installed Git library does
not support the repository `worktreeconfig` extension. The command continued
and exited zero. One run also reported canceled telemetry upload after success;
that did not change the function result.

### Go and source checks

Passed during the settled state:

```bash
go test -count=1 ./...
go test -race -count=1 ./...
go vet -a ./...
go mod verify
golangci-lint run -v --timeout 10m ./...
git diff --check
git dft -- internal/runtime internal/validation \
  internal/controller/deployment internal/controller/evidence \
  internal/observability dagger
```

The last exact AST proof inspected changed/new non-generated Go files and found:

- no file over 500 LOC;
- no declaration or function literal over 50 LOC;
- no production panic;
- no unchecked Helm map assertion.

Generated DeepCopy files, generated Dagger bindings, and CRD YAML are generated
artifacts and were excluded from handwritten-source LOC enforcement.

### Console and documentation checks

Passed:

```bash
cd console
npm test                 # 43 passed
npm run lint
npx tsc --noEmit

cd ..
go run ./hack/helm-contract
actionlint .github/workflows/ci.yml .github/workflows/release.yml
jq empty .cspell.json
markdownlint-cli2 README.md docs/engine-contract.md \
  docs/remediation-plan.md docs/adr/010-runtime-engine-contract.md \
  docs/adr/011-canonical-observability-planes.md \
  docs/adr/012-model-admission-planes.md docs/open-loops.md \
  docs/ci/current-state.md
```

The `cspell` executable was unavailable. `.cspell.json` parses, but a real spell
run remains useful when the binary is available.

## 6. Tool availability and orchestration state

- No sub-agent remains active.
- The requested `GPT-5.3-Codex-Spark`/Continue model was not callable through
  the available sub-agent API. Lower-cost `gpt-5.6-luna` workers were used.
- `continue` resolves only to the shell builtin, not a Continue delegation CLI.
- `dsh` exists at the current Node installation path but was not needed for the
  final state.
- No `justfile` exists, so no `just` recipe could be invoked.
- `graft` 0.12.0 is installed.
- `Tooling/graft-sync.sh` does not exist, so the requested graft sync script
  could not run.
- `git dft` is configured and was used for structural diff inspection.

Do not resume old agent IDs. Start the next session with no workers and create
new bounded workers only if the user explicitly authorizes delegation.

## 7. Open work, ordered for the next session

### Priority 0 — integration decision

Choose how this mixed checkout will be delivered:

1. Keep working locally without Git delivery.
2. Decompose the current index/worktree into reviewed commits.
3. Build a new isolated worktree and selectively transfer approved paths.
4. Produce a patch bundle for human review.

Do not choose implicitly. The staged area predates and overlaps this session.

### Priority 1 — external acceptance

- Run exact-head hosted CI and attach run URLs (`L-CI-005`).
- Run the fixed Nightly KIND harness (`L-OP-007`, `L-OP-008`).
- Complete the repository-native image/KIND proof (`L-OP-009`, `L-REL-005`).
- Exercise live stable-v1 create/default/convert (`L-API-001`, `L-OP-010`).
- Prove EPP dependency/RBAC behavior in a cluster (`L-OP-011`).
- Keep default CPU proof independent from optional CSI/FUSE (`L-OP-012`).
- Rehearse/publish and verify chart/image/provenance artifacts (`L-REL-004`).
- Run live LMCache hit/latency/ABI/failover acceptance (`L-OP-006`).
- Do not close GPU/multi-node acceptance from unit or Dagger evidence.

### Priority 2 — next local code slice

Recommended bounded slice: `L-CI-008`, the API surface-conformance gate.

Acceptance criteria:

- Every CRD spec field maps to a renderer/observable behavior or an executable
  refusal.
- Refusal entries must resolve to real validation code; a documentation-only
  registry is not acceptable.
- Generated/reflected inventory must fail when a new field is silent.
- Reuse conversion fuzz coverage.
- No broad `explicitly_refused` placeholder list.

An earlier delegated implementation that marked hundreds of fields refused
without executable refusal was rejected and removed. Do not recreate it.

### Priority 3 — architecture tracks

- Finish CKC-ENG image/metrics/receipt/health/workload contracts.
- Remove the redundant LWS reconciler only after behavior parity is proven.
- Replace the unresolved quant-cpp image with a digest-pinned runtime.
- Add SGLang and llama.cpp only through the adapter contract.
- Build the model-admission two-plane controller.
- Complete AIPack cryptographic verification and schema wiring.
- Build CKC-OBS integrity and receipt minimization.
- Consolidate chart sources.
- Reduce dynamic RBAC mutation authority.
- Move `EndpointPickerConfig` off the pre-GA API version.

## 8. First 30 minutes of the next session

Run this sequence before making a change:

```bash
pwd
git branch --show-current
git log -1 --oneline
git status --short --branch
git diff --cached --name-status
git diff --name-status
git ls-files --others --exclude-standard | sed -n '1,160p'

fd '^AGENTS.md$|^CLAUDE.md$|^RULES.md$|^RTK.md$' ..
sed -n '1,180p' docs/session-handoff-2026-08-24.md
sed -n '1,220p' docs/open-loops.md
sed -n '250,500p' docs/open-loops.md
```

Then verify the selected narrow slice, not the entire repository by reflex.
If the first action is delivery/commit work, stop and obtain the integration
choice described in Priority 0.

## 9. Completion-report template for the next session

Every report should include:

- **C:** implemented claims paired with commands or artifacts.
- **S:** implemented subset and the missing gate.
- **A:** unimplemented future direction.
- External hosted/runtime/hardware/provenance acceptance still required.
- Branch/head, dirty-state handling, and any staged/committed/pushed actions.

Do not say “complete” when a required hosted, runtime, hardware, browser,
publication, or provenance gate has not run on the exact delivered head.

## 10. Recommended handoff prompt

```text
Read AGENTS.md and docs/session-handoff-2026-08-24.md completely. Preserve the
dirty checkout and inspect staged, unstaged, deleted, and untracked state
separately. Confirm the current branch/head and rerun only the gates required by
the selected slice. Treat docs/open-loops.md as the status source and
docs/remediation-plan.md as a historical S-class plan. Do not claim hosted,
runtime, GPU, LMCache, browser, publication, or provenance acceptance from local
tests. Before Git delivery, request an explicit integration strategy; never
stage everything by default. Start with no sub-agents unless delegation is
explicitly authorized.
```
