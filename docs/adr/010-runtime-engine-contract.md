# ADR 010: Runtime Engine Contract and Adapter Seam

**Implementation classification: S — proposed architecture with a partial
vLLM tier-1 seam.**

## Status

Proposed

## Context

Engine knowledge is currently spread across four places that do not share a
contract:

- `internal/controller/deployment/builder.go` renders the single-node path and
  emits six arguments: `--model`, `--host`, `--port`, `--quantization`,
  `--enable-lora`, and `--kv-transfer-config`.
- `internal/controller/lws_reconciler.go` is referenced only by its own tests.
  Nothing in `cmd/` or `internal/` constructs it. It is nevertheless the only
  implementation of `--tensor-parallel-size`, `--pipeline-parallel-size`,
  `--data-parallel-size`, `--kv-cache-dtype`, `--cpu-offload-gb`,
  `--enable-eplb`, and speculative decoding.
- `internal/controller/wellknown.go` supplies `--max-model-len`,
  `--gpu-memory-utilization`, and tool-call parsers through substring matches on
  the model URI, hardcoding Gemma 4, DeepSeek, and Qwen3.
- The multimodal, embedding, reranker, and ASR controllers each re-emit engine
  flags by hand.

The consequence is that `spec.parallelism`, `spec.kvCache.dtype`, and
`spec.speculativeDecoding` are accepted by the CRD and never reach a container
argument on the single-node path. The API declares a surface the runtime does
not implement.

Adding SGLang and llama.cpp to this shape multiplies the problem. llm-d v0.9.0
ships SGLang and TensorRT-LLM as supported engines, and the Gateway API
Inference Extension already carries built-in EndpointPicker metric specifications
for SGLang, so a second and third engine are cheap to serve and expensive to
maintain without a seam.

## Decision

- Introduce `internal/runtime` with a single `Adapter` interface. Nothing else
  in the codebase knows an engine's flag names.
- The adapter surface is: `Name`, `Capabilities`, `Image`, `Render`,
  `MetricsContract`, `ReceiptContract`, `HealthContract`, `Validate`.
- `Capabilities()` is a total function over the capability enum. Every
  capability is declared `Supported`, `Unsupported`, or `Emulated`. Adding a
  capability breaks compilation for every adapter until each declares its
  position.
- `Validate()` returns a field error for every spec field the matrix marks
  unsupported. Unsupported fields are refused at admission, never accepted and
  dropped.
- Engines declare a conformance tier (0–3). The tier is enforced: a service
  requesting `router.scheduler` on a tier-1 engine is refused, not degraded.
- Well-known model tuning moves from compiled substring matching to a
  `ModelProfile` resource keyed by model family, engine, and hardware class,
  shipped with an in-repo default set.
- The five inference controllers share one `internal/workload` builder and each
  supplies its own `RenderRequest`.
- The flags stranded in `lws_reconciler.go` are ported into the vLLM adapter and
  the file is deleted. Multi-node continues to delegate to KServe `workerSpec`
  per ADR-009; this ADR does not reintroduce a standalone `LeaderWorkerSet`.

The normative contract is [`docs/engine-contract.md`](../engine-contract.md).

## Consequences

- A new engine is a directory under `internal/runtime/`, a registry entry, a
  `COMPONENTS.md` row, and an entry in the cross-engine end-to-end matrix. No
  controller, CRD, or upstream change.
- Capability drift becomes a compile error rather than a silent runtime gap.
- Users learn at admission time that a field is unsupported on their chosen
  engine, with the field path named, instead of discovering it from serving
  behaviour.
- The refactor touches five controllers. The golden-file fixtures and the
  surface-conformance gate must land first so the change is provably
  behaviour-preserving for vLLM before any second engine is added.
- `ReceiptContract` couples this ADR to ADR-011: uniform observability across
  engines is only reachable if the receipt is sourced at a seam the operator
  controls.

## References

- [Engine contract (CKC-ENG)](../engine-contract.md)
- [ADR-009: KServe 0.19 and llm-d integration boundary](009-kserve-019-and-llm-d-boundary.md)
- [ADR-011: Canonical observability planes](011-canonical-observability-planes.md)
- [llm-d v0.9.0](https://github.com/llm-d/llm-d/releases/tag/v0.9.0)
- [Gateway API Inference Extension model servers](https://gateway-api-inference-extension.sigs.k8s.io/implementations/model-servers/)
