# Inference engine runtime contract

This normative specification defines the engineering contract for inference runtimes managed by the CKodex KServe LLM Operator. It details capability declarations, conformance tiers, deterministic rendering invariants, and runtime integration requirements.

## Core contract invariant

Every engine adapter in `internal/runtime/` must implement a **total capability function**:

```go
Capabilities() inferenceruntime.CapabilityMatrix
```

The capability matrix permits no absent entries. Every capability must return `Supported`, `Unsupported`, or `Emulated`. Adding a new capability enum value breaks compilation across all adapters until every engine explicitly declares its support stance.

Admission controllers enforce these declarations: if a custom resource requests a feature marked `Unsupported` by the chosen engine, admission rejects the resource with an explanatory error.

## Conformance tiers

Runtimes are categorized into three formal conformance tiers:

| Tier | Name | Required capabilities | Reference runtime in v0.20.0 |
|---|---|---|---|
| **Tier 3** | Governed | Full OpenAI API, health probes, Prometheus metrics, disaggregated prefill/decode, KV-aware scheduling (EndpointPicker), LoRA hot-swap, deterministic replay, execution receipts | **vLLM v0.33.0** (`vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e`) |
| **Tier 2** | Routed | OpenAI API, health probes, metrics, KV-cache-aware scheduling via EndpointPicker series | Target tier for scheduled engines |
| **Tier 1** | Served | Standard `/v1/chat/completions` and `/v1/models` endpoints, health probes, basic metrics | **SGLang v0.5.22** (implements `ConformanceTierServed` in current Go adapter contract), **TensorFold v1.0.0** |
| **Tier 0** | Experimental | Feature-gated development runtimes; rejected in production admission | Untested community runtimes |

## The eight architectural clauses

### C0: Identity and provenance
- The adapter name must be a DNS-label-safe identifier matching `spec.engine` (for example, `vllm`, `sglang`, `tensorfold`).
- Images must resolve to explicit digests in production manifests, never mutable tags.
- Missing hardware variants are omitted rather than fabricated. For example, ROCm images without official upstream verification are omitted with an explicit operator error.

### C1: Capability matrix declaration
- The adapter must implement `Capabilities()` over all dimensions: `TensorParallel`, `PipelineParallel`, `DataParallel`, `ExpertParallel`, `KVCacheDtype`, `CPUOffload`, `KVTransfer`, `SpeculativeDecoding`, `Quantization`, and `LoRAHotSwap`.
- `Validate()` must return a `field.Error` for any requested spec field marked `Unsupported`.

### C2: Deterministic argument rendering
- The `Render(request)` function must be purely functional: identical `RenderRequest` inputs produce byte-identical command line arguments, environment variables, ports, and probes.
- Zero clock reads, randomness, or cluster API lookups are permitted inside `Render()`.
- Explicit user arguments in `spec.template.spec.containers` take precedence over adapter defaults.

### C3: OpenAI serving contract
- The runtime must expose `/v1/chat/completions`, `/v1/completions`, and `/v1/models` on the declared container port (default 8000).
- Independent probes:
  - Startup probe: Accommodates large model loading and weight pre-staging (up to 10 minutes without tripping liveness).
  - Readiness probe: Signals readiness to receive traffic via Gateway API.
  - Liveness probe: Detects deadlocks or hung processes on `/health`.
- Graceful termination: Runtimes must intercept `SIGTERM` and drain active inference streams within `terminationGracePeriodSeconds`.

### C4: Metrics and observability contract
- Exposes Prometheus metrics on `/metrics` (or declared port/path).
- Reports the four canonical series for KV-aware scheduling:
  1. `vllm:num_requests_queued`
  2. `vllm:num_requests_running`
  3. `vllm:gpu_cache_usage_factor`
  4. `vllm:num_active_loras`

### C5: Accelerator and parallelism contract
- Supports tensor parallelism (`--tensor-parallel-size`) across single-node GPUs.
- Supports pipeline parallelism (`--pipeline-parallel-size`) and Ray 2.45.1 clustering for multinode distributed execution.
- Configures expert parallelism and load balancing (`--enable-eplb`) for Mixture-of-Experts architectures like Kimi-K3 and DeepSeek-V3.

### C6: Key-value cache and transfer contract
- Supports typed KV cache datatypes: `auto`, `fp16`, `bf16`, `fp8`, and `nvfp4`.
- Integrates with LMCache connector specifications (`LMCacheConnectorV1` or `LMCacheMPConnector`) for prefix caching and shared memory transfers.

### C7: Security and sandboxing invariants
- Container command overrides are rejected to prevent bypassing security flags.
- FastAPI documentation endpoints (`/docs`, `/redoc`) remain disabled (`--disable-fastapi-docs`).
- Remote code execution requires explicit opt-in (`--trust-remote-code`).
- CORS parameters reject wildcard (`*`) origins and require explicit trusted domains.

## Modern feature annotations for vLLM v0.33.0

The vLLM adapter in `internal/runtime/vllm/adapter.go` recognizes declarative workload annotations:

| Annotation | Effect on container arguments | Purpose |
|---|---|---|
| `serving.ckodex.com/fast-restart: "true"` | Injects `--load-format ipc_cache --enable-fast-restart` | Reclaims shared memory weights after pod restart |
| `serving.ckodex.com/enable-flashmla: "true"` | Injects `--enable-flashmla` | Enables Flash Multi-Head Latent Attention kernels |
| `serving.ckodex.com/mega-gate: "true"` | Injects `--enable-mega-gate` | Enables hardware-accelerated MoE router gating |
| `serving.ckodex.com/trust-request-mm-kwargs: "true"` | Injects `--trust-request-mm-kwargs` | Permits dynamic multimodal image processing kwargs |
