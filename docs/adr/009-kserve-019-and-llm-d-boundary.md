# ADR 009: KServe 0.19 and llm-d Integration Boundary

## Status

Accepted

## Context

KServe 0.19 provides its own `LLMInferenceService`, model-storage integration,
Gateway API reconciliation, `LocalModelCache`, and llm-d integration. llm-d
0.8.1 describes a larger system containing routing/EPP, batch ingestion,
workload APIs, disaggregated prefill/decode, shared caches, and variant
autoscaling.

This repository predates that upstream convergence and currently owns a CKodex
API plus selected integrations. In particular, the presence of the llm-d EPP
image does not mean the complete llm-d 0.8.1 architecture is installed.
vLLM's in-process prefix cache is also not LMCache. The operator now exposes a
version-neutral KV-transfer contract for NIXL, LMCache, and Mooncake and wires
it into vLLM's `--kv-transfer-config` for disaggregated prefill/decode. It does
not install or own the selected backend service; backend health, cache hits,
tail latency, and failover remain live-cluster acceptance gates.

## Decision

- Treat KServe 0.19 as the serving substrate and avoid adding competing copies
  of upstream storage, routing, or workload primitives.
- Keep the CKodex controller focused on governance, policy, evidence, and the
  compatibility surface that is not yet migrated.
- Describe EPP routing as a component integration, not as full llm-d 0.8.1
  conformance.
- Preserve the current API while a separate, tested migration maps CKodex
  fields to upstream KServe resources. This repair does not silently replace
  existing CRDs.
- Explicit multi-node workloads map to KServe v0.19 `InferenceService`
  `workerSpec`; CKodex does not create a second standalone `LeaderWorkerSet`.
  The installed KServe multi-node runtime owns its image, Ray lifecycle, and
  health probes.
- Runtime images and model paths remain explicit: the configured vLLM image is
  honored, and the mounted artifact is passed as `--model /mnt/models`.
- KV-transfer configuration is explicit and backend-neutral: connector-specific
  settings stay in `extraConfig`; the operator does not claim llm-d/LMCache
  conformance until live evidence covers the enabled data path.

## Consequences

The immediate serving fixes are compatible with the current API, while future
work has a clear direction. Full llm-d claims require live evidence for every
enabled component, not merely a router image or an architecture diagram.

Multi-node status and routing are projected from the upstream KServe object.
Unsupported CKodex topology fields fail before resource creation instead of
producing incomplete worker pods.

## References

- [KServe v0.19.0](https://github.com/kserve/kserve/releases/tag/v0.19.0)
- [llm-d v0.8.1](https://github.com/llm-d/llm-d/releases/tag/v0.8.1)
- [vLLM v0.25.1](https://github.com/vllm-project/vllm/releases/tag/v0.25.1)
