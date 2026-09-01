# Local-First Serving Plan

**Repository:** `ckodex-kserve-llm-operator`  
**Plan status:** proposed execution plan  
**Baseline:** 2026-09-01  
**Primary outcome:** prove one small CPU inference path end to end before spending effort on GPU, console, provenance, LMCache, or Agent runtime expansion.

## 1. Executive decision

The project should advance through separate capability tracks. The tracks must
not be presented as one completed product:

1. **Core local path:** a small CPU model is declared through the stable API,
   reconciled by the operator, becomes ready, and answers an inference request.
2. **Hosted acceptance path:** the same CPU profile passes on a fresh hosted KIND
   environment with the documented dependencies and evidence.
3. **Release-trust path:** published artifacts can be verified downstream by
   signature, provenance, SBOM, checksum, and digest.
4. **Console path:** the observe-only console has an explicit authenticated
   boundary and hosted accessibility evidence.
5. **Advanced runtime path:** GPU/NVFP4 and LMCache are separately qualified
   only when suitable hardware and measurement capacity exist.
6. **Agent path:** Agent/SkillRegistry runtime execution is a later product, not
   a prerequisite for the operator beta.

The next milestone is only the first one. A small machine does not need to
execute the full model, NVFP4, GPU, LMCache, or Agent tracks.

## 2. Claim and evidence rules

Use the repository’s C/S/A convention for every milestone:

| Class | Meaning | Allowed wording |
|---|---|---|
| **C** | Implemented, tested, and enforceable in the current evidence boundary | “implemented and verified by …” |
| **S** | Code/design exists but required runtime, hosted, hardware, or identity evidence is missing | “partial,” “locally verified,” or “acceptance pending” |
| **A** | Aspirational, excluded, or outside the current product boundary | “planned,” “deferred,” or “excluded” |

Never promote a claim from S to C because a type, builder, unit test, generated
manifest, or release asset exists. Runtime claims require runtime evidence;
security claims require the corresponding verification result; performance
claims require a reproducible benchmark artifact.

## 3. Baseline assessment

### Confirmed C evidence

- The RC7 release workflow completed hosted release verification, publication,
  provenance generation, chart packaging, scans, and anonymous artifact
  acceptance: [run 33457020052](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33457020052).
- The repository has a small `glm5_next` fixture, pinned model revision,
  preflight script, conformance checks, and a direct CPU generation record:
  [`docs/evidence/glm5-next-tiny-local-2026-08-31.md`](../evidence/glm5-next-tiny-local-2026-08-31.md).
- The operator has API types, conversion code, controller builders, webhook
  routes, Gateway resources, scheduler wiring, and unit/conformance tests.
- Documentation corrections are prepared in [PR #58](https://github.com/ckodex-labs/ckodex-kserve-llm/pull/58).

### Current S evidence

- Fresh hosted KIND inference has not passed on the aligned node image.
- Live v1 admission/conversion and storage-version behavior remain to be
  proven in a real API server.
- The CPU control-plane path is implemented, but the latest evidence does not
  close the complete workload lifecycle and inference journey.
- The console has local/static/browser evidence but no completed authenticated
  operator boundary or full hosted assistive-technology evidence.
- Published signatures and attestations exist, but a downstream verifier run
  still needs to be recorded as evidence.

### Current A or explicitly deferred scope

- Full GLM-5.3 quality, full-size serving, NVFP4 execution, and GPU performance.
- LMCache live hit/transfer/failover behavior.
- Agent and SkillRegistry runtime tool execution.
- Automated traffic promotion and mutation authority.

The linked tiny checkpoint is an architecture and tooling fixture, not a small
quality-equivalent version of the production model. Its model card describes it
as an 84.4M-parameter, BF16, toy-data checkpoint intended for testing and
development, with no real language or vision capability.

## 4. Milestone map

| Milestone | Outcome | Status | Dependency | Exit evidence |
|---|---|---|---|---|
| M0 | Public claims and readiness sources agree | In progress | None | Merged docs PR; no unsupported C claim |
| M1 | Small-machine fixture remains reproducible | Mostly complete | M0 | Direct CPU load/generation record and fixture contract |
| M2 | Stable v1 admission/conversion works in a live local API server | Open | M1 | Create/update/read, defaulting, lossless conversion record |
| M3 | One CPU model runs through the operator end to end | Next | M2 | Fresh local KIND acceptance record with inference and recovery |
| M4 | CPU profile passes hosted KIND parity | Open | M3 | Exact-head hosted Nightly run and retained artifacts |
| M5 | Published release is downstream-verifiable | Partial | M4 or independent | Reproducible signature/provenance/SBOM/checksum verification |
| M6 | Observe-only console has a real access boundary | Open | M3, M5 | Authenticated browser and assistive-technology evidence |
| M7 | Controlled CPU beta decision is evidence-backed | Open | M4, M5, M6 | Green CPU profile, signed decision record, updated matrix |
| M8 | Blackwell/NVFP4 feasibility is known | Deferred | Suitable GPU | Hardware/model/backend compatibility record |
| M9 | GPU/NVFP4 profile is runtime-qualified | Deferred | M8 | Inference, recovery, benchmark, and quality evidence |
| M10 | LMCache is separately runtime-qualified | Deferred | M3 and a measured workload | Cache-hit, latency, ABI, and failover evidence |
| M11 | Agent runtime has a governed implementation proposal | Deferred | M7 and explicit product approval | Threat model, authority model, API, and conformance plan |

## 5. Milestone M0 — truth baseline

**Goal:** make public documentation describe evidence boundaries accurately.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M0.1 | Merge the claim-qualification documentation change and verify the Pages front page points to RC7 while identifying it as a prerelease | `README.md`, `docs/site/index.html`, `docs/beta/*`, `docs/open-loops.md` | **C** — `policy(role=integrator,rw=0.00,blast=localized,GAL=1,DAL=0) → C` |
| M0.2 | Keep the readiness ledger as the authority for current acceptance, not README prose | `docs/beta/readiness-ledger.md`, `docs/beta/acceptance-matrix.yaml` | **R** — `policy(role=architect,rw=0.35,blast=service,GAL=1,DAL=2) → R` |
| M0.3 | Run a claim scan for “supported,” “stable,” “production,” “verified,” “ready,” and “complete”; classify each public occurrence C/S/A | `README.md`, `CHANGELOG.md`, `docs/`, `docs/site/` | **S** — `policy(role=scaffolder,rw=0.10,blast=localized,GAL=1,DAL=0) → S` |

**Exit criteria:**

- PR #58 is merged with all required checks green.
- No public page describes RC7 as a stable or production release.
- Every runtime, auth, GPU, or cryptographic statement points to evidence or
  explicitly says acceptance is pending.
- `docs/beta/readiness-ledger.md` and `docs/beta/acceptance-matrix.yaml` do not
  contradict each other on closed CI/release gates.

**Do not claim yet:** beta approval, production readiness, or complete runtime
support.

## 6. Milestone M1 — small-machine fixture

**Goal:** provide a cheap, repeatable architecture check that does not require
GPU hardware or a full GLM-5.3 checkpoint.

The tiny model is useful for model-type registration, tokenizer/template
handling, weight loading, and bounded generation. It must not become the only
runtime proof for the operator because its card explicitly says it has no real
language capability.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M1.1 | Keep the model URI and revision pinned; retain low CPU/memory limits and short context defaults | `config/samples/llminferenceservice_glm5_next_tiny.yaml` | **B** — `policy(role=builder,rw=0.30,blast=module,GAL=1,DAL=1) → B` |
| M1.2 | Run the weight-free compatibility preflight before downloading or serving weights | `run/glm5-next-tiny-preflight.sh` | **S** — `policy(role=scaffolder,rw=0.10,blast=localized,GAL=1,DAL=0) → S` |
| M1.3 | Retain direct CPU load/generation evidence, including Python, Transformers, torch, CUDA availability, revision, and output limits | `docs/evidence/glm5-next-tiny-local-2026-08-31.md` | **C** — `policy(role=integrator,rw=0.00,blast=localized,GAL=1,DAL=0) → C` |
| M1.4 | Test whether the selected serving adapter can load `glm5_next`; if it cannot, keep GPT-2 for operator E2E and label the tiny fixture Transformers-only | `internal/runtime/`, `test/conformance/`, `docs/runbooks/glm5-next-tiny.md` | **R** — `policy(role=architect,rw=0.70,blast=service,GAL=1,DAL=2) → R` |

**Exit criteria:**

- Direct CPU preflight and generation pass on the small machine.
- The fixture’s model revision and runtime requirements are pinned.
- Operator compatibility is either proven or explicitly recorded as a gap.
- No quality, latency, NVFP4, GPU, or full-model claim is emitted.

## 7. Milestone M2 — live stable v1 contract

**Goal:** turn the v1 API from a tested Go/schema surface into a live API-server
contract.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M2.1 | Verify v1 admission registration, validator/defaulting behavior, webhook service identity, and certificate routes | `internal/webhook/webhook.go`, `internal/webhook/llminferenceservice_v1.go`, `deploy/helm/templates/cert-manager.yaml` | **R** — `policy(role=builder,rw=0.75,blast=service,GAL=2,DAL=3) → R` |
| M2.2 | Exercise v1 create, update, read, and delete against a real API server; record API server and webhook logs | `test/`, `docs/evidence/` | **R** — `policy(role=builder,rw=0.70,blast=cross_service,GAL=2,DAL=3) → R` |
| M2.3 | Prove v1 ↔ v1alpha2 round-trip preservation for status and experimental fields | `api/v1alpha2/conversion*.go`, `api/v1alpha2/conversion_test.go`, `test/conformance/` | **R** — `policy(role=architect,rw=0.70,blast=service,GAL=2,DAL=2) → R` |
| M2.4 | Add one negative test for malformed conversion payloads and one for rejected v1 admission | `internal/webhook/*_test.go`, `test/conformance/` | **R** — `policy(role=builder,rw=0.70,blast=service,GAL=2,DAL=2) → R` |

**Exit criteria:**

- A v1 resource is accepted, defaulted, persisted, read back, and updated in
  a live cluster.
- A v1alpha2 object converts to v1 and back without silent loss of declared
  status or experimental fields.
- Invalid input fails closed with an observable reason.
- Evidence names the cluster, commit, CRD digest, webhook image, and test time.

**Do not claim yet:** v1 is runtime-qualified on all clusters; only the tested
profile is qualified.

## 8. Milestone M3 — one local CPU inference profile

**Goal:** prove the smallest valuable end-to-end journey on a qualifying local
machine.

Use the existing GPT-2 CPU fixture first because it already represents the
operator’s intended inference path. Attempt the tiny GLM through the operator
only after M1.4 proves the adapter/runtime combination.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M3.1 | Preflight Docker/KIND descriptor capacity and verify the pinned node image before cluster creation | `local/01-kind-setup.sh`, `deploy/kind/*`, `run/e2e.sh` | **S** — `policy(role=scaffolder,rw=0.10,blast=localized,GAL=1,DAL=1) → S` |
| M3.2 | Build/load operator and initializer images with bounded context and BuildKit | `.dockerignore`, `Dockerfile`, `Makefile`, `run/e2e.sh` | **B** — `policy(role=builder,rw=0.35,blast=module,GAL=1,DAL=2) → B` |
| M3.3 | Install the documented external CRDs and namespace-scoped EPP identity; record permissions before applying the model | `local/02-prereqs.sh`, `deploy/helm/templates/epp-rbac.yaml`, `deploy/helm/templates/rbac.yaml` | **R** — `policy(role=builder,rw=0.75,blast=cross_service,GAL=2,DAL=3) → R` |
| M3.4 | Apply v1, wait for reconciliation and readiness, and require a strict OpenAI-compatible response through the declared route/hostname | `local/04-llm-inference-service.yaml`, `local/05-test-inference.sh` | **B** — `policy(role=builder,rw=0.45,blast=service,GAL=1,DAL=2) → B` |
| M3.5 | Restart the operator and inference workload; verify status convergence and repeat one request | `run/e2e.sh`, `local/05-test-inference.sh`, `docs/evidence/` | **R** — `policy(role=builder,rw=0.70,blast=service,GAL=1,DAL=2) → R` |

**Exit criteria:**

- Fresh or explicitly clean KIND cluster boots on a host meeting preflight.
- v1 object is reconciled into the expected Deployment, Service, Gateway, and
  route resources.
- Model reaches readiness and the strict probe receives non-empty completion
  data with the declared hostname.
- Operator/workload restart does not require manual object repair.
- Evidence records failures separately as cluster, image, storage, readiness,
  route, or inference failures.

**Do not claim yet:** GPU support, full GLM support, performance, or beta-wide
runtime support.

## 9. Milestone M4 — hosted CPU parity

**Goal:** close the gap between local fixes and hosted runtime acceptance.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M4.1 | Make Nightly use the same KIND version, digest-pinned node image, kubeadm patch shape, and descriptor preflight as local setup | `.github/workflows/nightly-chaos.yml`, `deploy/kind/*`, `local/01-kind-setup.sh` | **R** — `policy(role=architect,rw=0.70,blast=cross_service,GAL=2,DAL=3) → R` |
| M4.2 | Run exact-head hosted acceptance from a fresh checkout; retain cluster bootstrap, CRD, RBAC, readiness, route, inference, restart, and cleanup artifacts | `.github/workflows/nightly-chaos.yml`, `docs/ci/`, `docs/evidence/` | **C** — `policy(role=integrator,rw=0.15,blast=cross_service,GAL=2,DAL=3) → C` |
| M4.3 | Classify any failure before changing code: environment, dependency, image, API, reconciliation, route, model, or probe | `docs/beta/readiness-ledger.md`, `docs/open-loops.md` | **R** — `policy(role=architect,rw=0.70,blast=service,GAL=2,DAL=3) → R` |

**Exit criteria:**

- The exact commit tested is recorded.
- The hosted cluster boots with the pinned environment contract.
- The same CPU profile passes without test-harness lookup races.
- The hosted run proves real inference, not merely resource creation.
- Nightly evidence is linked from the readiness ledger.

## 10. Milestone M5 — downstream release verification

**Goal:** prove what a consumer can verify after RC7 publication.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M5.1 | Add or document a downstream verification script for image signatures, SLSA provenance, SBOM attestations, binary checksums, and chart retrieval | `run/verify-release.sh`, `docs/release-verification.md` | **R** — `policy(role=builder,rw=0.75,blast=cross_service,GAL=2,DAL=3) → R` |
| M5.2 | Verify certificate identity and OIDC issuer against the intended repository/workflow, not only “signature exists” | `run/verify-release.sh`, `docs/SECURITY_ARCHITECTURE.md` | **R** — `policy(role=builder,rw=0.85,blast=cross_service,GAL=2,DAL=3) → R` |
| M5.3 | Compare image/chart/binary digests and source commit to the release metadata; retain output as evidence | `docs/evidence/`, `docs/release-verification.md` | **C** — `policy(role=integrator,rw=0.15,blast=service,GAL=2,DAL=3) → C` |

**Exit criteria:**

- An independent consumer can retrieve the published artifacts anonymously.
- Signature, provenance, SBOM, and checksum commands return successful,
  identity-scoped verification results.
- The evidence distinguishes publication/acceptance from cryptographic
  verification performed downstream.

**Do not claim yet:** that metadata, a violet UI state, or an artifact URL alone
proves cryptographic validity.

## 11. Milestone M6 — authenticated observe-only console

**Goal:** make the console’s access and authority boundary real and testable.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M6.1 | Choose the boundary: authenticated ingress/OIDC, API-server proxy, or an explicitly documented local-only mode | `docs/adr/`, `console/README.md`, `docs/beta/plan.md` | **R** — `policy(role=architect,rw=0.80,blast=cross_service,GAL=2,DAL=3) → R` |
| M6.2 | Define identity projection, namespace visibility, read-only verbs, denial behavior, session expiry, and audit fields | `console/src/lib/`, `deploy/helm/templates/`, `docs/SECURITY_ARCHITECTURE.md` | **R** — `policy(role=architect,rw=0.85,blast=cross_service,GAL=2,DAL=3) → R` |
| M6.3 | Implement the selected adapter without adding mutation or cluster-tool authority | `console/src/lib/`, `console/src/app/`, `internal/` only where required | **R** — `policy(role=builder,rw=0.75,blast=cross_service,GAL=2,DAL=3) → R` |
| M6.4 | Run hosted browser journeys plus keyboard, reduced-motion, forced-colors, and assistive-technology checks | `console/`, `.github/workflows/`, `docs/evidence/` | **C** — `policy(role=integrator,rw=0.20,blast=cross_service,GAL=2,DAL=3) → C` |

**Exit criteria:**

- Unauthenticated access is denied or explicitly limited to a documented local
  mode.
- A user can see only authorized namespaces/resources.
- No console action mutates routes, workloads, RBAC, or cluster configuration.
- Browser and assistive-technology evidence names the packaged image and exact
  tested commit.

## 12. Milestone M7 — controlled CPU beta decision

**Goal:** make a narrow, honest beta decision for one CPU profile.

The beta decision is not “all CRDs work.” It is “one supported CPU profile has
repeatable evidence, a qualified access boundary, and explicit exclusions.”

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M7.1 | Freeze the supported CPU profile: model, URI, storage path, dependencies, resources, route, namespace, and recovery behavior | `docs/beta/acceptance-matrix.yaml`, `docs/beta/readiness-ledger.md`, `docs/getting-started.md` | **R** — `policy(role=architect,rw=0.80,blast=service,GAL=2,DAL=3) → R` |
| M7.2 | Require M2–M6 evidence references before changing the profile from S to C | `docs/beta/acceptance-matrix.yaml`, `docs/open-loops.md` | **R** — `policy(role=architect,rw=0.75,blast=cross_service,GAL=2,DAL=3) → R` |
| M7.3 | Create a signed decision record naming the owner, date, exclusions, rollback, and next review date | `docs/adr/`, `docs/evidence/` | **R** — `policy(role=architect,rw=0.80,blast=cross_service,GAL=2,DAL=3) → R` |

**Exit criteria:**

- M3 and M4 pass for the same declared CPU profile.
- M5 has downstream verification evidence.
- M6 has the selected access boundary and hosted accessibility evidence, or the
  console is explicitly excluded from the beta.
- The release is still labelled prerelease unless the project owner separately
  approves a beta promotion.

## 13. Milestone M8 — Blackwell/NVFP4 feasibility gate

**Goal:** decide whether a real GPU/NVFP4 path is worth pursuing before editing
the operator for it.

This milestone is hardware-gated. It is not a small-machine task.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M8.1 | Record the actual GPU model, compute capability, VRAM, driver, CUDA, framework, and container runtime | `docs/evidence/`, `docs/runbooks/` | **S** — `policy(role=scaffolder,rw=0.10,blast=localized,GAL=1,DAL=1) → S` |
| M8.2 | Select one exact model checkpoint and verify its publisher, revision, quantization format, tokenizer, license, and artifact digest | `docs/model-capacity.md`, `docs/runbooks/`, `config/samples/` | **R** — `policy(role=architect,rw=0.80,blast=service,GAL=2,DAL=2) → R` |
| M8.3 | Validate the backend/runtime combination on the real hardware; do not assume that a similarly named GLM checkpoint or the tiny BF16 fixture is NVFP4 | `docs/evidence/`, `docs/runbooks/` | **R** — `policy(role=architect,rw=0.85,blast=service,GAL=2,DAL=2) → R` |
| M8.4 | Confirm the selected backend rather than forcing an unqualified backend; record startup logs and compatibility failures | `docs/evidence/` | **B** — `policy(role=builder,rw=0.40,blast=module,GAL=1,DAL=2) → B` |

**Exit criteria:**

- The exact GPU and model checkpoint are identified and reproducible.
- The model starts with the selected runtime and returns a basic completion.
- VRAM, context length, startup time, and failure behavior are recorded.
- A go/no-go decision is recorded before any production-facing NVFP4 claim.

The NVFP4 guide’s core constraint applies: NVFP4 requires compatible Blackwell
hardware and model/backend support; older GPUs need a different format. The
current tiny checkpoint is BF16 and is not evidence for NVFP4.

## 14. Milestone M9 — GPU/NVFP4 runtime qualification

**Goal:** qualify one real GPU profile, not “GPU support” in the abstract.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M9.1 | Add a separate GPU profile with explicit resources, node selectors, tolerations, runtime class, model URI, and quantization fields | `config/samples/`, `deploy/helm/values.yaml`, `docs/runbooks/` | **B** — `policy(role=builder,rw=0.45,blast=service,GAL=1,DAL=2) → B` |
| M9.2 | Prove readiness, chat completion, restart, OOM handling, and recovery on the same hardware profile | `run/`, `docs/evidence/` | **R** — `policy(role=builder,rw=0.75,blast=cross_service,GAL=2,DAL=3) → R` |
| M9.3 | Benchmark fixed prompt/concurrency/context scenarios and retain p50/p95 latency, TTFT, output tokens/sec, memory, and error results | `docs/evidence/`, `docs/runbooks/` | **R** — `policy(role=builder,rw=0.75,blast=service,GAL=1,DAL=2) → R` |
| M9.4 | Run a small quality regression set against a declared baseline; do not use vendor numbers as repository evidence | `docs/evidence/`, `docs/model-capacity.md` | **R** — `policy(role=architect,rw=0.80,blast=service,GAL=1,DAL=2) → R` |

**Exit criteria:**

- The exact model, quantization, runtime, image digest, GPU, and driver are
  recorded.
- Inference and recovery pass on that profile.
- Performance claims include methodology and environment fingerprint.
- Quality results are separate from systems/runtime success.
- The profile is labelled C only for the tested hardware/model combination.

## 15. Milestone M10 — LMCache as an independent track

**Goal:** prove cache behavior without confusing KV-cache transfer with model
weight placement.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M10.1 | Keep `LocalModelCache` ownership and LMCache KV-cache ownership explicit in the design record | `docs/adr/009-kserve-019-and-llm-d-boundary.md`, `internal/scheduler/epp_manager.go`, `docs/runbooks/lmcache.md` | **R** — `policy(role=architect,rw=0.80,blast=service,GAL=2,DAL=3) → R` |
| M10.2 | Run miss → fill → hit scenarios with fixed prompts and record cache state, transfer time, and tail latency | `docs/evidence/`, `run/` | **R** — `policy(role=builder,rw=0.75,blast=cross_service,GAL=2,DAL=3) → R` |
| M10.3 | Restart one participant and verify fail-closed behavior, recovery, and no stale/corrupt result | `docs/evidence/`, `run/` | **R** — `policy(role=builder,rw=0.85,blast=cross_service,GAL=2,DAL=3) → R` |

**Exit criteria:**

- Cache-hit behavior is observable and reproducible.
- ABI/image/runtime compatibility is recorded.
- Performance improvement is reported only against a fixed no-cache baseline.
- Failure and failover behavior pass, or LMCache remains explicitly excluded.

## 16. Milestone M11 — governed Agent runtime proposal

**Goal:** decide whether Agent/SkillRegistry runtime execution belongs in this
operator and define its authority boundary before implementation.

The current resources are metadata/control-plane surfaces. Do not advertise them
as tool execution.

**Work items:**

| ID | Action | Files | Tier / routing |
|---|---|---|---|
| M11.1 | Write a threat model covering prompt injection, tool abuse, secrets, tenant escape, replay, and runtime isolation | `docs/adr/`, `docs/SECURITY_ARCHITECTURE.md`, `docs/threat-model/` | **R** — `policy(role=architect,rw=0.90,blast=cross_service,GAL=2,DAL=3) → R` |
| M11.2 | Define execution authority, leases, identity, approval, audit, cancellation, resource limits, and rollback | `docs/adr/`, `api/`, `internal/validation/` | **R** — `policy(role=architect,rw=0.90,blast=cross_service,GAL=2,DAL=3) → R` |
| M11.3 | Build a minimal non-production conformance harness before a runtime controller | `test/conformance/`, `docs/evidence/` | **B** — `policy(role=builder,rw=0.45,blast=service,GAL=2,DAL=3) → R` |

**Exit criteria:**

- Explicit product approval exists.
- The runtime has a separate authority and isolation design.
- No tool execution is enabled by merely creating an Agent or SkillRegistry
  object.
- The beta contract continues to exclude Agent runtime until its own evidence
  exists.

## 17. Dependency graph

```text
M0 truth baseline
  ↓
M1 small-machine fixture
  ↓
M2 live v1 admission/conversion
  ↓
M3 local CPU operator inference
  ↓
M4 hosted CPU parity ─────┐
  ↓                       │
M5 downstream release     │
  ↓                       │
M6 authenticated console ─┘
  ↓
M7 controlled CPU beta decision

M3 ──→ M8 GPU/NVFP4 feasibility ──→ M9 GPU/NVFP4 qualification
M3 ──→ M10 LMCache qualification
M7 ──→ M11 governed Agent runtime proposal
```

Parallelism is intentional:

- M5 can proceed after RC7 publication and does not require a GPU.
- M6 can design the access boundary while M4 is being qualified, but cannot
  claim hosted console acceptance early.
- M8–M10 are independent advanced tracks and must not block M3.
- M11 must not begin as implementation work until its authority model is
  approved.

## 18. Small-machine operating budget

For routine local work:

1. Run `run/glm5-next-tiny-preflight.sh` and direct CPU generation for model
   architecture checks.
2. Run focused Go/conformance tests for code changes.
3. Run the full Dagger gate before merge, not after every small edit.
4. Run KIND only for M2/M3 acceptance or when changing cluster, webhook,
   storage, route, scheduler, or image-build behavior.
5. Do not download the full GLM-5.3 checkpoint on the small machine.
6. Do not attempt NVFP4 locally without compatible Blackwell hardware.
7. Let hosted CI perform repeatable release, scan, and architecture-specific
   checks.

## 19. Definition of done for the next release decision

The project may call itself **CPU-profile qualified** only when M2, M3, and M4
are complete for the same declared profile and the evidence is linked from the
ledger.

The project may call itself **controlled beta** only when M5 is complete and
M6 is either complete or the console is explicitly excluded from the beta
contract, with a signed decision record from M7.

The project may call itself **GPU/NVFP4 qualified** only for the exact model,
checkpoint, hardware, runtime, and benchmark envelope that passed M8 and M9.

The project must continue to describe LMCache and Agent runtime as separate
tracks until M10 and M11 produce their own evidence.

## 20. External technical references

- [Tiny GLM-5.3-Flash model card](https://huggingface.co/inference-optimization/GLM-5.3-Flash-0.1B-A0.1B) — architecture-preserving, toy-data fixture boundary.
- [vLLM supported models](https://docs.vllm.ai/en/stable/models/supported_models/) — runtime support must be established by the actual serving result, not by model-name similarity.
- [vLLM GLM-5.3-Flash recipe index](https://docs.vllm.ai/projects/recipes/en/latest/index.html) — current upstream recipe context for full-model serving.
