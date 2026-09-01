# Capability and conformance audit — 2026-08-28

## Verdict

The repository supports a narrow, locally tested vLLM LLM-serving profile. It
does not yet support the full multi-engine and governed inference-system use
case described in the accompanying design text. The statement “every test was
bidirectional” would be false.

## Engine inventory

| Surface | Observed contract | Evidence classification |
|---|---|---|
| LLM `spec.engine` | The CRD enum admits `vllm` and `sglang` | C at schema/admission level |
| First-class runtime adapter seam | `internal/runtime/vllm/adapter.go` and `internal/runtime/sglang/adapter.go` implement `runtime.Adapter` behind an immutable registry | C locally; both adapters remain served tier |
| vLLM and SGLang | Argument rendering, explicit-argument precedence, image/health/metrics contracts, capability declaration, and rejection of unsupported fields are tested | C locally; live runtime acceptance remains open |
| Quant-CPP | Admission and legacy image configuration are removed from the release surface | S/A: future llama.cpp adapter work remains tracked in L-RT-003 |
| llama.cpp, TensorRT-LLM, MLX | No LLM adapter or registration exists | A for this checkout |
| ASR and embedding runtimes | Separate specialized CRDs admit several runtime strings | Separate controller surfaces, not evidence of a common LLM engine contract |

The engine contract itself is explicitly classified `S`; it says the vLLM and
SGLang adapters implement only tier-1 subsets and that image, metrics, receipt,
health, workload extraction, and higher-tier work remain open. See
[`docs/engine-contract.md`](../engine-contract.md).

## What was actually tested bidirectionally

| Area | Forward direction | Reverse direction | Result |
|---|---|---|---|
| v1/v1alpha2 conversion | alpha2 fixture converts to the v1 hub | hub converts back and fields/status are compared; fuzz target checks round-trip equality | Tested locally |
| Generated CRD schema | Registered Go types are projected into expected JSON field names | Generated CRD field names are compared back to the Go projection | Tested locally |
| Vector-state metrics | Spec tuple names can be emitted as OTel attributes | Runtime metric name and `tuple_type` attribute are checked against the spec | Tested locally |
| Vector-state forbidden tuples | Counter coverage exists for four tuple names; structural tests cover anti-execute and empty-high-DAL | No structural runtime proof was found for active-untrusted or negative-escalation-skipped | Partial |
| Engine capability contract | vLLM and SGLang fields render through registered adapters and unsupported fields are rejected | Registry/schema direction and adapter-specific validation are tested; a generated all-capability correspondence table is still absent | Partial |
| Cross-engine behavior | No shared engine matrix runs the same service through multiple engines | No runtime-to-spec equivalence or receipt/metric parity test exists | Missing |
| Pasted system use cases | No conformance vectors cover Light/Medium/High routing, fairness queues, capacity epochs, or GPU residency | No evidence reconstruction proves those behaviors occurred | Missing |

The focused command below passed. Its pass result means the existing vectors
passed; it does not mean the missing vectors were silently run:

```text
go test -race -count=1 ./test/conformance/... ./internal/runtime/... ./internal/controller/deployment
```

The conformance directory now contains API/schema, AIPack, engine-registry, and
vector-state tests. Engine registry tests cover admitted-vs-registered values
in both directions; the broader cross-engine serving and receipt suite is still
absent.

## Pasted use-case disposition

| Use case | Repository evidence | Classification |
|---|---|---|
| Intra-model endpoint routing using queue/KV/prefix signals | EPP ConfigMap generation includes queue, KV-utilization, and prefix-cache scorers; Gateway API InferencePool semantics keep one model-server pool per EPP | C configuration path; live hosted behavior pending |
| Inter-model Light/Medium/High intelligence routing | Bounded access-plane policy evaluator and optional RequestPipeline gate exist, but no `IntelligenceRoute`, `InferenceSystemPolicy`, or external production caller exists | S: verified seam, not a live tier router |
| Prefill/decode separation | `spec.prefill` creates a separate prefill Deployment and assigns KV producer/consumer roles | S: code path and unit tests; no live transfer/accuracy/failover proof |
| Weight cache | `LocalModelCache` reconciles warmup PVCs/jobs and LRU eviction | C for model-weight lifecycle primitives; live profile proof pending |
| LMCache KV sharing/offload | Typed config and container wiring exist for in-process and multiprocess modes | S: no live cache-hit, latency, ABI, or failover evidence |
| Four-level tenant/tier/endpoint/engine queues | Access-plane evaluator enforces tenant/target concurrency and queue-capacity decisions; no fair queue executor or production caller exists | S |
| Backpressure with deterministic 429/deadline semantics | RequestPipeline enforces policy decisions and deadlines before endpoint work; no external HTTP caller maps the typed outcomes to 429/Retry-After | S |
| GPU residency and High epochs | Residency FSM implements guarded cold/cached/loading/warming/ready/draining/evicting transitions and reverse projection; durable owner-effect execution remains open | S |
| Exact 3-GPU / 10-user profile | No current real-GPU, ten-user, throughput, tail-latency, or recovery acceptance artifact exists | A as a support claim |

The operator's own status code says that a configured KV connector is not
evidence of cache hits and requires live validation. See
[`internal/controller/status/reconciler.go`](../../internal/controller/status/reconciler.go:139).
The beta plan also explicitly excludes automated promotion, LMCache
performance, cryptographic verification, and unsupported specialized runtime
claims until acceptance evidence exists. See
[`docs/beta/plan.md`](../beta/plan.md:14).

## Current official upstream boundary

- [Gateway API InferencePool](https://gateway-api-inference-extension.sigs.k8s.io/api-types/inferencepool/)
  defines a pool around a shared compute configuration, base model, and model
  server; its EPP selects among replicas and an EPP associates with one pool.
- [KServe control-plane API](https://kserve.github.io/website/docs/reference/crd-api)
  documents `prefill` as a separate deployment for prompt processing.
- [vLLM disaggregated prefilling](https://docs.vllm.ai/en/stable/features/disagg_prefill/)
  states that the feature is experimental and uses separate prefill/decode
  instances with KV transfer.
- [vLLM LMCache examples](https://docs.vllm.ai/en/stable/examples/disaggregated/lmcache/)
  distinguish in-process and multiprocess modes and call out distributed KV
  storage as a separate integration path.
- [SGLang v0.5.18 release](https://github.com/sgl-project/sglang/releases/tag/v0.5.18)
  and [server arguments](https://github.com/sgl-project/sglang/blob/main/docs/cookbook/base/reference/server_arguments.mdx)
  provide the release and launch-flag references used by the adapter.

These upstream capabilities validate the architectural direction; they do not
prove that this repository has implemented or accepted them end to end.

## Required proof before broad support is claimed

1. Maintain the registry and shared engine suite for each admitted engine, with
   capability-to-validation correspondence and cross-engine OpenAI,
   readiness, metrics, receipt, and failure semantics.
2. Keep Quant-CPP unadmitted until a verified llama.cpp adapter exists.
3. Define the cross-model routing and capacity-lease API, then test both
   policy-to-runtime compilation and runtime evidence-to-policy reconstruction.
4. Add live CPU and real-GPU profile evidence for P/D, LMCache hits/failover,
   bounded queues, backpressure, residency transitions, and recovery.
5. Keep exact-head hosted CI, Nightly, provenance, and public artifact checks
   separate from local unit/conformance results.

## Local command record

```text
go test -race -count=1 ./test/conformance/... ./internal/runtime/... ./internal/controller/deployment  PASS
go test -race ./...                                                        PASS
go vet -a ./...                                                            PASS
make release-readiness                                                     PASS
```

The local commands are evidence for the tested paths only. They are not a
release or broad-runtime acceptance decision.
