# Component catalog and data flows

This document details the system components of the CKodex KServe LLM Operator and traces the four primary data flows across the control and data planes.

## Component catalog

```mermaid
flowchart TD
    subgraph ControlPlane["Operator Control Plane: ckodex-system"]
        CM["Controller Manager: cmd/manager"]
        Val["Validation Pipeline: internal/validation"]
        DiffEngine["Diff Normalizer: internal/controller/reconciler/diff.go"]
        GateFence["Gateway Fencing: internal/gateway/fence.go"]
        SpireRec["SPIRE Reconciler: internal/security"]
    end

    subgraph IngressPlane["Ingress Plane: envoy-gateway-system"]
        EG["Envoy Gateway 1.2.0"]
        EPP["EPP Scheduler: llm-d Router"]
    end

    subgraph WorkloadPlane["Workload Plane: tenant namespaces"]
        Init["Storage Initializer: cmd/storage-initializer"]
        Runtime["Inference Engine: vLLM v0.33.0"]
        LMCache["LMCache Engine: v0.5.6"]
        LocalCache["LocalModelCache Daemon"]
    end

    subgraph IdentityPlane["Identity and Audit"]
        SPIRE["SPIRE Server / Agent"]
        FlightRec["RFC 9162 Merkle Flight Recorder"]
    end

    CM --> Val
    CM --> DiffEngine
    CM --> GateFence
    CM --> SpireRec
    GateFence --> EG
    SpireRec --> SPIRE
    CM --> WorkloadPlane
    EG --> EPP
    EPP --> Runtime
    Init --> Runtime
    Runtime <--> LMCache
    LocalCache --> Init
    CM --> FlightRec
```

### 1. Controller manager (`cmd/manager/main.go`, `internal/controller/`)
The central Kubernetes operator process. Hosts reconcilers for:
- `LLMInferenceService`: Reconciles deployments, services, and routes for text and chat models.
- `AIPack`: Manages immutable OCI artifact admission and verification across 16 kinds.
- `MultimodalInferenceService`: Deploys OCR (Baidu Unlimited-OCR-2) and segmentation (Meta SAM 3) models.
- `Agent`: Reconciles agent identity, tool manuals, and skill bindings.
- `LocalModelCache`: Pre-stages model weights to node NVMe storage.

### 2. Storage initializers (`cmd/storage-initializer/`, `cmd/huggingface-initializer/`)
Init containers executed before the inference engine starts:
- Standard storage initializer: Downloads weights from S3, GCS, or OCI registries.
- Hugging Face initializer: Authenticates against Hugging Face, downloads repository checkpoints, and verifies Cosign signatures.
- Local cache binder: Detects existing `LocalModelCache` paths and binds local storage directly.

### 3. Gateway and routing plane (`internal/gateway/`)
Coordinates ingress traffic through Gateway API v1.6.2 and Envoy Gateway 1.2.0:
- HTTPRoute and GRPCRoute provisioning.
- Automated security response headers injection (`Cache-Control: no-store`, `X-Content-Type-Options: nosniff`).
- Route fencing engine (`fence.go`): Strips routing forwarders within 5 seconds during security incidents while preserving route rules.

### 4. Scheduler and KV cache fabric
- **EPP Scheduler (`llm-d Router`)**: Provides KV-cache-aware scheduling across replicas by monitoring queue depths and cache factors.
- **LMCache Engine v0.5.6**: Manages in-process connectors (`LMCacheConnectorV1`) or node-local shared-memory daemons via CUDA IPC and `/dev/shm`.

### 5. Universal Tool Calling Protocol engine (`internal/utcp/`)
Pure Go protocol implementation:
- Validates UTCP v1.0.1 manuals against strict JSON schemas.
- Converts between UTCP manuals, OpenAI tool formats, and Model Context Protocol (MCP) servers.
- Enforces security boundaries on tool execution templates.

### 6. Workload identity and cryptographic audit
- **SPIRE Agent and Server**: Issues short-lived X.509 SVIDs directly to pods over a local Unix domain socket for mutual TLS.
- **RFC 9162 Merkle Flight Recorder**: Emits signed Ed25519 receipts for state mutations and admission decisions into an append-only cryptographic ledger.

---

## Primary data flows

### 1. Workload deployment flow

```mermaid
sequenceDiagram
    autonumber
    actor Engineer as Platform Engineer
    participant API as Kubernetes API Server
    participant CM as Controller Manager
    participant Val as Validation Space
    participant Node as K8s Node
    participant Init as Storage Initializer
    participant Engine as vLLM v0.33.0

    Engineer->>API: Apply LLMInferenceService manifest
    API->>CM: Reconcile event
    CM->>Val: Evaluate 4-stage admission pipeline
    Val-->>CM: Disposition ADMIT
    CM->>API: Create Deployment, Service, and HTTPRoute
    API->>Node: Schedule workload Pod
    Node->>Init: Launch init container
    Init->>Init: Verify Cosign signature & download weights
    Init-->>Node: Weights ready in shared volume
    Node->>Engine: Start vLLM container
    Engine->>Engine: Warm KV cache & compile CUDA graphs
    Engine->>API: Health check /health passes (Ready)
    CM->>API: Update status condition to Ready=True
```

### 2. Inference request flow

```mermaid
sequenceDiagram
    autonumber
    actor Client as API Client
    participant EG as Envoy Gateway
    participant EPP as EPP Scheduler
    participant Pod as vLLM Worker Pod
    participant LMCache as LMCache Memory

    Client->>EG: POST /v1/chat/completions (mTLS)
    EG->>EPP: Route request
    EPP->>EPP: Evaluate replica KV cache affinity
    EPP->>Pod: Forward request to selected pod
    Pod->>LMCache: Check prefix KV tensors
    alt Cache hit
        LMCache-->>Pod: Return cached attention tensors (85% faster TTFT)
    else Cache miss
        Pod->>Pod: Compute full prompt prefill
    end
    Pod->>Pod: Autoregressive token generation
    Pod-->>EG: Stream completion tokens
    EG-->>Client: Stream response with security headers
```

### 3. Autonomous agent tool execution flow

```mermaid
sequenceDiagram
    autonumber
    actor User as Agent User
    participant Agent as Agent Client
    participant Model as LLMInferenceService
    participant UTCP as UTCP Engine
    participant Tool as Tool Service (OCR)

    User->>Agent: "Extract invoice table"
    Agent->>Model: Prompt with bound UTCP tools
    Model-->>Agent: Emit tool call: parse_invoice(doc_url)
    Agent->>UTCP: Validate payload against manual schema
    UTCP-->>Agent: Validation passed
    Agent->>Tool: Dispatch HTTP request to cluster service
    Tool-->>Agent: Return structured Markdown tables
    Agent->>Model: Return tool output
    Model-->>User: Final reasoned answer
```

### 4. Route fencing flow

```mermaid
sequenceDiagram
    autonumber
    participant Mon as Anomaly Monitor
    participant Vector as State Vector S(e,t)
    participant Fence as Gateway Fencer
    participant Route as Envoy HTTPRoute
    participant Attacker as Untrusted Traffic

    Mon->>Vector: Report invariant failure (e.g. digest mismatch)
    Vector->>Vector: Transition coherence to DECOHERENT
    Vector->>Fence: Trigger SAFE_HOLD or QUARANTINED
    Fence->>Route: Strip backend forwarders (< 5 seconds)
    Attacker->>Route: Incoming request
    Route-->>Attacker: HTTP 503 / Dropped immediately
```

## Next steps

- [Consult the Four Spaces architecture overview](overview.md)
- [Review security architecture and capability leases](security-architecture.md)
- [Learn compliance verification with OSCAL](compliance-oscal.md)
