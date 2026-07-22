# Gemma 4 Deployment Guide

This guide documents the operator defaults currently associated with the Gemma 4 model family. It is deployment guidance, not CI-backed benchmark evidence.

## Supported Model Guidance

| Model       | Resource Profile            | Hardware Tier           | Type    | Image                     |
| :---------- | :-------------------------- | :---------------------- | :------ | :------------------------ |
| **E2B**     | 8 CPU / 32Gi / 1 GPU        | Consumer (8GB VRAM)     | Dense   | `vllm/vllm-openai:v0.25.1` |
| **E4B**     | 16 CPU / 64Gi / 1 GPU       | Mid-range (16GB VRAM)   | Dense   | `vllm/vllm-openai:v0.25.1` |
| **26B-A4B** | 32 CPU / 128Gi / 1 GPU      | High-end (24GB VRAM)    | **MoE** | `vllm/vllm-openai:v0.25.1` |
| **31B**     | 32 CPU / 256Gi / **2 GPUs** | Enterprise (48GB+ VRAM) | Dense   | `vllm/vllm-openai:v0.25.1` |

## Prerequisites

1. **GPU Nodes**: Ensure your cluster has nodes with `nvidia.com/gpu` available.
2. **Operator Config**: Keep `vllm.defaultImage` at v0.25.1 or set an explicitly validated newer image.
3. **HuggingFace Secret**: For models like 31B, you may need an account and token to access the official Google repositories.

## Deployment Steps

### 1. Simple Deployment (E4B)

The E4B profile uses one GPU in the current WellKnown configuration:

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: gemma-4-e4b
spec:
  model:
    uri: hf://google/gemma-4-E4B-it
    name: gemma-4-e4b
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          resources:
            limits:
              cpu: "16"
              memory: 64Gi
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

The operator applies its current Gemma 4 Well-Known settings:

- Pass the mounted artifact explicitly with `--model /mnt/models`.
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
    uri: hf://google/gemma-4-31B-it
    name: gemma-4-31b
  parallelism:
    tensor: 2
  template:
    spec:
      containers:
        - name: vllm
          resources:
            limits:
              cpu: "32"
              memory: 256Gi
              nvidia.com/gpu: "2"
  router:
    gateway:
      managed:
        gatewayClassName: envoy
    route:
      httpRoute: {}
    scheduler:
      pool: {}
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

- **NVFP4**: Use vLLM v0.25.1 or newer for the mixed-dtype Gemma/Qwen NVFP4 correctness fix. Kernel selection is a runtime observation and must be confirmed from the serving pod logs.
- **CUDA compatibility mode**: The operator does not force `VLLM_ENABLE_CUDA_COMPATIBILITY`. vLLM documents that mode for drivers older than the image's CUDA toolkit, and its compatibility libraries can produce CUDA error 803 on unsupported RTX systems. Enable it explicitly only after validating that host/image combination.
- **Persistent kernel tuning cache**: FlashInfer autotuning can generate dozens of kernel configurations during the first startup. Keep those artifacts across pod replacements with a dedicated writable PVC; do not persist all of `/tmp` and do not write into a read-only model-volume mount:

  ```yaml
  spec:
    template:
      spec:
        containers:
          - name: vllm
            env:
              - name: VLLM_FLASHINFER_AUTOTUNE_CACHE_DIR
                value: /var/cache/vllm/flashinfer
              - name: TORCHINDUCTOR_CACHE_DIR
                value: /var/cache/vllm/torchinductor
            volumeMounts:
              - name: kernel-cache
                mountPath: /var/cache/vllm
        volumes:
          - name: kernel-cache
            persistentVolumeClaim:
              claimName: gemma-kernel-cache
  ```

  The operator preserves these pod-template values. The claim must support the access mode required by the chosen replica and rollout strategy.
- **CPU KV Offloading**: For the 31B model, if you have limited GPU VRAM but plenty of system RAM, you can manually enable CPU offloading:

  ```yaml
  spec:
    template:
      spec:
        containers:
          - name: vllm
            args: ["--cpu-offload-gb", "16"]
  ```

If you're using custom mirrors, mirror the exact `vllm.defaultImage` configured in Helm. A container image supplied directly in `spec.template.spec.containers[0].image` takes precedence over the operator default.
