# Configure LMCache prefix caching

This guide explains how to configure LMCache v0.5.6 with the vLLM v0.33.0 runtime engine to cache key/value (KV) attention states across prompt requests.

## Overview

LMCache eliminates redundant prefill computation by caching KV tensors for repeated prompt prefixes in host CPU RAM, local NVMe storage, or node-local shared memory segments via CUDA IPC. This reduces time-to-first-token (TTFT) latency by up to 85% for multi-turn conversations and document analysis.

LMCache operates in two modes:

1. **In-process connector**: The `LMCacheConnectorV1` runs inside the vLLM container, utilizing local CPU memory and disk without cross-pod IPC.
2. **Multiprocess shared-memory fabric**: A node-level `LMCacheEngine` coordinates KV cache segments across multiple pods on the same physical GPU node using CUDA IPC and `/dev/shm`.

## Prerequisites

- Kubernetes cluster with NVIDIA GPU nodes (driver version 580.178.04 or higher).
- Operator v0.20.0 installed.
- Host system memory (CPU RAM) allocated to worker nodes (minimum 32 GiB per GPU).

## Step 1: Configure in-process prefix caching

For single-pod or independent replica deployments, enable in-process caching directly in the `LLMInferenceService` custom resource:

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: qwen7b-lmcache
  namespace: default
spec:
  model:
    name: qwen-7b-instruct
    uri: hf://Qwen/Qwen2.5-7B-Instruct
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
          args:
            - --trust-remote-code
            - --kv-transfer-config
            - '{"kv_connector":"LMCacheConnectorV1","kv_role":"kv_both","kv_connector_extra_config":{"chunk_size":256,"local_cpu":true,"max_local_cpu_size":20}}'
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

In this configuration:
- `chunk_size: 256` aligns with the standard vLLM v0.33.0 block size.
- `local_cpu: true` activates host CPU RAM offloading.
- `max_local_cpu_size: 20` reserves up to 20 GiB of host RAM for cached KV blocks.

Apply the manifest:

```bash
kubectl apply -f qwen7b-lmcache.yaml
```

## Step 2: Configure multiprocess shared-memory caching

Multiprocess mode shares KV cache blocks across different pods scheduled on the same node:

### 1. Label the namespace for privileged IPC admission

Node-local shared memory requires `hostIPC: true`:

```bash
kubectl label namespace inference pod-security.kubernetes.io/enforce=privileged
```

### 2. Deploy the LMCacheEngine daemon

Create `lmcache-engine.yaml`:

```yaml
apiVersion: lmcache.ai/v1alpha1
kind: LMCacheEngine
metadata:
  name: lmcache-node-engine
  namespace: inference
spec:
  image: lmcache/lmcache:v0.5.6
  sizeGB: 60
  port: 65432
  auth:
    enabled: false
```

Apply the engine manifest:

```bash
kubectl apply -f lmcache-engine.yaml
```

### 3. Deploy the inference service with multiprocess connector

Create `shared-service.yaml`:

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: shared-inference-service
  namespace: inference
spec:
  experimental:
    kvCache:
      transfer:
        connector: lmcache
        role: kv_both
        lmcache:
          mode: multiprocess
          engineRef:
            name: lmcache-node-engine
  model:
    name: qwen-7b
    uri: hf://Qwen/Qwen2.5-7B-Instruct
  template:
    spec:
      hostIPC: true
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
          volumeMounts:
            - mountPath: /dev/shm
              name: shm
          resources:
            requests:
              cpu: "8"
              memory: 32Gi
              nvidia.com/gpu: "1"
            limits:
              cpu: "8"
              memory: 32Gi
              nvidia.com/gpu: "1"
      volumes:
        - name: shm
          emptyDir:
            medium: Memory
            sizeLimit: 16Gi
```

Apply the service manifest:

```bash
kubectl apply -f shared-service.yaml
```

## Step 3: Verify cache hits and latency reduction

Send two consecutive inference requests with an identical long system prompt (for example, 4,000 tokens):

```bash
# Query 1: Cold cache (computes full prefill)
curl -w "TTFT: %{time_starttransfer}s | Total: %{time_total}s\n" -s -o /dev/null \
  -X POST http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen-7b-instruct","messages":[{"role":"system","content":"...4000 token prompt..."},{"role":"user","content":"Run step 1."}],"max_tokens":16}'

# Query 2: Warm cache (retrieves prefix KV tensors)
curl -w "TTFT: %{time_starttransfer}s | Total: %{time_total}s\n" -s -o /dev/null \
  -X POST http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen-7b-instruct","messages":[{"role":"system","content":"...4000 token prompt..."},{"role":"user","content":"Run step 2."}],"max_tokens":16}'
```

Expected result: Query 2 shows an 80% to 85% reduction in time-to-first-token (`time_starttransfer`).

Verify cache hits in the vLLM container logs:

```bash
kubectl logs deploy/qwen7b-lmcache -c vllm | grep "LMCache"
```

The output confirms:
```text
INFO: LMCache: prefix cache hit: 3840/4000 tokens retrieved from local CPU storage.
```

## Step 4: Common configuration adjustments

- **CUDA IPC permissions**: If pods fail with `cudaIpcOpenMemHandle failed`, verify that `hostIPC: true` is configured and that both containers run under matching Linux UIDs (for example, UID 65532).
- **Chunk size alignment**: Keep `chunk_size` set to `256` tokens. Using mismatched chunk sizes degrades hit ratios.
- **Node memory limits**: Keep `max_local_cpu_size` under 50% of the worker node total allocatable memory to prevent Linux out-of-memory kernel killing.

## Next steps

- [Troubleshoot workload failures](troubleshoot-workload-failures.md)
- [Consult runtime engine contracts](../reference/engine-contract.md)
- [Review operator workload defaults](../reference/operator-defaults.md)
