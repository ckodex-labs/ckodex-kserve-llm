# Component Version Inventory

This document tracks the exact versions of all sub-components orchestrated by the **ckodex-kserve-llm** operator. These versions are verified against the OIS v0.1 supply-chain contract.

## 1. Inference Engines
These images power the primary data plane for LLM and Embedding workloads.

| Component | Version | Image Reference | Use Case |
| :--- | :--- | :--- | :--- |
| **vLLM** | `v0.19.0` | `vllm/vllm-openai:v0.19.0` | Optimized standard throughput. |
| **vLLM (Gemma 4)** | `gemma4` | `vllm/vllm-openai:gemma4` | Optimized for TurboQuant & Gemma 4. |
| **Quant-CPP** | `v0.1.0` | `ckodex/quant-cpp:v0.1.0` | Apple Silicon & low-memory GGUF. |

## 2. Infrastructure & Data Plane
Components responsible for model distribution and observability.

| Component | Version | Image/Resource | Use Case |
| :--- | :--- | :--- | :--- |
| **KServe Storage Init** | `v0.17.0` | `kserve/storage-initializer:v0.17.0` | Standard S3/OCI model download. |
| **CKodex Storage Init** | `v0.1.0` | `ckodex/storage-initializer:v0.1.0` | Optimized swfs:// and hf:// downloads. |
| **Vector Sidecar** | `0.54.0` | `timberio/vector:0.54.0-distroless-libc` | OIS Signal translation & OTel routing. |
| **SeaweedFS Client** | `v3.x` | Go SDK (integrated) | High-speed model weight distribution. |

## 3. Security & Governance
Zero-trust infrastructure injected into every governed workload.

| Component | Version | Image Reference | Purpose |
| :--- | :--- | :--- | :--- |
| **SPIRE Agent** | `v1.14.5` | `ghcr.io/spiffe/spire-agent:1.14.5` | Workload Identity (SVID). |
| **SPIRE Server** | `v1.14.5` | `ghcr.io/spiffe/spire-server:1.14.5` | Central Identity Authority. |
| **SPIFFE Helper** | `v0.9.0` | `ghcr.io/spiffe/spiffe-helper:0.9.0` | SVID rotation and PEM management. |
| **OPA Gatekeeper** | `v3.x` | `templates.gatekeeper.sh/v1` | Policy enforcement (GPU Quotas, etc.). |
| **Istio Proxy** | `v1.29.1` | Managed by Istio Operator | L7 DPI and egress isolation. |
| **Lula Validation** | `v0.9.5` | `defenseunicorns/lula:v0.9.5` | Automated OSCAL control assessment. |
| **OSCAL Schema** | `v1.1.2` | NIST SP 800-53 Rev 5 | Standard for security control mapping. |

---

> [!NOTE]
> All versions are pinned in `internal/controller/api/constants.go` and `internal/security/spire_reconciler.go`. Any changes to these versions trigger a new SLSA provenance receipt.
