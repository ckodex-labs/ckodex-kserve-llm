# CKodex KServe LLM Operator

[![Go Report Card](https://goreportcard.com/badge/github.com/ckodex-labs/ckodex-kserve-llm)](https://goreportcard.com/report/github.com/ckodex-labs/ckodex-kserve-llm)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Documentation](https://img.shields.io/badge/docs-v0.20.0-brightgreen.svg)](docs/README.md)
[![Release](https://img.shields.io/badge/release-v0.20.0-blue.svg)](https://github.com/ckodex-labs/ckodex-kserve-llm/releases/tag/v0.20.0)
[![Security Scorecard](https://img.shields.io/ossf-scorecard/github.com/ckodex-labs/ckodex-kserve-llm?label=scorecard&style=flat)](https://securityscorecards.dev/viewer/?uri=github.com/ckodex-labs/ckodex-kserve-llm)

The CKodex KServe LLM Operator automates the deployment, routing, scaling, governance, and cryptographic attestation of large language models and multimodal foundation models on Kubernetes.

For a step-by-step introduction, start with the [getting started tutorial](docs/tutorials/getting-started.md). For an architectural breakdown, read the [architecture overview](docs/explanation/overview.md). For complete documentation organized by the Diátaxis framework, visit the [documentation hub](docs/README.md).

## Core capabilities in release v0.20.0

Release `v0.20.0` (commit `9da7f18`) establishes production defaults across the inference control plane:

- **vLLM v0.33.0 runtime engine**: Pinned to `vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e`. Built on PyTorch 2.14.0 and CUDA 13.0. Supports the Kimi-K3 architecture family (Gated-DSA, MoE, and KDA linear attention), chunked prefill, V1 engine architecture, tensor parallelism, and Ray 2.45.1 clustering.
- **Universal Tool Calling Protocol (UTCP v1.0.1)**: Pure domain protocol implementation in `internal/utcp/`. Provides native tool discovery and direct invocation over HTTP, gRPC, and CLI without proxy wrapper overhead. Includes bidirectional converters for Model Context Protocol (MCP) and OpenAPI schemas.
- **Multimodal foundation models**: Admitted `MultimodalInferenceService` tasks for Baidu Unlimited-OCR-2 (`task: ocr`, multi-page document parsing with SR-SWA) and Meta SAM 3 (`task: segmentation`, promptable concept segmentation masks).
- **LMCache v0.5.6 prefix caching**: Multiprocess shared memory key-value fabric over CUDA IPC and P2P transfers. Yields up to 85% time-to-first-token (TTFT) reduction for repeated system prompts.
- **Envoy Gateway 1.2.0 integration**: TLS termination, route fencing for quarantined workloads, and automated response security headers. HTTPRoute automatically injects `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, and `Referrer-Policy: no-referrer`, while TLS, HSTS, and frame options are terminated at the edge Gateway layer.
- **Normalized reconciler diff engine (`L-OP-018`)**: Reconciles container commands, working directories, ports, security contexts, probes, lifecycles, and pod scheduling constraints in `internal/controller/reconciler/diff.go` with API server default normalization to prevent reconciliation drift loops.
- **Consolidated Helm charts (`L-OP-013`)**: Canonical deployment chart in `deploy/helm`, with byte parity enforced against `charts/ckodex-kserve-llm-operator` through the `hack/helm-contract` verification suite.

## User roles and boundaries

| User | Inputs provided | Outputs received |
|---|---|---|
| Model scientist | Model URI, model name, accelerator resources, tool manuals | OpenAI-compatible endpoint, metrics, and readiness conditions |
| Platform engineer | Kubernetes cluster, Gateway API, storage secrets, node selectors | Governed control plane, automated reconciliation, and autoscaling |
| Security auditor | Identity policies, network boundaries, signing certificates | Cryptographic receipts, OSCAL assessments, and SLSA provenance |

## Architecture

The operator adheres to the Four Spaces architecture defined in the CKODEX Constitution:

```mermaid
flowchart TD
    subgraph Presentation["Presentation space"]
        CR["CRD Manifests: LLMInferenceService, AIPack, Agent"]
        Ctrl["Controllers: LLM, AIPack, Agent, Gateway"]
        WH["Admission and Conversion Webhooks"]
    end

    subgraph Validation["Validation space"]
        Val["Shared Admission Pipeline: Data, Model, Deployment, Request"]
        Pol["Policy Guards and Resource Invariants"]
    end

    subgraph Kernel["Semantic kernel"]
        SK["Pure Domain State Machine: 9-State Lifecycle"]
        SV["State Vector: Presence, Valence, Coherence, Evidence, Lifecycle, Epoch"]
        UTCP["UTCP v1.0.1 Dispatcher and Protocol Converter"]
    end

    subgraph Proof["Proof and evidence space"]
        Receipt["Execution Receipts and RFC 9162 Merkle Flight Recorder"]
        OSCAL["OSCAL Assessment Engine and Lula Validation"]
    end

    CR --> WH
    WH --> Val
    Ctrl --> Val
    Val --> SK
    SK --> SV
    Ctrl --> Kernel
    Kernel --> Proof
    Ctrl --> K8s["Kubernetes Resources: Deployment, Service, HTTPRoute, Gateway"]
```

## Quickstart

### Prerequisites

- Kubernetes cluster v1.30 or higher (Kind or cloud managed).
- `kubectl` v1.30+ and Helm v3.14+.
- NVIDIA GPU with CUDA 13.0 driver installed for GPU-accelerated workloads.

### Install the operator

Install the operator via the canonical Helm chart:

```bash
helm install ckodex-operator deploy/helm \
  --namespace ckodex-system \
  --create-namespace \
  --set image.repository=ghcr.io/runbase-ai/ckodex-kserve-llm-operator \
  --set image.tag=v0.20.0
```

Verify controller readiness:

```bash
kubectl rollout status deployment/ckodex-operator-controller-manager -n ckodex-system
```

### Deploy a specimen model

Deploy the Kimi-K3.1 specimen service using `serving.ckodex.com/v1`:

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: kimi-k31-specimen
  namespace: default
spec:
  model:
    name: tiny-random-kimi-k31-pro
    uri: hf://tiny-random/kimi-k3.1-pro
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
          args:
            - --trust-remote-code
            - --enforce-eager
            - --gpu-memory-utilization=0.50
            - --enable-auto-tool-choice
            - --tool-call-parser=hermes
            - --max-model-len=8192
          resources:
            limits:
              cpu: "4"
              memory: 8Gi
              nvidia.com/gpu: "1"
  router:
    gateway:
      managed:
        gatewayClassName: envoy
    route:
      httpRoute: {}
    scheduler:
      pool: {}
```

Apply the manifest and inspect status:

```bash
kubectl apply -f config/samples/llminferenceservice_kimi_k31_specimen.yaml
kubectl get llminferenceservices.serving.ckodex.com -w
```

## Primary custom resources

The operator exposes both stable (`v1`) and specialized (`v1alpha2`) custom resource definitions:

| Resource | Version | Purpose |
|---|---|---|
| `LLMInferenceService` | `serving.ckodex.com/v1` | Core LLM serving workload managing vLLM, SGLang, and TensorFold |
| `AIPack` | `serving.ckodex.com/v1alpha2` | Declarative packaging of models, LoRA adapters, and UTCP manuals |
| `MultimodalInferenceService` | `serving.ckodex.com/v1alpha2` | Foundation vision models (Baidu Unlimited-OCR-2, Meta SAM 3) |
| `EmbeddingInferenceService` | `serving.ckodex.com/v1alpha2` | Dense embedding serving workloads |
| `ASRInferenceService` | `serving.ckodex.com/v1alpha2` | Automatic speech recognition workloads |
| `Agent` | `serving.ckodex.com/v1alpha2` | Autonomous agent identity and UTCP tool manual bindings |
| `LocalModelCache` | `serving.ckodex.com/v1alpha2` | Cluster-wide and node-local weight pre-staging |
| `ModelServingProfile` | `serving.ckodex.com/v1alpha2` | Standardized hardware and execution presets |

## Verification and compliance

Every release artifact is signed and verified through automated Dagger pipelines:

```bash
dagger call all --source=.       # Hosted fast gate: vet, lint, and build check
dagger call test --source=.      # Unit and race tests with per-package coverage gates
dagger call scan --source=.      # Trivy container vulnerability scan
dagger call lula --source=. export --path=assessment-results.yaml  # OSCAL assessment
```

Run contract checks locally:

```bash
go test -count=1 ./hack/helm-contract/...
go test -count=1 ./hack/public-release-contract/...
```

## Documentation index

The full documentation suite is organized according to the [Diátaxis framework](https://diataxis.fr/) in the [documentation hub](docs/README.md).

### 1. Tutorials (learning-oriented)
- [Getting started with the operator](docs/tutorials/getting-started.md): End-to-end installation and first inference query in five minutes.

### 2. How-to guides (task-oriented)
- [Deploy a vLLM inference service](docs/how-to/deploy-vllm-service.md): Production vLLM v0.33.0 deployment with tensor parallelism.
- [Deploy multimodal foundation models with OCR and SAM 3](docs/how-to/deploy-multimodal-ocr-sam3.md): Baidu Unlimited-OCR-2 and Meta SAM 3.
- [Author and bind UTCP tools](docs/how-to/author-utcp-tools.md): Universal Tool Calling Protocol v1.0.1 manuals and agent bindings.
- [Configure LMCache prefix caching](docs/how-to/configure-lmcache-prefix-caching.md): Shared-memory prefix caching with up to 85% TTFT reduction.
- [Upgrade the operator to v0.20.0](docs/how-to/upgrade-to-v0.20.0.md): Zero-downtime rolling upgrade and CRD migration.
- [Troubleshoot inference workload failures](docs/how-to/troubleshoot-workload-failures.md): Decision tree for CUDA OOM, 503 errors, and startup probes.
- [Deploy the operator in an air-gapped environment](docs/how-to/airgap-deployment.md): Disconnected operation across Sovereignty Tiers 1 through 5.
- [Recover from cluster and node failures](docs/how-to/disaster-recovery.md): Complete disaster recovery and PVC weight re-population.

### 3. Reference (information-oriented)
- [Custom resource definitions reference](docs/reference/crds.md): Complete specifications for LLMInferenceService, AIPack, MultimodalInferenceService, and Agent.
- [Operator configuration defaults](docs/reference/operator-defaults.md): Environment variables, resources, flags, and diff normalization.
- [Inference engine runtime contract](docs/reference/engine-contract.md): Total capability matrix and conformance tiers.
- [Hardware accelerator profiles](docs/reference/accelerator-profiles.md): Profiles for NVIDIA, TPU v6e, Trainium 2, Gaudi 3, and Apple Silicon.
- [CLI and YAML cheat sheet](docs/reference/cheat-sheet.md): Fast copy-paste commands and minimal manifests.
- [API deprecation policy](docs/reference/api-deprecation-policy.md): Timeline, dual-serve window, and v1alpha2 to v1 migration rules.

### 4. Explanation (understanding-oriented)
- [Architecture and the Four Spaces](docs/explanation/overview.md): Pure kernel, shared validation, presentation, and proof.
- [Security architecture and zero-trust governance](docs/explanation/security-architecture.md): Capability leases, SPIRE workload identity, and route fencing.
- [Compliance as code with OSCAL and Lula](docs/explanation/compliance-oscal.md): Continuous NIST SP 800-53 Rev 5 control verification.
- [Component catalog and data flows](docs/explanation/components.md): Deep architectural catalog and sequence diagrams.

## Project status and governance

- Release version: `v0.20.0`
- Git commit: `9da7f18`
- Default operator container: `ghcr.io/runbase-ai/ckodex-kserve-llm-operator:v0.20.0`
- Governance standard: CKODEX Agent Engineering Constitution (Level 5 Assured Conformance)

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
