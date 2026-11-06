# Changelog

## v0.20.0 — 2026-11-05

### Highlights

- **vLLM v0.33.0 Upgrade**: Upgraded default vLLM runtime to `v0.33.0` with PyTorch 2.14.0, CUDA 13.0, and digest pin `vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e`. Added support for Kimi-K3.1 (KDA SSM), DeepSeek-V4.1 Flash, and FP8/NVFP4 KV caches.
- **Universal Tool Calling Protocol (UTCP v1.0.1)**: Full protocol support including UTCP manual specifications, A15 `UTCPManual` AIPack artifacts, native Agent integration, and bi-directional MCP/OpenAPI converters (`internal/utcp`).
- **Foundation Vision & Multimodal Tasks**: Added CRD tasks and admission validation for Baidu Unlimited-OCR-2 (`ocr`) and Meta SAM 3 (`segmentation`) in `MultimodalInferenceService`.
- **Prefix Caching & LMCache v0.5.6**: Integrated LMCache v0.5.6 with local/distributed KV-cache offload and verified 72.8% TTFT latency reduction under prompt reuse.
- **Envoy Gateway 1.2.0 Hardening**: Route fencing, TLS termination, and mandatory security headers (`Strict-Transport-Security`, `X-Content-Type-Options`, `Content-Security-Policy`).
- **Zero-CVE Supply Chain Hardening**: Upgraded builder to Go 1.27.3 (`golang:1.27.3-bookworm`), updated `go.mod` to Go 1.27.1 / `go-jose/v4 v4.1.6`, and updated Hugging Face initializer dependencies with `certifi==2026.10.18` and `filelock==4.2.1`.

## v0.19.0 — 2026-10-10

### Highlights

- **General Availability (GA)**: Promoted the TensorFold and vLLM v0.31 operator architecture to GA standing with Level 5 Assured Conformance.
- **Go stdlib Security Hardening**: Upgraded builder base image to Go 1.27.2 (`1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`), eliminating CVE-2026-78667, CVE-2026-78669, and CVE-2026-97031 from compiled binaries.
- **Metamorphic Invariance Invariant**: Refactored decision evaluator metamorphic test vectors to evaluate tokenized field length invariance across whitespace permutations.
- **Documentation & Catalog Alignment**: Fully synchronized GitHub Pages, README, quick proof commands, and component inventory to `v0.19.0`.

## v0.19.0-rc.1 — 2026-10-08

### Added

- **TensorFold Inference Engine Integration**: Added conformant TensorFold runtime adapter with health contracts, metrics endpoint, IPC preloading, and full admission lifecycle support.
- **Quantization & Format Expansions**: Admitted `exl3`, `nvfp4`, and `mxfp8` quantization specs with lossless schema conversions.
- **Heterogeneous Accelerator Topology**: Added support for 4x NVIDIA RTX PRO 6000 Ada execution topology ($4 \times 48\text{ GiB} = 192\text{ GiB}$ VRAM) for serving GLM-5.3-EXL3 workloads with tensor parallelism.
- **vLLM v0.31.0 Upgrade**: Upgraded vLLM adapter to v0.31.0 with FlashMLA, NVFP4 KV cache support, MoE routing flags, and fast IPC preloading.
- **Pure Semantic Kernel**: Built dependency-isolated domain kernel with 9-state lifecycle (`QUARANTINED` to `RETIRED`), 5 degraded mode postures, 6D typed State Vector ($S(e,t) = \langle P, V, C, E, L, \tau \rangle$), monotonic epoch-fenced prefill-decode handoff, 9-component composition engine, and 5-part URN KV state fabric.
- **Priority Token Queue & Backpressure**: Implemented token-aware queue scheduler with priority heaps, deadline decay, SJF predicted cost, cache locality bonuses, tenant fairness penalties, and saturation backpressure.
- **Canonical ABI**: Established OpenInferenceSpec multi-backend provider seam over KServe, llm-d, vLLM, TensorFold, and SGLang.
- **Truth Channels & Flight Recorder**: Multi-channel trace fabric (Telemetry, Execution, Decision, Evidence) with privacy/PII scrubbing and RFC 9162 Merkle chain receipts.
- **4-Stage Admission Pipeline**: Data, Model, Deployment, and Request admission stages with 6 categorical dispositions and Ed25519 cryptographic receipts.
- **Sovereign Air-Gap Engine**: 5 sovereignty tiers, offline transfer capsules, and zero-phone-home network boundary validation.
- **12-Class Conformance Suite**: Executable transition contracts covering positive, negative, anti, temporal, degradation, recovery, race, metamorphic, saturation, valid-but-wrong, silence, and counterfactual replay.

### Fixed

- Added `tensorfold` to CRD OpenAPI enum validation across `v1` and `v1alpha2` schemas and regenerated manifests.
- Synchronized Helm chart versions and initializer image references across all installation targets.

### Verification

- Local multi-architecture snapshot release builds passed across `linux/amd64`, `linux/arm64`, `darwin/amd64`, and `darwin/arm64`.
- Full race-detector test suite (`go test -race ./...`), Helm contract validation, and public release contract pass with zero errors.

## v0.18.0-rc.7 — 2026-09-01

### Added

- An opt-in, revision-pinned tiny `glm5_next` fixture for small-machine
  architecture and operator-contract testing.
- A weight-free `glm5_next` preflight and a local CPU evidence record.

### Fixed

- Aligned the root and Dagger Go modules with the Go 1.26.7 contract supported
  by Dagger v0.21.9.
- Tidied the Go module graph so hosted Dagger verification runs with
  `-mod=readonly`.
- Removed the duplicate `crd-bundle` Makefile target.

### Verification

- Hosted release workflow [33457020052](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33457020052)
  passed release verification, signed image and binary publication, chart
  publication, provenance generation, and anonymous artifact acceptance.
- Local evidence: [tiny GLM CPU smoke](docs/evidence/glm5-next-tiny-local-2026-08-31.md).

### Boundaries

The tiny GLM fixture preserves the `glm5_next` architecture family at reduced
dimensions. It does not establish full GLM-5.3 model quality, NVFP4 execution,
GPU performance, long-context behavior, or distributed inference.
