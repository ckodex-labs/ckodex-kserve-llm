# Operator workload defaults

The CKodex KServe LLM Operator reads default runtime and resource configurations from environment variables at startup. Helm values in both `deploy/helm` and `charts/ckodex-kserve-llm-operator` configure these variables.

Modifying operator defaults requires rolling the operator Deployment. Explicit per-service settings in custom resource manifests always supersede operator defaults.

## Environment variables and default pins

| Environment variable | Default value in v0.20.0 | Description |
|---|---|---|
| `CKODEX_RUNTIME_IMAGE` | `vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e` | Pinned default vLLM container image (PyTorch 2.14.0, CUDA 13.0) |
| `CKODEX_CUSTOM_STORAGE_INITIALIZER_IMAGE` | `kserve/storage-initializer:v0.21.0@sha256:3a3a6a8053478c8d8d90f96fc5e54c1892c07784102dda70104374d4f89a3672` | Init container image for S3, GCS, and OCI model downloads |
| `CKODEX_HUGGING_FACE_INITIALIZER_IMAGE` | `ghcr.io/ckodex-labs/ckodex-kserve-llm-huggingface-initializer:v0.20.0` | Initializer image for `hf://` model downloads with signature verification |
| `CKODEX_VLLM_CPU_REQUEST` | `2` | Default CPU core request for vLLM and reranker workloads |
| `CKODEX_VLLM_MEMORY_REQUEST` | `4Gi` | Default memory request for vLLM workloads |
| `CKODEX_TERMINATION_GRACE_PERIOD_SECONDS` | `60` | Default termination grace period for graceful inference stream draining |
| `CKODEX_ASR_CPU_REQUEST` | `1` | Default CPU core request for ASR inference services |
| `CKODEX_ASR_MEMORY_REQUEST` | `2Gi` | Default memory request for ASR inference services |
| `CKODEX_ASR_TERMINATION_GRACE_PERIOD_SECONDS` | `30` | Default grace period for ASR and cache synchronization Jobs |
| `CKODEX_CACHE_CPU_REQUEST` | `1` | CPU request for LocalModelCache warm-up Jobs |
| `CKODEX_CACHE_MEMORY_REQUEST` | `1Gi` | Memory request for LocalModelCache warm-up Jobs |

## Stack compatibility and baseline versions

| Subsystem | Supported baseline | Notes |
|---|---|---|
| **Kubernetes** | `1.30` through `1.32+` | Tested on Kind `v1.35.8` / Kubernetes 1.32 |
| **Inference engine** | vLLM `v0.33.0` | CUDA 13.0 only (CUDA 12.9 dropped) |
| **Clustering runtime** | Ray `2.45.1` | Used for multinode tensor and pipeline parallel execution |
| **Ingress gateway** | Envoy Gateway `1.2.0` | Supports HTTPRoute and GRPCRoute with route fencing |
| **Gateway API** | `v1.6.2` | Core Gateway API standard conformance |
| **Prefix cache** | LMCache `v0.5.6` | In-process connector and multiprocess shared memory KV fabric |
| **Workload identity** | SPIRE `v1.15.4` | Node-local SPIFFE Workload API and automated SVID issuance |

## Reconciler field diff normalization (`L-OP-018`)

The reconciler diff engine in `internal/controller/reconciler/diff.go` normalizes Kubernetes API server defaults to avoid spurious reconciliation updates:
- **Container image pull policy**: Automatically defaults omitted pull policies according to API server conventions (`PullAlways` for `:latest` tags, `PullIfNotPresent` for digest-pinned images).
- **Environment variables**: Evaluates environment variable slices as order-insensitive sets (`EnvsEqual`).
- **Container commands and arguments**: Normalizes command slices and validates long-option parameter syntax.
- **Security contexts and probes**: Compares container and pod security contexts and probes using semantic equality rather than raw pointer comparisons.
- **Pod scheduling**: Reconciles `nodeSelector`, `tolerations`, `affinity`, and `topologySpreadConstraints` cleanly across updates.
