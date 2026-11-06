# Component Version Inventory

This document tracks configured component references and their qualification status for the **ckodex-kserve-llm** operator. A pinned version does not by itself prove artifact provenance, compatibility, or runtime acceptance.

## 1. Inference Engines

These images power the primary data plane for LLM and Embedding workloads.

| Component | Version | Image Reference | Use Case |
| :--- | :--- | :--- | :--- |
| **vLLM runtime and adapter** | `v0.33.0` | `vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e` | Single default and adapter release; CUDA 13.0 only (CUDA 12.9 dropped); PyTorch 2.14.0; Medusa/MLP speculator removed; `--mamba-cache-mode` deprecated; GPU/driver acceptance pending. |
| **vLLM CPU** | `v0.33.0` | `vllm/vllm-openai-cpu:v0.33.0@sha256:87167cf0ec6e1e9607bb1cf9bf618a67d04963236a34bd955ac75b7d31a6edf5` | Digest-pinned linux/amd64 and linux/arm64 CPU image index. |
| **TensorFold** | `UNVERIFIED` | Adapter selects `tensorfold/tensorfold:v1.0.0@sha256:d8e5f2a1b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0`; configured Helm image is ignored by engine selection. | Conflicting image identities and no verified provenance; do not qualify until one image and digest are independently verified. |
| **SGLang** | `v0.5.22` | `lmsysorg/sglang:v0.5.22` | Active served-tier pin; requires adapter and target-GPU qualification. |

AMD clusters must configure an independently validated ROCm image; the
operator does not fabricate a default tag.

## 2. Infrastructure & Data Plane

Components responsible for model distribution and observability.

| Component | Version | Image/Resource | Use Case |
| :--- | :--- | :--- | :--- |
| **KServe** | `v0.21.0` | `ghcr.io/kserve/charts/kserve-resources@sha256:2d189c8c9d51e1ad658f9a973eaf9ea73b251305e7f93507adb1b559303f1e9a` | Local core integration; CRDs use the separate digest-pinned v0.21.0 kserve-crd chart. |
| **KServe Storage Init** | `v0.21.0` | `kserve/storage-initializer:v0.21.0@sha256:3a3a6a8053478c8d8d90f96fc5e54c1892c07784102dda70104374d4f89a3672` | Multi-architecture standard S3/OCI model download. |
| **Gateway API (local profile)** | `v1.6.2` CRDs; `v1.6.2` Go module | Kubernetes Gateway API standard bundle | Aligned Go module and CRDs for Envoy Gateway v1.9.2 and Agent Router v1.2.0. |
| **Envoy Gateway** | `v1.9.2` | `oci://docker.io/envoyproxy/gateway-helm:v1.9.2` | Coupled controller pin for Agent Router v1.2.0 and Gateway API v1.6.2. |
| **Agent Router (formerly Envoy AI Gateway)** | `v1.2.0` | `oci://docker.io/envoyproxy/ai-gateway-helm:v1.2.0` | Upgraded pin (candidate `v1.2.1-rc2`); coupled with Envoy Gateway v1.9.2 and Gateway API v1.6.2. |
| **llm-d core** | `v1.0.0` | Release bundle | Unified v1.0.0 organization-wide alignment across all llm-d components. |
| **llm-d Router EPP** | `v1.0.0` | `ghcr.io/llm-d/llm-d-router-endpoint-picker@sha256:024188e998f516879f68532016ca24fabca00afbdcc2af9a61b17634e197df2c` | EPP serving GA `InferencePool`; config API is `llm-d.ai/v1`. |
| **Gateway API Inference Extension** | `v1.6.2` | Official CRD bundle | GA `InferencePool` API; request-policy CRDs are supplied by Router. |
| **Hugging Face CSI** | `v0.15.1` | `ghcr.io/huggingface/charts/hf-csi-driver:0.15.1` | Optional privileged `hf-mount://` path; updated to v0.15.1. |
| **SeaweedFS** | `4.47` | `chrislusf/seaweedfs:4.47` | Used by the local integration manifest and may be managed externally in other environments; updated to 4.47. No Go SDK dependency is present. |
| **LMCache** | `v0.5.7` engine / `operator-v0.1.1` operator | Typed in-process and upstream multiprocess integration | Non-root container defaults, API-key authentication, `grpc.aio` / `pygrpc-native`, removal of legacy TCP cache server; `LMCacheEngine` remains owned by upstream operator. |
| **CKodex Storage Init** | `v0.1.0` | `ckodex/storage-initializer:v0.1.0` | Optimized swfs:// and hf:// downloads. |
| **Vector Sidecar** | `0.59.0` | `timberio/vector:0.59.0-distroless-static` | Optional telemetry sidecar; updated to 0.59.0. |

## 3. Security & Governance

Zero-trust infrastructure injected into every governed workload.

| Component | Version | Image Reference | Purpose |
| :--- | :--- | :--- | :--- |
| **SPIRE Agent** | `v1.15.4` | `ghcr.io/spiffe/spire-agent:1.15.4` | Workload identity default; updated to v1.15.4. |
| **SPIRE Server** | `v1.15.4` | `ghcr.io/spiffe/spire-server:1.15.4` | Identity authority default; keep agent and server versions aligned. |
| **SPIFFE Helper** | `v0.11.0` | `ghcr.io/spiffe/spiffe-helper:0.11.0` | SVID rotation and PEM management. |
| **OPA Gatekeeper** | `v3.23.1` target | `templates.gatekeeper.sh/v1` | Externally managed compatibility target, not an image installed by this chart; updated to v3.23.1. |
| **Istio** | `1.30.6` / `1.29.4` target | Managed outside this chart | External compatibility target; includes HTTP/2 AuthorizationPolicy security fixes. |
| **OSKAL** | caller-supplied source | Dagger source input | OSCAL schema validation; source revision must be recorded by the caller. |
| **OSCAL Schema** | `v1.1.2` | NIST SP 800-53 Rev 5 | Standard for security control mapping. |

## 4. Agent Tool Protocols & Specialized Workloads

Standardized tool calling protocols and verified specimen model workloads.

| Component / Model | Specification / Version | Source / Repository | Purpose |
| :--- | :--- | :--- | :--- |
| **Universal Tool Calling Protocol (UTCP)** | `v1.0.1` | [`universal-tool-calling-protocol/utcp-specification`](https://github.com/universal-tool-calling-protocol/utcp-specification) | Open standard for zero-wrapper-overhead native tool discovery and invocation (HTTP, gRPC, CLI). Supported across Go, Rust, TS, and Python. |
| **UTCP-MCP Bridge** | `v1.0.0` | [`universal-tool-calling-protocol/utcp-mcp`](https://github.com/universal-tool-calling-protocol/utcp-mcp) | Exposes native UTCP manuals and tools to standard Model Context Protocol (MCP) clients. |
| **Baidu Unlimited-OCR-2** | `3B` Vision-Language | [`baidu/Unlimited-OCR-2`](https://huggingface.co/baidu/Unlimited-OCR-2) | Deep document parsing, OCR, and table/formula extraction with Sparse Reference Sliding Window Attention (SR-SWA). |
| **Meta SAM 3** | Foundation model | [`facebook/sam3`](https://huggingface.co/facebook/sam3) | Segment Anything Model 3; Promptable Concept Segmentation (PCS) and visual tracking. |
| **Kimi K3.1 Pro Specimen** | Specimen | [`tiny-random/kimi-k3.1-pro`](https://huggingface.co/tiny-random/kimi-k3.1-pro) | Canonical specimen model for testing Kimi K3.1 Pro MoE (KDA, Gated-DSA) and tool-calling execution loops. |

---

> [!NOTE]
## Compatibility Notes

- The default image and runtime adapter both target vLLM `v0.33.0`; the active
  CUDA image is CUDA 13.0 only (CUDA 12.9 dropped) and requires a compatible node driver.
- GGUF/Quant-CPP is not an admitted LLM runtime in this release; a future
  llama.cpp adapter must supply an image digest and pass the shared contract
  before it can be enabled.
- SGLang is admitted at served tier only; KV-transfer, quantization,
  speculative decoding, and multi-node fields are rejected by its adapter.
- llm-d core and Router are unified under `v1.0.0` organization-wide alignment. Router
  readiness uses gRPC health on port `9003`; hosted cross-version acceptance
  remains open.
- hf-csi `v0.15.1` is the pinned lazy-mount profile; verify its CSI/runtime
  contract before enabling it in a cluster.
- The active Go module declares `go 1.26.0` and `toolchain go1.26.7`, while
  the Docker builder uses Go `1.26.9`; align these pins before claiming one
  build toolchain. Dagger Engine is `v0.21.9` and Trivy is `v0.72.0`; newer
  releases are candidates for the full Dagger gate. Cosign `v3.1.3` and the
  Dagger GitHub Action `v8.4.1` remain current as of 2026-10-18.
- The local scheduler stack is aligned to Gateway API CRDs `v1.6.2`, Envoy
  Gateway `v1.9.2`, Agent Router `v1.2.0`, GIE `v1.6.2`, and llm-d Router
  `v1.0.0`. Agent Router `v1.2.0` calls for Gateway API 1.6 CRDs and Envoy
  Gateway `v1.9.2`.
- SPIRE runtime defaults are `v1.15.4`; SPIFFE Helper remains `v0.11.0`. Gatekeeper
  and Istio are external compatibility requirements, not versions installed by this chart.
- Runtime and infrastructure candidates include SGLang `v0.5.22`, SeaweedFS `4.47`,
  LMCache `v0.5.7`, and Vector `0.59.0`.

The local scheduler acceptance profile pins Gateway API CRDs `v1.6.2`, Envoy
Gateway `v1.9.2`, Agent Router `v1.2.0`, GIE `v1.6.2`, and llm-d Router EPP
`v1.0.0`. The Go library is Gateway API `v1.6.2`. The KIND acceptance node is
Kubernetes `v1.35.8`, within Envoy Gateway v1.9's tested Kubernetes range.

## Live upstream alignment (2026-11-05)

The release tags above were checked against the upstream GitHub release APIs:

- [KServe v0.21.0](https://github.com/kserve/kserve/releases/tag/v0.21.0)
- [llm-d v1.0.0](https://github.com/llm-d/llm-d/releases/tag/v1.0.0)
- [llm-d Router v1.0.0](https://github.com/llm-d/llm-d-router/releases/tag/v1.0.0)
- [vLLM v0.33.0](https://github.com/vllm-project/vllm/releases/tag/v0.33.0)
- [SGLang v0.5.22](https://github.com/sgl-project/sglang/releases/tag/v0.5.22)
- [SeaweedFS 4.47](https://github.com/seaweedfs/seaweedfs/releases/tag/4.47)
- [LMCache v0.5.7](https://github.com/LMCache/LMCache/releases/tag/v0.5.7) / [operator-v0.1.1](https://github.com/LMCache/LMCache/releases/tag/operator-v0.1.1)
- [Gateway API v1.6.2](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.6.2)
- [Envoy Gateway v1.9.2](https://gateway.envoyproxy.io/news/releases/notes/v1.9.2/)
- [Agent Router v1.2.0](https://github.com/theagentrouter/agent-router/releases/tag/v1.2.0)

The remaining component audit is recorded in
[`docs/dependency-alignment.md`](docs/dependency-alignment.md). Upstream newer
versions listed there are candidates, not promoted runtime baselines.

LMCache remains an optional dependency. CKodex renders the in-process connector
or consumes the upstream operator's multiprocess connection ConfigMap; live
cache-hit and failover evidence remains a separate promotion gate.

## 4. Universal Tool Calling Protocol (UTCP) & Foundation Models (2026-11)

### 4.1 Universal Tool Calling Protocol (UTCP v1.0.1)
- **Organization & Specification**: [universal-tool-calling-protocol](https://github.com/universal-tool-calling-protocol), [utcp-specification](https://github.com/universal-tool-calling-protocol/utcp-specification) (v1.0.1 schema).
- **Core Abstractions**:
  - `Manual`: Root container defining tools with version metadata and security schemes.
  - `Tool`: Atomic callable capability with strict inputs JSON schema and native `tool_call_template`.
  - `tool_call_template`: Native execution binding eliminating intermediate proxy server overhead (`http`, `grpc`, `cli`, `websocket`, `sse`).
- **MCP Bridge**: [utcp-mcp](https://github.com/universal-tool-calling-protocol/utcp-mcp) enables direct projection of UTCP manuals into standard Model Context Protocol (MCP) clients without wrapper code.
- **Language SDK Alignment**:
  - Rust: `rs-utcp`
  - TypeScript: `typescript-utcp`
  - Go: `go-utcp` (integrated in pure domain kernel `internal/utcp/`)
  - Python: `python-utcp`
- **Kubernetes CRD Integration**:
  - `Agent` (`api/v1alpha2`): `utcpManuals` array in `AgentConfiguration` and `protocol: "utcp"` in `ToolDefinition`.
  - `AIPack` (`api/v1alpha2`): `ArtifactKind: UTCPManual` and `UTCPManuals` composition list.

### 4.2 Foundation Models & Specimen Integration
- **Baidu Unlimited-OCR-2** (`baidu/Unlimited-OCR-2`):
  - 3B Vision-Language document parsing and OCR foundation model.
  - Features Sparse Reference Sliding Window Attention (SR-SWA) for multi-page (100–300+ pages) document parsing into Markdown, tables, and mathematical formulas.
  - Bound via `MultimodalInferenceService` with task `ocr`, applying `--limit-mm-per-prompt image=N` and `--trust-remote-code`.
- **Meta SAM 3** (`facebook/sam3`):
  - Segment Anything Model 3 foundation model for Promptable Concept Segmentation (PCS) and visual object tracking.
  - Bound via `MultimodalInferenceService` with task `segmentation`, exposing `/v1/images/segmentations` with `--trust-remote-code`.
- **Specimen Model: Kimi K3.1 Pro** (`tiny-random/kimi-k3.1-pro`):
  - Specimen model mirroring Moonshot AI Kimi K3.1 Pro architecture (Kimi Delta Attention / KDA, Gated-DSA MoE).
  - Pre-configured in `internal/controller/wellknown.go` with minimal footprint (2 CPU, 4Gi RAM, 1 GPU, `--trust-remote-code`, `--enforce-eager`, `--tool-call-parser hermes`, `--enable-auto-tool-choice`) for rapid local, CI, and integration test passes.

