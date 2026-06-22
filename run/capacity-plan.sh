#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

cat <<'EOF'
==> CKodex frontier model capacity plan

This is a static planning report. It does not download models or start pods.

Current repo envelope
- Sample local service memory limit: 8 GiB
- Tenant quota: 5 LLM services, 8 GPUs, 64 CPU, 256 GiB memory
- Per-container LimitRange max: 32 CPU, 128 GiB memory
- LocalModelCache default size: 20 GiB

Repo primitives available for larger models
- model URI schemes: hf://, hf-mount://, oci://, ocis://, pvc://, s3://,
  modelpack://
- quantization: awq, gptq, gguf, bitsandbytes, fp8
- KV cache: dtype + CPU swap-space
- parallelism: tensor, data, pipeline, expert
- scaling: min/max replicas, KEDA, HPA, WVA
- cache prewarm: LocalModelCache warmNodes + maxCacheSize

GLM-5.2
- Total parameters: 744B
- Active parameters: 40B
- Context: 1M tokens
- Weight footprint: 1.51 TiB full precision; about 239 GiB at 2-bit dynamic
  quantization; about 217 GiB at 1-bit dynamic quantization
- Verdict: schema-supported, but not a fit for the default local KIND envelope
- Constraint hit: quantized weights alone exceed the 128 GiB container memory
  cap

Kimi K2.7 Code
- Total parameters: 1T
- Active parameters: 32B
- Context: 98,304 to 262,144 tokens
- Weight footprint: 605 GiB full precision; about 325-350 GiB at dynamic 2-bit
  quantization
- Verdict: schema-supported, but not a fit for the default local KIND envelope
- Constraint hit: quantized weights alone exceed the 128 GiB container memory
  cap

Users / concurrency
- No trustworthy per-user capacity number can be derived from the static docs
  alone.
- For frontier models, concurrency is dominated by KV cache budget, prompt
  length, and the serving benchmark rather than by model name alone.

Next planning step
- Raise the storage and memory envelope first, then pin quantization, KV cache,
  and parallelism settings before attempting a live deployment.
EOF
