# LMCache operations runbook

This operational runbook details configuration, deployment, verification, and troubleshooting for LMCache v0.5.6 with the CKodex KServe LLM Operator.

LMCache caches language model attention key/value (KV) states across prompt completions. It operates as either an in-process connector or a multiprocess shared-memory service. It does not cache model weights (use `LocalModelCache` for weight pre-staging).

## Architectural modes

1. **In-process caching**: Runs the `LMCacheConnectorV1` inside the primary vLLM worker container. Uses host system memory (CPU RAM) and local NVMe storage as an offloaded tier-2 KV cache. Suitable for single-pod deployments requiring fast prefix re-use.
2. **Multiprocess shared-memory fabric**: Runs a separate `LMCacheEngine` pod on each GPU worker node. Exposes a shared-memory segment via CUDA IPC and POSIX shared memory (`/dev/shm`), backed by optional peer-to-peer (P2P) network synchronization across nodes. Yields up to 85% reduction in time-to-first-token (TTFT) for recurring prompt prefixes.

## Configuring in-process prefix caching

Enable in-process caching in the `LLMInferenceService` custom resource:

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: kimi-k31-cached
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

The operator validates connector parameters and passes the configuration directly to vLLM's KV transfer subsystem.

## Configuring multiprocess shared memory caching

Multiprocess mode shares KV cache blocks across multiple inference pods on the same physical node via CUDA IPC:

1. **Namespace security configuration**: Node-local shared memory requires `hostIPC: true`. Label the namespace for privileged pod security admission:
   ```bash
   kubectl label namespace inference pod-security.kubernetes.io/enforce=privileged
   ```
2. **Deploy LMCacheEngine daemon**:
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
3. **Reference the engine from LLMInferenceService**:
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
         containers:
           - name: vllm
             image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
   ```

## Verifying cache performance and hit rates

Execute repeated completion queries containing identical 4,000-token system prompt contexts:

```bash
# Query 1: Cold cache (prefill execution)
curl -w "TTFT: %{time_starttransfer}s | Total: %{time_total}s\n" -s -o /dev/null \
  -X POST http://kimi-k31-cached.default.svc.cluster.local:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"tiny-random-kimi-k31-pro","messages":[{"role":"system","content":"...long system prompt..."},{"role":"user","content":"Run query 1."}],"max_tokens":10}'

# Query 2: Warm cache (prefix reuse)
curl -w "TTFT: %{time_starttransfer}s | Total: %{time_total}s\n" -s -o /dev/null \
  -X POST http://kimi-k31-cached.default.svc.cluster.local:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"tiny-random-kimi-k31-pro","messages":[{"role":"system","content":"...long system prompt..."},{"role":"user","content":"Run query 2."}],"max_tokens":10}'
```

**Expected results**:
- Query 1 TTFT: ~1.2s to 2.5s (full prefill computation).
- Query 2 TTFT: ~0.15s to 0.35s (85%+ latency reduction via cached KV tensors).

Check vLLM engine logs for hit confirmation:
```bash
kubectl logs deploy/kimi-k31-cached -c vllm | grep "LMCache"
```
Output confirms:
```text
INFO: LMCache: prefix cache hit: 3840/4000 tokens retrieved from local CPU storage.
```

## Troubleshooting LMCache

### 1. CUDA IPC memory mapping error
**Symptom**: Pod crashes on startup with `RuntimeError: cudaIpcOpenMemHandle failed: out of memory` or `permission denied`.
**Cause**: The inference container and LMCache engine run under different Linux UIDs, or `hostIPC: true` is missing from the pod specification.
**Remediation**:
- Ensure both containers specify identical `securityContext.runAsUser` (for example, UID 65532).
- Verify `hostIPC: true` is set on the inference pod template.
- Check that `/dev/shm` is mounted with sufficient memory allocation (`emptyDir: { medium: "Memory", sizeLimit: 32Gi }`).

### 2. Cache reload parity mismatch
**Symptom**: Generated tokens diverge when cache is warm compared to a clean prefill.
**Cause**: Quantization or FP8/BF16 tensor layout mismatch between the vLLM engine and the LMCache serialization protocol.
**Remediation**:
- Ensure `--kv-cache-dtype` is set to `auto` or matches the explicit dtype declared in `kv_connector_extra_config`.
- Set `chunk_size` to `256` tokens (the standard alignment boundary for vLLM v0.33.0).

### 3. Eviction thrashing and CPU memory exhaustion
**Symptom**: High CPU memory utilization and worker slowdowns on multi-tenant nodes.
**Cause**: `max_local_cpu_size` exceeds node physical memory budget, causing OS swapping.
**Remediation**:
- Set `max_local_cpu_size` to at most 50% of available allocatable node memory.
- Configure pod memory limits to provide a 10 GiB buffer above the cache reservation.
