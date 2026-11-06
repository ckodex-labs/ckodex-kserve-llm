# Deploy a vLLM inference service

This guide describes how to deploy a production-grade language model inference service using the vLLM v0.33.0 runtime engine with hardware acceleration.

## Overview

The operator provides native support for vLLM v0.33.0 (CUDA 13.0, PyTorch 2.14.0) through the `LLMInferenceService` custom resource (`serving.ckodex.com/v1`). The operator coordinates storage downloads, container argument rendering, GPU resource scheduling, health probes, and ingress routing.

## Prerequisites

Before deploying:

- Kubernetes cluster with NVIDIA GPU worker nodes and the NVIDIA GPU Operator installed.
- NVIDIA driver version 580.178.04 or higher supporting CUDA 13.0.
- A model repository URI (for example, `hf://Qwen/Qwen2.5-7B-Instruct` or private S3/OCI storage).
- A valid Hugging Face API token stored in a Kubernetes secret if using gated repositories.

To create the Hugging Face token secret:

```bash
kubectl create secret generic hf-token \
  --namespace default \
  --from-literal=token="hf_your_api_token"
```

## Step 1: Create the service manifest

Create a manifest named `vllm-qwen7b.yaml`. This configuration deploys Qwen 2.5 7B across one NVIDIA GPU with 24 GiB VRAM:

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: qwen25-7b
  namespace: default
  annotations:
    serving.ckodex.com/fast-restart: "true"
spec:
  model:
    name: qwen2.5-7b-instruct
    uri: hf://Qwen/Qwen2.5-7B-Instruct
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
          env:
            - name: HF_TOKEN
              valueFrom:
                secretKeyRef:
                  name: hf-token
                  key: token
                  optional: true
            - name: VLLM_GPU_MEMORY_UTILIZATION
              value: "0.90"
          resources:
            requests:
              cpu: "8"
              memory: 32Gi
              nvidia.com/gpu: "1"
            limits:
              cpu: "8"
              memory: 32Gi
              nvidia.com/gpu: "1"
```

## Step 2: Configure tensor parallelism for larger models

For models requiring multiple GPUs on a single node (such as 70B models), configure tensor parallelism by specifying container arguments and GPU request counts:

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: llama3-70b
  namespace: default
spec:
  model:
    name: llama-3.1-70b-instruct
    uri: hf://meta-llama/Llama-3.1-70B-Instruct
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
          args:
            - --tensor-parallel-size=4
            - --max-model-len=8192
            - --trust-remote-code
          resources:
            requests:
              cpu: "32"
              memory: 160Gi
              nvidia.com/gpu: "4"
            limits:
              cpu: "32"
              memory: 160Gi
              nvidia.com/gpu: "4"
```

## Step 3: Apply the manifest and monitor rollout

Apply the manifest:

```bash
kubectl apply -f vllm-qwen7b.yaml
```

Track custom resource status:

```bash
kubectl get llminferenceservice qwen25-7b -w
```

Track underlying pod initialization:

```bash
kubectl get pods -l app.kubernetes.io/name=qwen25-7b -w
```

Inspect weight download progress inside the storage-initializer container:

```bash
kubectl logs deploy/qwen25-7b -c storage-initializer -f
```

Inspect runtime startup inside the vLLM container:

```bash
kubectl logs deploy/qwen25-7b -c vllm -f
```

Startup completes when the log prints:
```text
INFO:     Application startup complete.
INFO:     Uvicorn running on http://0.0.0.0:8000
```

## Step 4: Test chat completions

Forward local port 8000:

```bash
kubectl port-forward svc/qwen25-7b 8000:8000 &
```

Send a test request:

```bash
curl -X POST http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen2.5-7b-instruct",
    "messages": [
      {"role": "system", "content": "You are a concise technical writer."},
      {"role": "user", "content": "Explain what tensor parallelism does."}
    ],
    "max_tokens": 128,
    "temperature": 0.2
  }' | jq .
```

## Step 5: Tune performance annotations

The operator supports performance annotations in `metadata.annotations`:

| Annotation | Parameter injected | Use case |
|---|---|---|
| `serving.ckodex.com/fast-restart: "true"` | `--load-format ipc_cache --enable-fast-restart` | Speeds up pod restarts by reusing shared memory weights |
| `serving.ckodex.com/enable-flashmla: "true"` | `--enable-flashmla` | Enables Flash Multi-Head Latent Attention kernels for MoE |
| `serving.ckodex.com/mega-gate: "true"` | `--enable-mega-gate` | Accelerates MoE token routing logic |

## Next steps

- [Configure LMCache prefix caching](configure-lmcache-prefix-caching.md)
- [Review runtime engine contracts](../reference/engine-contract.md)
- [Consult accelerator profiles](../reference/accelerator-profiles.md)
