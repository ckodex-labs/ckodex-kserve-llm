# Engine Contract (CKC-ENG)

Normative contract for inference runtimes served by this operator.

**Implementation classification: S.** The contract is design-locked and the
current vLLM adapter implements a tier-1 subset. Compile-enforced totality,
image, metrics, receipt, health, workload extraction, and higher tiers remain
implementation work tracked in [open-loops.md](open-loops.md).

This document defines what a runtime must provide to be a first-class engine,
what it may decline, and what happens when it declines. It is the reference
consulted when adding a new engine; the decision to adopt it is recorded in
[ADR-010](adr/010-runtime-engine-contract.md).

## Contract invariant

```text
the capability matrix has no absent entries; silence is not a declaration
```

`Capabilities()` is a **total function** over the capability enum. Every
capability returns `Supported`, `Unsupported`, or `Emulated` — never absent.
Adding a capability to the enum therefore breaks compilation for every adapter
until each one declares its position.

This is the mechanism that keeps the operator honest as the API grows: a field
the API accepts is a field the runtime emits, or admission refuses it.

## Conformance tiers

Not every runtime can do everything. An engine declares a tier; admission
enforces it.

| Tier | Name | Must provide | Reference engine |
|---|---|---|---|
| 3 | Governed | Everything in tier 2, plus KV-aware routing participation, disaggregated prefill/decode, LoRA hot-swap, complete `InferenceReceipt`, deterministic replay | vLLM |
| 2 | Routed | Everything in tier 1, plus the four EndpointPicker series, so the engine participates in KV-cache-aware scheduling | SGLang |
| 1 | Served | OpenAI contract, health contract, the uniform `infer.*` family, and an `InferenceReceipt` with its gaps declared | llama.cpp |
| 0 | Experimental | Feature-gated; refused by admission under `Enforce` | new work |

The tier is load-bearing, not documentation:

- A service that sets `router.scheduler` on a tier-1 engine is **refused at
  admission**, not silently degraded.
- `ModelAdmissionPolicy` may require a minimum tier per namespace or tenant.
- A tier is earned by passing the shared conformance suite at that tier. Nothing
  else promotes an engine.

## The eight clauses

### C0 — Identity and provenance

- A stable, DNS-label-safe adapter name. It becomes the `spec.engine` value and
  the pod engine label consumed by the EndpointPicker.
- Images resolved to digests per hardware profile. Never tags.
- Variants that do not exist upstream are **absent, not fabricated**. The
  existing ROCm handling — an empty image constant plus an explicit operator
  error — is the precedent to follow.
- An entry in [`COMPONENTS.md`](../COMPONENTS.md) with an upstream release link,
  and inclusion in the CI vulnerability scan matrix.

### C1 — Capability declaration

- `Capabilities()` is total over the enum, as above.
- `Validate()` returns a `field.Error` for every spec field the matrix marks
  `Unsupported`, naming the field path and the reason.
- A generated table test asserts the correspondence between the matrix and
  `Validate()` adapter by adapter. An engine cannot claim a capability its
  renderer does not emit, nor silently drop a field it declares unsupported.

This clause is the contract. The rest is detail.

### C2 — Deterministic rendering

- `Render()` is pure: the same `RenderRequest` produces byte-identical
  arguments, environment, ports, and probes.
- No clock reads, no randomness, no cluster reads inside `Render()`.
- An explicitly user-supplied argument always wins over an adapter default.
- Rendering is idempotent under repeated reconciliation. Arguments must not
  accumulate across reconciles.

Purity is what makes golden-file tests and deterministic replay possible.

### C3 — Serving contract

- OpenAI-compatible `/v1/chat/completions`, `/v1/completions`, and `/v1/models`
  on the declared port — or a declared translation shim that provides them.
- Startup, readiness, and liveness mapped to `HealthContract`. Startup is a
  separate probe because model load legitimately takes minutes and must not trip
  liveness.
- Graceful drain on `SIGTERM` within the pod termination grace period.

### C4 — Metrics contract

- A scrape endpoint and path, plus the pod engine label value.
- Either the four EndpointPicker series — queued requests, running requests,
  KV-cache utilization, LoRA adapters — or an explicit `Unsupported` for
  KV-aware routing, which caps the engine at tier 1.
- Derivation rules for the uniform `infer.*` family defined in CKC-OBS §8:
  native series where they exist, a documented mapping where they do not.

Sourcing operational metrics through the adapter is what makes `infer.ttft` mean
the same thing on every engine.

### C5 — Receipt contract

For each `InferenceReceipt` field, declare the source: native metric, response
header, sidecar observation, or `not-available`.

- `model_digest`, `runtime_digest`, `container_image_digest`, `quantization`,
  and `adapters` are **mandatory**. They come from the adapter's own `Image()`
  and `Render()`, so they cost nothing.
- Token usage, time-to-first-token, and inter-token latency are native or
  sidecar-derived.
- Content minimization is not negotiable. An adapter may not route raw prompt or
  output into a receipt, and the receipt type does not permit it. See CKC-OBS §7.

### C6 — Failure semantics

- Declare engine behaviour on out-of-memory, model-load failure, and context
  overflow, mapped onto the standard outcome vocabulary
  (`allow`, `deny`, `degrade`, `quarantine`).
- An engine that cannot start must surface a terminal status condition on the
  owning resource, not an unexplained crash loop.

### C7 — Air-gap behaviour

- The image must be mirrorable into a local registry.
- Any runtime fetch — weights, kernels, tokenizers, compile caches — is either
  absent or explicitly declared.

An engine that reaches the network on first token is a tier-0 engine in a
sovereign deployment regardless of what else it does.

### C8 — Conformance suite

The engine passes the shared suite at its declared tier:

- golden-file rendering, one fixture per supported capability;
- the capability/validation correspondence table;
- the cross-engine end-to-end assertion that one `LLMInferenceService` produces
  the same OpenAI contract, readiness semantics, status conditions, and `infer.*`
  family regardless of `spec.engine`;
- metric presence at the declared tier;
- receipt completeness against the declared `ReceiptContract`.

## Adding an engine

```text
internal/runtime/<name>/
    adapter.go            # the eight clauses
    capabilities.go       # total over the enum — compiler-enforced
    testdata/*.golden     # one fixture per supported capability
    adapter_test.go       # shared suite, invoked at the declared tier

register  in the adapter registry
add       the image digest to COMPONENTS.md and the vulnerability scan matrix
add       the engine to run/e2e.sh's cross-engine matrix
declare   the tier; admission and the generated docs follow from it
```

No controller changes. No CRD changes. No vendored client library. No upstream
patches.

If adding an engine requires any of those, the adapter seam is in the wrong
place and should be moved rather than worked around.

## What the contract does not require

An engine need not support LoRA, tensor parallelism, KV transfer, or structured
output. It need not be fast. It need not be maintained by this project.

It must only be honest about itself. The capability matrix and admission convert
that honesty into behaviour a user can predict before they deploy.

## References

- [ADR-010: Runtime engine contract](adr/010-runtime-engine-contract.md)
- [ADR-011: Canonical observability planes](adr/011-canonical-observability-planes.md)
- [ADR-009: KServe 0.19 and llm-d integration boundary](adr/009-kserve-019-and-llm-d-boundary.md)
- [Remediation plan](remediation-plan.md)
