# Frontier Model Capacity Planning

This note is a static planning aid. It does not download models, start pods,
or benchmark throughput.

Use it to answer two separate questions:

1. Can the repo express the deployment plan?
2. Can the default local KIND envelope actually carry the model?

## Repo primitives already available

The repository already has the knobs needed to describe a large-model
deployment:

- `LLMInferenceService.spec.model.uri` supports `hf://`, `hf-mount://`,
  `oci://`, `ocis://`, `pvc://`, `s3://`, and `modelpack://`.
- `LLMInferenceService.spec.quantization` supports `awq`, `gptq`, `gguf`,
  `bitsandbytes`, and `fp8`.
- `LLMInferenceService.spec.kvCache` supports KV cache dtype and CPU swap-space
  planning.
- `LLMInferenceService.spec.parallelism` supports tensor, data, pipeline, and
  expert parallelism.
- `LocalModelCache` supports node-local model prewarm, `modelSize`, and
  `maxCacheSize`.
- `TenantQuotaReconciler` enforces GPU, CPU, memory, and service-count limits per
  labelled tenant namespace. It does not aggregate usage across multiple
  namespaces sharing one tenant ID.

That means the schema can represent frontier-model placement and sizing.

## Current local envelope

The repo-local KIND path is intentionally much smaller than the frontier-model
docs:

- `local/04-llm-inference-service.yaml` uses a single replica and an 8 GiB
  container memory limit.
- `DefaultTenantQuota()` caps each labelled tenant namespace at 5 LLM services,
  8 GPUs, 64 CPU, and 256 GiB memory.
- `LimitRange` sets a per-container maximum of 32 CPU and 128 GiB memory.
- `LocalModelCache.ModelSizeQuantity()` defaults to 20 GiB when `modelSize` is
  omitted.

Those defaults are enough for the repo's small local fixture, but not for the
two frontier models below.

## Frontier model fit matrix

| Model | Static weight footprint from the vendor doc | Context window from the vendor doc | Fit in default local KIND? | Why |
| --- | --- | --- | --- | --- |
| GLM-5.2 | 1.51 TiB full precision; about 239 GiB at 2-bit dynamic quantization; about 217 GiB at 1-bit dynamic quantization | 1M tokens | No | The quantized weight footprint alone exceeds the repo's 128 GiB per-container memory cap and far exceeds the sample local service memory limit. |
| Kimi K2.7 Code | 605 GiB full precision; about 325-350 GiB at dynamic 2-bit quantization | 98,304 to 262,144 tokens | No | The quantized weight footprint also exceeds the repo's 128 GiB per-container memory cap and the sample local service limit. |

## Capacity interpretation

- The operator can express a deployment plan for both models.
- The default local KIND environment cannot carry either model at the sizes
  described in the attached vendor docs.
- Exact users-per-replica capacity is not derivable from these static docs
  alone. For large models, concurrency depends on prompt length, KV cache
  budget, tensor parallelism, and the benchmarked serving stack.
- If you want to plan a deployment for either model, you must first raise the
  local storage and memory envelope, then pin an explicit quantization strategy
  and cache budget.

## Practical planning notes

- Use `LocalModelCache` or a mirrored `oci://`/`ocis://` artifact for the model
  weights.
- Set explicit `quantization` and `kvCache` values rather than relying on
  defaults.
- Increase `template.spec.resources` and `LocalModelCache.modelSize` to match
  the chosen artifact.
- For multi-node or multi-GPU planning, use `parallelism.tensor`,
  `parallelism.pipeline`, `parallelism.expert`, and `scaling` together.
- Treat the local KIND path as a repo-native verification loop for the
  operator, not as an environment sized for 200+ GiB model footprints.

## Companion helper

Run `./run/capacity-plan.sh` to print the static capacity assessment from the
repo root.
