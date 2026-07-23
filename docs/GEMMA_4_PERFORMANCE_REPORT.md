# Gemma 4 Operator Notes

This document summarizes the Gemma 4 operator wiring currently present in the repo. It is not a release benchmark report and should not be treated as independently reproduced performance evidence.

## System Architecture

The following diagram illustrates the high-assurance serving stack for Gemma 4, highlighting the integrated hardening layers.

```mermaid
graph TD
    subgraph "Kubernetes Cluster"
        LLMSVC["LLMInferenceService (Gemma 4)"]
        
        subgraph "CKodex Operator (Control Plane)"
            CTRL["LLM Controller"]
            RECON["Status Reconciler (EXPERIMENTAL)"]
            WELL["Well-Known Config Registry"]
            SEC["Security Reconciler (NetworkPolicy/Vault)"]
        end
        
        subgraph "Data Plane (vLLM Node)"
            DEP["Deployment"]
            POD["Inference Pod"]
            VLLM["vLLM v0.25.1 (OpenAI-compatible)"]
        end
        
        STORAGE["SeaweedFS / OCI / HF"]
    end

    LLMSVC --> CTRL
    CTRL --> WELL
    CTRL --> SEC
    CTRL --> DEP
    WELL -- "Apply model-specific resources and args" --> DEP
    DEP --> POD
    POD --> VLLM
    VLLM -- "Fetch Artifacts" --> STORAGE
    
    RECON -- "Observability Plane" --> LLMSVC
    DEP -- "Report Readiness" --> RECON
```

## Performance Evidence

No latency, throughput, or VRAM benchmark is asserted by this repository. The
Well-Known profiles provide scheduling requests and runtime arguments only.
Measure the selected model, quantization, image digest, GPU, driver, and context
length together before setting capacity or SLOs.

### Notes

1. **NVFP4 runtime**: The default is vLLM v0.25.1 because its patch release includes a mixed-dtype Gemma/Qwen NVFP4 correctness fix.
2. **Environment Dependence**: Real latency and VRAM behavior depend on the runtime image, hardware tier, model variant, and cluster tuning.
3. **Status Hardening**: With `CKODEX_FEATURE_ENABLE_EXPERIMENTAL_STATUS_HARDENING` active, the operator provides the `DeploymentReady` condition for stronger rollout signaling.

## Hardening Details

- **Atomic Reconciliation**: All status updates use Optimistic Concurrency Control (CAS) with ResourceVersion integrity checks.
- **Supply Chain Security**: Runtime images must come from the configured allowlist, and `verified` provenance status only applies when signature, provenance, and SBOM attestation checks have been recorded successfully.
- **Resource Isolation**: NetworkPolicies are automatically provisioned to isolate egress from the ToolSurface.

Treat this file as implementation guidance, not as audited benchmark evidence.
