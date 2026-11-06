# Custom resource definitions reference

This document provides technical specifications for custom resources managed by the CKodex KServe LLM Operator under the `serving.ckodex.com` API group.

## 1. LLMInferenceService

The primary workload resource for deploying large language model inference engines.

- **Group**: `serving.ckodex.com`
- **Version**: `v1` (storage version), `v1alpha2` (supported deprecated version)
- **Kind**: `LLMInferenceService`
- **Scope**: Namespaced

### Specification (`spec`)

| Field | Type | Description |
|---|---|---|
| `model.name` | `string` | Canonical model name exposed in the OpenAI `/v1/models` and `/v1/chat/completions` API. Required. |
| `model.uri` | `string` | Storage URI for weights (`hf://<repo>`, `s3://<bucket>/<path>`, `oci://<registry>/<image>:<tag>`). Required. |
| `replicas` | `int32` | Desired pod replica count. Defaults to `1`. |
| `template` | `PodTemplateSpec` | Pod template configuring container image, commands, arguments, resource requests, limits, and volumes. |
| `scaling` | `ScalingSpec` | Autoscaling configuration (KEDA, metric triggers, min/max replicas, scale-to-zero). |
| `routing` | `RoutingSpec` | Gateway API HTTPRoute configuration, hostnames, path prefixes, timeouts, and route fencing parameters. |
| `experimental.kvCache` | `KVCacheSpec` | Advanced KV cache transfer and LMCache connector specifications. |

### Status (`status`)

| Field | Type | Description |
|---|---|---|
| `state` | `LifecycleState` | Current state machine position (`QUARANTINED`, `ADMITTED`, `PROVISIONING`, `WARMING`, `READY`, `DEGRADED`, `SAFE_HOLD`, `RECONCILING`, `RETIRED`). |
| `conditions` | `[]Condition` | Standard Kubernetes status conditions (`Ready`, `Admitted`, `WarmingComplete`, `RoutingConfigured`, `Compliance-SI-7`). |
| `url` | `string` | Resolved cluster-internal or external Gateway URL for inference queries. |
| `modelRevision` | `string` | Resolved immutable commit hash or digest of the deployed model artifact. |
| `replicas` | `int32` | Total number of running pod replicas. |
| `readyReplicas` | `int32` | Number of pods passing readiness probes. |

---

## 2. MultimodalInferenceService

Deploys multimodal foundation models specialized for document OCR or concept segmentation.

- **Group**: `serving.ckodex.com`
- **Version**: `v1alpha2`
- **Kind**: `MultimodalInferenceService`
- **Scope**: Namespaced

### Specification (`spec`)

| Field | Type | Description |
|---|---|---|
| `task` | `string` | Multimodal task identifier. Permitted values: `ocr`, `segmentation`. Required. |
| `runtime` | `string` | Runtime engine adapter. Defaults to `vllm`. |
| `model.name` | `string` | Identifier matching the model (for example, `baidu-unlimited-ocr-2`, `meta-sam3`). Required. |
| `model.uri` | `string` | Weight repository URI. Required. |
| `maxImagesPerPrompt` | `int32` | Maximum concurrent image tensors allowed in a single request. Defaults to `16`. |
| `replicas` | `int32` | Workload replica count. |
| `template` | `PodTemplateSpec` | Container resources, limits, and GPU allocations. |

---

## 3. AIPack

Packaging specification for AI artifacts conforming to AIPACK-SPEC v0.1.1. Packages models, LoRA adapters, skills, and UTCP manuals into OCI artifacts.

- **Group**: `serving.ckodex.com`
- **Version**: `v1alpha2`
- **Kind**: `AIPack`
- **Scope**: Namespaced

### Artifact kinds (16 total: 15 atomic + 1 composite)

| Kind code | Kind name | Family | Description |
|---|---|---|---|
| **A1** | `BaseModel` | `model` | Full model weights (safetensors, GGUF, AWQ). |
| **A2** | `LoRA` | `model` | Low-Rank Adaptation delta weights. |
| **A3** | `FineTune` | `model` | Checkpoint delta from supervised fine-tuning. |
| **A4** | `Skill` | `capability` | Reusable procedural capability. |
| **A5** | `Tool` | `capability` | Standalone executable tool. |
| **A6** | `MCPServer` | `capability` | Model Context Protocol server bundle. |
| **A7** | `PromptTemplate` | `control` | Structured system or user prompt template. |
| **A8** | `Guardrail` | `control` | Content safety filter or boundary check. |
| **A9** | `RetrievalIndex` | `knowledge` | Vector index or embedding store. |
| **A10** | `Dataset` | `knowledge` | Reference evaluation or fine-tuning dataset. |
| **A11** | `Harness` | `assurance` | Benchmark test harness specification. |
| **A12** | `Eval` | `assurance` | Evaluation metric suite and scoring rule. |
| **A13** | `Workflow` | `capability` | Multi-step agentic orchestration flow. |
| **A14** | `PolicyBundle` | `control` | Rego/Cedar authorization policy ruleset. |
| **A15** | `UTCPManual` | `capability` | Universal Tool Calling Protocol v1.0.1 manual. |
| **C1** | `Agent` | `composite` | Composed agent bundling base model, adapters, skills, and tools. |

### Specification (`spec`)

| Field | Type | Description |
|---|---|---|
| `kind` | `ArtifactKind` | One of the 16 kind values listed above. Required. |
| `family` | `ArtifactFamily` | Semantic family (`model`, `capability`, `control`, `knowledge`, `assurance`, `composite`). |
| `source.ref` | `string` | Fully qualified OCI reference with mandatory sha256 digest (`<repo>@sha256:<hex>`). Tag-only refs rejected. |
| `source.mediaType` | `string` | AIPACK manifest media type. |
| `utcpManual` | `UTCPManualSpec` | Endpoint, schema, and version metadata for `kind: UTCPManual`. |
| `composition` | `AIPackComposition` | Component slot references for composite `kind: Agent`. |

---

## 4. Agent

Declares autonomous agent identity, model bindings, and UTCP tool manual associations.

- **Group**: `serving.ckodex.com`
- **Version**: `v1alpha2` (schema in `v1`)
- **Kind**: `Agent`
- **Scope**: Namespaced

### Specification (`spec`)

| Field | Type | Description |
|---|---|---|
| `identity.name` | `string` | Human-readable agent display name. Required. |
| `identity.version` | `string` | Semantic version string. Required. |
| `modelRef` | `string` | Name of an `LLMInferenceService` providing reasoning capabilities. Required. |
| `utcpManuals` | `[]UTCPManualRef` | Array of tool manuals bound to this agent (via HTTP URL or `AIPack` reference). |
| `skills` | `[]SkillRef` | Array of skill references registered in a `SkillRegistry`. |
| `maxTokens` | `int32` | Maximum generation tokens permitted per agent turn. |

---

## 5. Supporting custom resources

### LocalModelCache (`v1alpha2`)
Pre-stages model weights to node-local NVMe storage:
- `spec.modelName`: Name of the model.
- `spec.source.uri`: Source OCI or S3 repository.
- `spec.nodeSelector`: Target node selection labels.
- `spec.storage.path`: Host path mount location (for example, `/var/lib/ckodex/models/<name>`).
- `spec.retentionPolicy`: `Retain` or `Delete`.

### LLMLoraAdapter (`v1`)
Attaches dynamic LoRA adapter weights to running inference services:
- `spec.inferenceServiceRef`: Name of the base `LLMInferenceService`.
- `spec.source.uri`: Storage URI for adapter weights.
- `spec.scalingFactor`: LoRA scaling factor ($alpha / r$).

### ModelServingProfile (`v1alpha2`)
Defines cluster-wide hardware and runtime configuration presets:
- `spec.accelerator`: Hardware vendor and model constraints.
- `spec.engine`: Required runtime engine and conformance tier.
- `spec.defaultArguments`: Mandatory container argument injections.

## Status condition types

| Condition type | Status values | Meaning |
|---|---|---|
| `Ready` | `True` / `False` | Overall workload operational status. |
| `Admitted` | `True` / `False` | Workload passed all four admission stages. |
| `WarmingComplete` | `True` / `False` | Model weights pre-staged and KV memory allocated. |
| `RoutingConfigured` | `True` / `False` | Gateway API HTTPRoute synchronized without fencing. |
| `Compliance-SI-7` | `True` / `False` | Cryptographic signature, SLSA provenance, and SBOM verified. |
