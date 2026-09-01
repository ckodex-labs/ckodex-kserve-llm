# Evidence: GLM-5.3-Flash Tiny Local CPU Smoke

Date: 2026-08-31

## Subject

Pinned fixture:

```text
model:    inference-optimization/GLM-5.3-Flash-0.1B-A0.1B
revision: 8311399447eba9c9b215e3209ab6f25e59c7d21e
format:   BF16, unquantized
```

The fixture is an architecture and tooling test artifact. It is not a
quality-preserving miniature of GLM-5.3-Flash.

## Environment

```text
host:         Darwin arm64
python:       3.11.7
torch:        2.13.0
transformers: 5.16.0.dev0
source:       Hugging Face Transformers commit 4da05482135896a529d5536c3c003102d36528a2
safetensors:  0.8.0
cuda:         unavailable
```

## Checks

The repository preflight was run with the isolated Python environment:

```bash
PYTHON_BIN=/tmp/ckodex-glm5-next-run/bin/python \
  bash run/glm5-next-tiny-preflight.sh
```

Observed result:

```text
model_type=glm5_next
config_class=Glm5NextConfig
revision=8311399447eba9c9b215e3209ab6f25e59c7d21e
weights_loaded=false
glm5_next preflight passed
```

The pinned checkpoint was then loaded and exercised on CPU with a bounded
20-token generation request.

```text
input_tokens=7
output_tokens=27
generated_tokens=20
continuation=', there is no way a bee should be able to fly. Its wings are too small to get'
```

## Classification

- **C:** the pinned tiny checkpoint's `glm5_next` configuration, tokenizer, and
  CPU forward/generation path work in the isolated local environment.
- **S:** operator reconciliation, vLLM/SGLang server startup, and Kubernetes
  route acceptance remain unverified for this architecture.
- **A:** full GLM-5.3-Flash quality, NVFP4 execution, GPU performance, long
  context, and distributed inference remain outside this evidence.

## Integrity references

```text
run/glm5-next-tiny-preflight.sh
sha256:bcbf04ce849cc1d6db2444c587dd3a4951887239ccbe12c4177fe11b8a068a4c

config/samples/llminferenceservice_glm5_next_tiny.yaml
sha256:3a8c3149229c6379e3ed02d78b6a9f32048b133a3f810ec8a2a4b4e15054b402
```
