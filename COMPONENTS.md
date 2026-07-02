# Component Version Inventory

This document tracks the exact versions of all sub-components orchestrated by the **ckodex-kserve-llm** operator. These versions are verified against the OIS v0.1 supply-chain contract.

## 1. Inference Engines

These images power the primary data plane for LLM and Embedding workloads.

| Component | Version | Image Reference | Use Case |
| :--- | :--- | :--- | :--- |
| **vLLM** | `v0.24.0` | `vllm/vllm-openai:v0.24.0` | CUDA inference runtime. |
| **vLLM CPU** | `v0.24.0` | `vllm/vllm-openai-cpu:v0.24.0` | Local and CPU-only inference. |
| **vLLM (Gemma 4)** | `gemma4` | `vllm/vllm-openai:gemma4` | Optimized for TurboQuant & Gemma 4. |
| **Quant-CPP** | `v0.1.0` | `ckodex/quant-cpp:v0.1.0` | Apple Silicon & low-memory GGUF. |

The `v0.24.0-rocm` tag does not exist in `vllm/vllm-openai`. AMD clusters
must configure an independently validated ROCm image; the operator does not
fabricate a default tag.

## 2. Infrastructure & Data Plane

Components responsible for model distribution and observability.

| Component | Version | Image/Resource | Use Case |
| :--- | :--- | :--- | :--- |
| **KServe** | `v0.19.0` | `ghcr.io/kserve/charts/kserve-resources:v0.19.0` | Local integration baseline. |
| **KServe Storage Init** | `v0.19.0` | `kserve/storage-initializer:v0.19.0` | Standard S3/OCI model download. |
| **llm-d** | `v0.8.1` | Release bundle | Orchestration compatibility baseline. |
| **llm-d Router EPP** | `v0.9.0` | `ghcr.io/llm-d/llm-d-router-endpoint-picker:v0.9.0` | KV-aware endpoint selection. |
| **Hugging Face CSI** | `v0.11.1` | `ghcr.io/huggingface/charts/hf-csi-driver:0.11.1` | Lazy `hf-mount://` model access. |
| **SeaweedFS** | `4.37` | `chrislusf/seaweedfs:4.37` | Local S3/filer integration. |
| **CKodex Storage Init** | `v0.1.0` | `ckodex/storage-initializer:v0.1.0` | Optimized swfs:// and hf:// downloads. |
| **Vector Sidecar** | `0.54.0` | `timberio/vector:0.54.0-distroless-libc` | OIS Signal translation & OTel routing. |
| **SeaweedFS Client** | `v3.x` | Go SDK (integrated) | High-speed model weight distribution. |

## 3. Security & Governance

Zero-trust infrastructure injected into every governed workload.

| Component | Version | Image Reference | Purpose |
| :--- | :--- | :--- | :--- |
| **SPIRE Agent** | `v1.15.1` | `ghcr.io/spiffe/spire-agent:1.15.1` | Workload Identity (SVID). |
| **SPIRE Server** | `v1.15.1` | `ghcr.io/spiffe/spire-server:1.15.1` | Central Identity Authority. |
| **SPIFFE Helper** | `v0.11.0` | `ghcr.io/spiffe/spiffe-helper:0.11.0` | SVID rotation and PEM management. |
| **OPA Gatekeeper** | `v3.22.2` | `templates.gatekeeper.sh/v1` | Policy enforcement (GPU quotas, etc.). |
| **Istio** | `1.30.2` | Managed outside this chart | Service-mesh compatibility target. |
| **Lula Validation** | `v0.16.0` | Release binary verified by checksum in Dagger | OSCAL control assessment. |
| **OSCAL Schema** | `v1.1.2` | NIST SP 800-53 Rev 5 | Standard for security control mapping. |

---

> [!NOTE]
## Compatibility Notes

- vLLM `v0.24.0` removes `--enable-v2-runner`,
  `--num-speculative-tokens`, `--speculative-model`, `--swap-space`, and
  `--gptq-ckpt-path`. The operator emits the replacement flags.
- llm-d `v0.8.1` consumes Router EPP `v0.9.0`; the image was renamed from
  `llm-d-inference-scheduler`, and readiness uses gRPC health on port `9003`.
- hf-csi `v0.11.1` fixes termination-grace propagation and a CI workflow
  vulnerability. The chart version is pinned in the local installer.
- KServe `v0.19.0`, Gateway API `v1.6.0`, SPIRE `v1.15.1`, SPIFFE Helper
  `v0.11.0`, Gatekeeper `v3.22.2`, Istio `1.30.2`, Trivy `v0.72.0`, Syft
  `v1.46.0`, Cosign `v3.1.1`, and Dagger `v0.21.7` were checked on
  2026-07-01. CI tool upgrades remain separate changes from runtime support.
