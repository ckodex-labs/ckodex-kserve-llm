# Component Version Inventory

This document tracks the exact versions of all sub-components orchestrated by the **ckodex-kserve-llm** operator. These versions are verified against the OIS v0.1 supply-chain contract.

## 1. Inference Engines

These images power the primary data plane for LLM and Embedding workloads.

| Component | Version | Image Reference | Use Case |
| :--- | :--- | :--- | :--- |
| **vLLM** | `v0.28.0` | `vllm/vllm-openai:v0.28.0` | CUDA inference runtime; operator default. |
| **vLLM CPU** | `v0.28.0` | `vllm/vllm-openai-cpu:v0.28.0` | Local and CPU-only inference; explicit opt-in. |
| **vLLM (Gemma 4)** | `v0.28.0` | `vllm/vllm-openai:v0.28.0` | NVFP4-capable runtime; model is selected by the workload. |
| **SGLang** | `v0.5.18` | `lmsysorg/sglang:v0.5.18@sha256:9e148f…c29a1a1` | Served-tier CUDA runtime; explicit opt-in with reduced capability surface. |

AMD clusters must configure an independently validated ROCm image; the
operator does not fabricate a default tag.

## 2. Infrastructure & Data Plane

Components responsible for model distribution and observability.

| Component | Version | Image/Resource | Use Case |
| :--- | :--- | :--- | :--- |
| **KServe** | `v0.20.0` | `ghcr.io/kserve/charts/kserve-resources:v0.20.0` | Local integration baseline. |
| **KServe Storage Init** | `v0.20.0` | `kserve/storage-initializer:v0.20.0` | Standard S3/OCI model download. |
| **Gateway API (local profile)** | `v1.5.1` | Kubernetes Gateway API standard bundle | HTTPRoute and GatewayClass contract used by the local profile. |
| **Envoy Gateway** | `v1.8.1` | `oci://docker.io/envoyproxy/gateway-helm:v1.8.1` | Gateway API controller and data-plane proxy. |
| **Envoy AI Gateway** | `v1.1.0` | `oci://docker.io/envoyproxy/ai-gateway-helm:v1.1.0` | InferencePool extension manager and AI routing integration. |
| **llm-d core** | `v0.9.0` | Release bundle | Orchestration compatibility baseline. |
| **llm-d Router EPP** | `v0.10.0` | `ghcr.io/llm-d/llm-d-router-endpoint-picker@sha256:2e516f…aaaa70` | Executable GA `InferencePool` KV-aware endpoint selection. |
| **Gateway API Inference Extension** | `v1.5.0` | Official CRD bundle | GA `InferencePool` API retained by the local profile; the EPP executable moved to llm-d Router. |
| **Hugging Face CSI** | `v0.14.0` | `ghcr.io/huggingface/charts/hf-csi-driver:0.14.0` | Lazy `hf-mount://` model access. |
| **SeaweedFS** | `4.40` | `chrislusf/seaweedfs:4.40` | External S3/filer integration target; not installed by this chart. |
| **LMCache** | `operator-v0.1.1` | Typed in-process and upstream multiprocess integration | `LMCacheEngine` remains owned by the upstream operator; see L-OP-006. |
| **CKodex Storage Init** | `v0.1.0` | `ckodex/storage-initializer:v0.1.0` | Optimized swfs:// and hf:// downloads. |
| **Vector Sidecar** | `0.58.0` | `timberio/vector:0.58.0-distroless-static` | OIS Signal translation & OTel routing. |
| **SeaweedFS Client** | `v3.x` | Go SDK (integrated) | High-speed model weight distribution. |

## 3. Security & Governance

Zero-trust infrastructure injected into every governed workload.

| Component | Version | Image Reference | Purpose |
| :--- | :--- | :--- | :--- |
| **SPIRE Agent** | `v1.15.3` | `ghcr.io/spiffe/spire-agent:1.15.3` | Workload Identity (SVID). |
| **SPIRE Server** | `v1.15.3` | `ghcr.io/spiffe/spire-server:1.15.3` | Central Identity Authority. |
| **SPIFFE Helper** | `v0.11.0` | `ghcr.io/spiffe/spiffe-helper:0.11.0` | SVID rotation and PEM management. |
| **OPA Gatekeeper** | `v3.23.0` | `templates.gatekeeper.sh/v1` | Policy enforcement (GPU quotas, etc.). |
| **Istio** | `1.30.3` | Managed outside this chart | Service-mesh compatibility target. |
| **Lula Validation** | `v0.16.0` | Release binary verified by checksum in Dagger | OSCAL control assessment. |
| **OSCAL Schema** | `v1.1.2` | NIST SP 800-53 Rev 5 | Standard for security control mapping. |

---

> [!NOTE]
## Compatibility Notes

- vLLM `v0.28.0` removes `--enable-v2-runner`,
  `--num-speculative-tokens`, `--speculative-model`, `--swap-space`, and
  `--gptq-ckpt-path`. The operator emits the replacement flags.
- GGUF/Quant-CPP is not an admitted LLM runtime in this release; a future
  llama.cpp adapter must supply an image digest and pass the shared contract
  before it can be enabled.
- SGLang is admitted at served tier only; KV-transfer, quantization,
  speculative decoding, and multi-node fields are rejected by its adapter.
- llm-d core `v0.9.0` is paired with llm-d Router EPP `v0.10.0`; the image is
  `ghcr.io/llm-d/llm-d-router-endpoint-picker` and readiness uses gRPC health
  on port `9003`.
- hf-csi `v0.14.0` is the pinned lazy-mount profile; verify its CSI/runtime
  contract before enabling it in a cluster.
- KServe `v0.20.0`, Gateway API library `v1.6.1`, SPIRE `v1.15.3`, SPIFFE Helper
  `v0.11.0`, Gatekeeper `v3.23.0`, Istio `1.30.3`, Trivy `v0.72.0`,
  Cosign `v3.1.3`, Dagger `v0.21.9`, and Go `v1.27.0` were checked on
  2026-08-27. The CI, release, and image-build pins use these versions.

The local scheduler acceptance profile intentionally qualifies Gateway API
`v1.5.1`, Envoy Gateway `v1.8.1`, Envoy AI Gateway `v1.1.0`, and llm-d Router
EPP `v0.10.0` together; the InferencePool backend extension is not a property
of Envoy Gateway alone. The Go library is checked at Gateway API `v1.6.1`,
while the installed CRD bundle remains `v1.5.1` for this tested integration
profile.

## Live upstream alignment (2026-08-27)

The release tags above were checked against the upstream GitHub release APIs:

- [KServe v0.20.0](https://github.com/kserve/kserve/releases/tag/v0.20.0)
- [llm-d v0.9.0](https://github.com/llm-d/llm-d/releases/tag/v0.9.0)
- [llm-d Router v0.10.0](https://github.com/llm-d/llm-d-router/releases/tag/v0.10.0)
- [vLLM v0.28.0](https://github.com/vllm-project/vllm/releases/tag/v0.28.0)
- [SGLang v0.5.18](https://github.com/sgl-project/sglang/releases/tag/v0.5.18)
- [SeaweedFS 4.40](https://github.com/seaweedfs/seaweedfs/releases/tag/4.40)
- [LMCache operator-v0.1.1](https://github.com/LMCache/LMCache/releases/tag/operator-v0.1.1)

LMCache remains an optional dependency. CKodex renders the in-process connector
or consumes the upstream operator's multiprocess connection ConfigMap; live
cache-hit and failover evidence remains a separate promotion gate.
