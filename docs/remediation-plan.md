# Remediation Plan: Declared, Then Implemented

Nine phases closing the gap between the operator's declared API surface and its
live code paths.

**Classification: S — historical implementation plan.** This plan was authored
from commit `4b596ef`; several Phase 0, Phase 1, Phase 2, Phase 4.9/4.10,
Phase 5.2, Phase 6.0/6.1, and Phase 8 items have since progressed. Treat
[open-loops.md](open-loops.md) and current test evidence as the status source,
not the original findings below.

Findings come from a static read of the working tree at commit `4b596ef` plus
upstream release verification. Nothing here was confirmed by compiling or
running the test suite; the Go toolchain was unreachable from the authoring
environment. Claims established by inference rather than direct observation are
marked as such.

## Governing laws

Every item serves one of three sentences.

```text
Law 1 — surface
a field the API accepts is a field the runtime emits, or admission refuses it

Law 2 — governance
an unimplemented check refuses; it does not pass

Law 3 — evidence (CKC-OBS §13, verbatim)
Observability failure SHALL NOT silently convert a governed execution into an
ungoverned execution.
```

Phase 0 exists because none of them is enforceable until the test suite gates a
merge.

## Phase 0 — Make the truth checkable

CI runs `dagger call all`, which is lint plus a compile check. The race-enabled
suite with per-package coverage gates exists in `dagger/main.go` and is never
invoked, so roughly 28,500 lines of unit tests gate nothing.

| ID | Item |
|---|---|
| 0.1 | Split the exact-patch Go directive into `go 1.26` plus an explicit `toolchain` line so pinned environments build without fetching a toolchain |
| 0.2 | Add `dagger call test` to `ci.yml` |
| 0.3 | Replace the fixed controller coverage constant (27%) with a ratchet that refuses a decrease; target 60% by end of Phase 3 |
| 0.4 | Set `run.tests: true` in golangci; extend the gosec allowlist with G101, G107, G204, G301, G304, G306, G404 |
| 0.5 | Build the surface-conformance gate: walk every CRD spec field, assert each resolves to a rendered argument, an observable behaviour, or a registered refusal |
| 0.6 | Add `v1alpha2 -> v1 -> v1alpha2` round-trip fuzzing for every type with a spoke |

**Exit criteria.** A pull request that drops a spec field fails CI. One that
lowers coverage fails CI. A clean clone builds without reaching the Go toolchain
server.

Estimated ~1 engineer-week. Unblocks every later phase.

## Phase 1 — API truth: one hub, no phantom fields

`ConvertTo`, `ConvertFrom`, and `Hub()` are fully written. No CRD manifest
carries a `spec.conversion` stanza, in `config/crd/`, `deploy/helm/`, or
`charts/`, so none of it runs. `v1` is the storage version while every
controller watches `v1alpha2`, and the README directs new resources to `v1`.

Eight spec fields diverge between the versions: `engine`, `kvCache`,
`observability`, `prefill`, `quantization`, `speculativeDecoding`, `toolSurface`,
`worker`.

**Decision required.** Option A completes `v1` by extending `ExperimentalSpec`
to carry `engine`, `quantization`, and `speculativeDecoding`, and promoting
`toolSurface` and `observability` to the stable surface — additive, no break for
existing objects. Option B sets `v1` to `served: false` until the surface is
real. **Recommendation: A.** Option B publicly retracts a version already
recommended in the README, to fix a problem that is additive to solve.

| ID | Item |
|---|---|
| 1.1 | Complete the conversion functions. `ConvertTo` currently maps three of eight fields; the other five are dropped by the function itself, so wiring the webhook without this converts a silent gap into active data loss |
| 1.2 | Emit `spec.conversion.strategy: Webhook` with `conversionReviewVersions` and the cert-manager CA-injection annotation from `make manifests`; ship it in both charts |
| 1.3 | Register per-version webhook paths. The Helm rules match `["v1alpha2","v1"]` but both point at the `v1alpha2` handler |
| 1.4 | Move controllers onto the hub. A controller watching a spoke sees whatever conversion leaves behind |
| 1.5 | Repeat for the other six spokes: Agent, SkillRegistry, ModelOnboarding, InferenceSession, InferenceActor, CoactorGroup |
| 1.6 | Record the dual-serve window and storage-version migration in `docs/api-deprecation-policy.md` |

Estimated ~2–3 engineer-weeks. Runs in parallel with Phase 2.

## Phase 2 — The engine seam

See [ADR-010](adr/010-runtime-engine-contract.md) and the normative
[engine contract](engine-contract.md).

| ID | Item |
|---|---|
| 2.1 | Introduce `internal/runtime` with the `Adapter` interface |
| 2.2 | Capability matrix as data, total over the enum per clause C1 |
| 2.3 | `MetricsContract` and `ReceiptContract` as separate declarations |
| 2.4 | Replace the model-URI substring switch with a `ModelProfile` resource |
| 2.5 | Extract `internal/workload`; unify the five inference controllers |
| 2.6 | Port the flags stranded in `lws_reconciler.go` into the vLLM adapter, then delete the file |
| 2.7 | Golden-file rendering fixtures, one per adapter per capability |

Estimated ~3–4 engineer-weeks. Blocks Phases 3, 4, and 7.

## Phase 3 — Four engines behind one contract

| Engine | Today | Work | Declared unsupported |
|---|---|---|---|
| vLLM | Default; six flags reach the single-node path | Port the stranded flags; move speculative decoding onto the single `--speculative-config` JSON that replaced the `--spec-*` flags | — |
| SGLang | Absent | Adapter, digest-pinned image, `--tp-size`, `--dp-size`, `--context-length`, `--mem-fraction-static`, and `--enable-metrics`, which is off by default and without which the EndpointPicker is blind | LoRA metrics |
| llama.cpp | Seam exists; image does not | Repoint from `ckodex/quant-cpp:v0.1.0` — not built in this repository, and its registry tag API returns 404 — to a digest-pinned upstream `llama.cpp` server image; map `-m`, `-c`, `-ngl`, `-np`, `--alias`, `--metrics` | TP, PP, KV transfer, LoRA hot-swap, KV-aware routing |
| TensorRT-LLM | Absent | Optional, behind a feature gate; llm-d v0.9.0 ships a supported image | scoped at design time |

SGLang is the cheap one: the Gateway API Inference Extension already ships
built-in EndpointPicker metric specifications for SGLang, selected by a pod
engine label, with vLLM as the default for unlabelled pods.

llama.cpp is an edge runtime and the contract should say so. `llama-server` is
OpenAI-compatible and exposes Prometheus metrics with `--metrics`, but has no
KV-cache-utilization semantics for the EndpointPicker to schedule on.

The deliverable is the cross-engine acceptance test: one `LLMInferenceService`,
four values of `spec.engine`, and an assertion that the OpenAI contract, the
route, readiness semantics, status conditions, and the emitted `infer.*` family
are identical.

Estimated ~2 engineer-weeks for vLLM and SGLang, ~1 for llama.cpp.

## Phase 4 — CKC-OBS: three planes

See [ADR-011](adr/011-canonical-observability-planes.md).

| ID | Item |
|---|---|
| 4.1 | Split into `internal/telemetry`, `internal/signal`, `internal/evidence`; enforce the canonicality declaration at registration |
| 4.2 | Canonical envelope as a Go type plus a JSON Schema |
| 4.3 | Produce `InferenceReceipt` from an evidence sidecar on the OpenAI path, with per-engine derivations from `ReceiptContract` |
| 4.4 | Enforce content minimization in the type system; `ContentMessage` and `ContentPart` are content-bearing by construction today |
| 4.5 | Add integrity: SPIRE-derived producer identity, signature, sequence chain, Merkle commitment, transparency proof reference |
| 4.6 | Migrate to `exp.*` / `infer.*` / `ops.*`; four Prometheus prefixes and two event prefixes are currently in use |
| 4.7 | Source the required operational metric families through the adapter |
| 4.8 | `EvidenceCheckpoint` at each state transition, with the five dispositions |
| 4.9 | Observe the evidence system itself — missing receipts, signature failures, sequence gaps, broken causal edges |
| 4.10 | Declare graded failure semantics per profile; the Kubernetes event write currently discards its error |
| 4.11 | `ReplayBundle`, deterministic and counterfactual; the missing inputs are the context manifest and generation-config digests |
| 4.12 | Twelve conformance vectors, one per numbered conformance requirement |

Estimated ~5–6 engineer-weeks. Items 4.4, 4.5, and 4.10 are load-bearing.

## Phase 5 — Model admission

See [ADR-012](adr/012-model-admission-planes.md).

| ID | Item |
|---|---|
| 5.1 | `ModelAdmissionPolicy` and `ModelAdmissionHook`; ordered chain with per-hook failure policy and enforcement mode |
| 5.2 | Graduate `ModelOnboarding` into the progressive-admission pipeline; an unrecognised stage type must fail, not skip |
| 5.3 | Extend `GateCriteria` to eval score, VAD coverage, risk band, and cost budget |
| 5.4 | `spec.admission.required` withholds the Deployment and route until `Admitted` |
| 5.5 | Decision counters by hook and verdict, latency histograms, deny events, `Admitted` condition |

Estimated ~4–5 engineer-weeks.

## Phase 6 — AIPack, in full

Thirteen artifact schemas, nine extension predicate schemas, and three predicate
schemas already exist under `schema/`, with typed Go APIs across §§3–22. What is
missing is the implementations, and the direction they fail in.

**6.0 (P0) — verification must verify.** `VerifyAIPackAttestation` returns
`Verified: true` when required predicate URIs are present and non-empty. There
is no cosign call, no Rekor lookup, no binding to the artifact digest, and no
validation of the predicate payload. That result sets the
`Compliance-SR-2-AIPack` condition, and attestation is documented as required
for promotion to staging and above. Replace with cosign signature verification
bound to the artifact digest, Rekor inclusion proof, payload validation against
the schemas already in `schema/predicates/`, and a TTL cache.

**6.1 (P0) — the fail-closed sweep.** `ValidatePattern` returns nil.
`ValidateVADDeclaration` returns nil. `ManifoldDistance` returns 0.
`InferPattern` always returns the baseline pattern. `EvaluatePolicyBundle` skips
families, required predicates, and maximum risk band. Every unimplemented
validator must return `ErrNotImplemented`, and admission must treat that as deny
under `Enforce` and a warning under `Audit`. This is a one-day change that
converts an unknown quantity of silent false negatives into a visible backlog.
Do it before implementing any section below.

| Section | Subject | State | Work |
|---|---|---|---|
| §6, §7 | Attestation | Presence check only | Item 6.0 |
| §11 | Lineage | Stub; hand-rolled digest | Real RFC 8785 JCS; assembly hash; also repairs `ComputeCompositeDigest` |
| §12 | Blast radius | Stub | Needs a persistent artifact graph — promote the in-memory `Registry` to an index backed by an AIPack informer plus the OCI referrers API |
| §13 | Risk valence | Formula present; `signals.go` empty | Wire the thirteen signal sources; several arrive free from the Phase 4 drift and safety families |
| §14 | Outlier | Stub | Statistical thresholding over eval history; dismissal predicate |
| §15 | TEA | Stub | Real HTTP client with retries; offline mode for air-gapped sites |
| §16 | Deprecation | Stub | ISO 8601 parsing, sunset expiry, notice and revocation predicates |
| §17 | Air gap | Stub | Embedded trust roots, RFC 3161 timestamp token, validity window |
| §18 | Patterns | Returns constants | P1–P7 classification from slot presence; slot-vector Hamming distance |
| §19 | Policy | Partial | Full evaluation plus Rego bundle execution, reusing the existing OPA reconciler |
| §21 | Quarantine | Stub | Use CEL rather than a bespoke DSL: sandboxed, bounded, already in the dependency tree |
| §22 | VAD | Stub | Perturbation families; per-kind class coverage |

| ID | Item |
|---|---|
| 6.2 | Wire the 25 JSON Schemas into the Phase 5 admission chain; assert every schema has a conformance vector |
| 6.3 | One conformance vector per normative rule ID |
| 6.4 | Emit attestations, SBOM and AI-BOM references, policy digests, and transparency proofs onto the Phase 4 evidence plane rather than an AIPack-private store |

Estimated ~1.5 engineer-weeks for 6.0 and 6.1; ~6–8 for the remaining sections,
parallelizable by section.

## Phase 7 — Upstream currency

| Surface | Pinned | Current | What arrives |
|---|---|---|---|
| KServe | v0.19.0 | v0.20.0 | Managed DRA for LLMInferenceService, traffic splitting, model-based routing gates, secondary filesystem tiers for KV offload, vLLM as a supported runtime |
| llm-d | v0.8.1 | v0.9.0 | SGLang as a shipped engine, TensorRT-LLM, multi-tier KV offload to external storage, heterogeneous memory allocation, predicted-latency scheduling |
| vLLM | v0.25.1 | v0.27.1 | Model Runner V2, FlashAttention 4, PyTorch 2.13, KV offload with the hybrid memory allocator |
| GIE EPP | v1.5.0 | re-pin | Move `EndpointPickerConfig` off `inference.networking.x-k8s.io/v1alpha1`; the pool it feeds is already GA `v1` |

Two of these change architecture rather than a pin: llm-d multi-tier KV
offloading overlaps with the LMCache work tracked as L-OP-006, and KServe managed
DRA is the correct replacement for the hand-rolled GPU device selection.

Rule 4 of [`dependency-alignment.md`](dependency-alignment.md) stands: no upgrade
is inferred from a newer tag. Each bump needs a compatibility test, a release
artifact, and live serving evidence for the affected hardware profile.

Estimated ~2 engineer-weeks plus live-cluster acceptance time.

## Phase 8 — Operational hygiene

| ID | Item |
|---|---|
| 8.1 | Give the EndpointPicker a ServiceAccount, Role, and RoleBinding, and set `serviceAccountName`. It needs to read `InferencePool` and list Pods; it fails closed today, presenting as a scheduler that never becomes ready |
| 8.2 | Consolidate the two Helm charts. `deploy/helm/` carries CRDs, webhook configurations, and cert-manager wiring; `charts/ckodex-kserve-llm-operator/` carries none, and the `appVersion` strings differ by a leading `v` |
| 8.3 | Derive `VLLM_TARGET_DEVICE` from the adapter rather than defaulting to `cpu` when hardware detection yields `Unknown`; drop `GPU_MEMORY_UTILIZATION`, which vLLM does not read |
| 8.4 | Replace the flag/value pair-walking slice in `ApplyHardwareOptimizations` with a typed struct; a single boolean flag would panic |
| 8.5 | Restore `open-loops.md` as a tracker rather than a changelog |
| 8.6 | Generate the engine matrix documentation from the capability matrix so it cannot drift |

Estimated ~1.5 engineer-weeks.

## Traceability

| Finding | Closed by |
|---|---|
| Conversion functions written, never wired; no `spec.conversion` in any CRD | 1.2 |
| Controllers watch the spoke while `v1` is storage | 1.4 |
| Five of eight divergent fields dropped by `ConvertTo` itself | 1.1 |
| Webhook rules match `v1` but route to the `v1alpha2` handler | 1.3 |
| `lws_reconciler.go` dead; sole home of the advanced vLLM flags | 2.6 |
| Single-node path emits no TP/PP, KV dtype, or speculative decoding | 2.1, 2.6 |
| Model tuning hardcoded as substring matches on model URI | 2.4 |
| Five controllers each re-emit engine flags by hand | 2.5 |
| Audit path has no signature, hash chain, or sequence number | 4.5 |
| Receipt types carry raw content and base64 payloads by construction | 4.4 |
| Audit write errors discarded — governed becomes ungoverned in silence | 4.10 |
| Four Prometheus prefixes; two event prefixes for one concept | 4.6 |
| Promotion gate reads an operator-emitted metric that will not survive a second engine | 4.7, 5.3 |
| No mechanism detects an expected receipt that never arrives | 4.9 |
| Unknown onboarding stage type logs and passes | 5.2 |
| AIPack attestation verified by string presence | 6.0 |
| AIPack validators fail open | 6.1 |
| AIPACK-SPEC §§11–22 unimplemented | Phase 6 table |
| Twenty-five JSON Schemas enforced nowhere | 6.2 |
| EndpointPicker runs without a ServiceAccount or RBAC | 8.1 |
| `EndpointPickerConfig` still on `x-k8s.io/v1alpha1` | Phase 7 |
| Unit tests never gate a merge; controller floor at 27% | 0.2, 0.3 |
| Test files unlinted; gosec omits the injection families | 0.4 |
| `ckodex/quant-cpp:v0.1.0` unbuilt and unresolvable | 3 (llama.cpp row) |
| Two divergent Helm charts; mismatched `appVersion` | 8.2 |
| `VLLM_TARGET_DEVICE=cpu` fallback; inert GPU env var | 8.3 |
| Exact-patch Go directive forces a toolchain download | 0.1 |
| KServe, llm-d, and vLLM each one or two minors behind | Phase 7 |

## Sequencing

```text
00 foundation --+-> 01 API truth ------------------------+
                |                                        |
                +-> 02 seam --+-> 03 engines -> 07 upstream
                |             |                          +-> 05 admission
                |             +-> 04 CKC-OBS ------------+
                |                                        |
                +-> 06.0 / 06.1 fail-closed -------------+

     08 hygiene -- independent, run continuously
     06.2 ...   -- AIPack sections, parallel by section after 06.1
```

Three tracks after Phase 0: an API track, a runtime-and-evidence track, and a
governance track. Phase 5 is the join — it needs the capability matrix for its
first hook, the evidence plane for its receipts, and real verification for its
provenance hook.

Roughly 21–26 engineer-weeks in total; about 11–14 calendar weeks with three
engineers, one per track.

**Two items belong in week one regardless of track assignment.** Item 6.1 is a
one-day change converting silent governance false negatives into a visible list.
Item 4.10 is a similarly small change that stops a discarded error from turning a
governed execution into an ungoverned one.

## Risks

| ID | Risk | Mitigation |
|---|---|---|
| R1 | Enabling enforcement breaks live workloads. Phases 5, 6.1, and 4.10 turn silent passes into denials | Every gate ships with audit mode first; enforcement flips per namespace once the audit record is quiet |
| R2 | The conversion fix touches stored objects | Rehearse storage-version migration against a cluster with real data, not only envtest; budget a runbook and a rollback path |
| R3 | The adapter refactor touches five controllers | Phase 0 golden files and the surface-conformance gate land first, so the refactor is provably behaviour-preserving for vLLM |
| R4 | The evidence sidecar adds per-request latency | Measure it in the cross-engine matrix; make the sampling policy explicit — the canonicality rule exists so not every invocation needs a retained receipt |
| R5 | Effort figures are estimates | The AIPack range is widest: §12 depends on an artifact graph that does not exist, and §13's cost is sourcing thirteen signals, not the arithmetic |

## References

- [Engine contract (CKC-ENG)](engine-contract.md)
- [ADR-010: Runtime engine contract](adr/010-runtime-engine-contract.md)
- [ADR-011: Canonical observability planes](adr/011-canonical-observability-planes.md)
- [ADR-012: Model admission planes](adr/012-model-admission-planes.md)
- [ADR-009: KServe 0.19 and llm-d integration boundary](adr/009-kserve-019-and-llm-d-boundary.md)
- [Open loops](open-loops.md)
- [KServe releases](https://github.com/kserve/kserve/releases)
- [llm-d releases](https://github.com/llm-d/llm-d/releases)
- [vLLM releases](https://github.com/vllm-project/vllm/releases)
