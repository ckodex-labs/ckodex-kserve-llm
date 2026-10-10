# Changelog

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
