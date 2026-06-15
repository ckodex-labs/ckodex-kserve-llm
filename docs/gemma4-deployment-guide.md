# Gemma 4 Deployment Guide

This guide documents the operator defaults currently associated with the Gemma 4 model family. It is deployment guidance, not CI-backed benchmark evidence.

## Supported Model Guidance

| Model       | Resource Profile            | Hardware Tier           | Type    | Image                     |
| :---------- | :-------------------------- | :---------------------- | :------ | :------------------------ |
| **E2B**     | 8 CPU / 32Gi / 1 GPU        | Consumer (8GB VRAM)     | Dense   | `vllm/vllm-openai:gemma4` |
| **E4B**     | 16 CPU / 64Gi / 1 GPU       | Mid-range (16GB VRAM)   | Dense   | `vllm/vllm-openai:gemma4` |
| **26B-A4B** | 32 CPU / 128Gi / 1 GPU      | High-end (24GB VRAM)    | **MoE** | `vllm/vllm-openai:gemma4` |
| **31B**     | 32 CPU / 256Gi / **2 GPUs** | Enterprise (48GB+ VRAM) | Dense   | `vllm/vllm-openai:gemma4` |

## Prerequisites

1. **GPU Nodes**: Ensure your cluster has nodes with `nvidia.com/gpu` available.
2. **Operator Config**: Set `vllm.gemma4Image: "vllm/vllm-openai:gemma4"` in your Helm values.
3. **HuggingFace Secret**: For models like 31B, you may need an account and token to access the official Google repositories.

## Deployment Steps

### 1. Simple Deployment (E4B)

Gemma 4 E4B is the best all-rounder. Use the following manifest:

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: gemma-4-e4b
spec:
  model:
    uri: "hf://google/gemma-4-E4B-it"
    name: "gemma-4-e4b"
  replicas: 1
```

The operator applies its current Gemma 4 Well-Known settings:

- Enforce TurboQuant args (`--enable-turboquant`).
- Guaranteed QoS (Requests == Limits).
- Optimized vLLM image.

### 2. Multi-GPU Deployment (31B)

For the dense 31B model, the operator defaults to **Tensor Parallelism (TP=2)** because it requires more VRAM than typical single-GPU nodes:

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: gemma-4-31b
spec:
  model:
    uri: "hf://google/gemma-4-31B-it"
  parallelism:
    tensor: 2
```

### 3. Mixture-of-Experts (26B-A4B)

The 26B-A4B is a **MoE** model. While it has 27B total parameters, only **4B** are active during any single inference step. The operator enables **Expert Parallelism** automatically.

## Monitoring Capacity

If you deploy a model that exceeds your cluster's GPU count, the operator will emit an informational event:

```bash
kubectl get events | grep InsufficientGPUCapacity
```

**Status Condition**: Check the `GPUCapacity` condition on your `LLMInferenceService`:

```bash
kubectl get llmisvc gemma-4-31b -o jsonpath='{.status.conditions[?(@.type=="GPUCapacity")]}'
```

## Optimization Tips

- **TurboQuant**: All Gemma 4 models are pre-configured with `--enable-turboquant`. This significantly reduces VRAM footprint and improves throughput.
- **CPU KV Offloading**: For the 31B model, if you have limited GPU VRAM but plenty of system RAM, you can manually enable CPU offloading:

  ```yaml
  spec:
    vllmArgs: ["--cpu-offload-gb", "16"]
  ```

If you're using custom mirrors, ensure your registry has the `vllm/vllm-openai:gemma4` image expected by your operator configuration.
