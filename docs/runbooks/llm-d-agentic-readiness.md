# Enabling llm-d Agentic-Workload Profile (Disaggregated + KV-cache + Prefill scaling)

This runbook defines the end-to-end path to run the repo in the agentic/long-context profile described in the llm-d article.

## Objective

Enable inference for long-context, bursty, cache-sensitive workloads using:

- `spec.prefill` + `spec.kvCache.transfer` (disaggregated prefill/decode)
- vLLM speculative decoding (`mtp`, `eagle`, `medusa`, `ngram`)
- multi-GPU distribution (`parallelism.tensor`, `data`, `pipeline`, `expert`, `eplbEnabled`)
- optional KV sharing backend (`lmcache`/`nixl`/`mooncake`)
- optional KV-aware routing (`router.scheduler`)

## Capability prerequisites in CRD and admission

| Capability | Required fields | Admission behavior |
|---|---|---|
| Disaggregated prefill | `spec.prefill` + `spec.kvCache.transfer.connector` | `spec.prefill` requires `kvCache.transfer.connector`; each prefill workload needs at least one container in `spec.prefill.template`. |
| KV transfer | `spec.kvCache.transfer.connector: nixl|lmcache|mooncake` | Connector-specific settings are passed through `--kv-transfer-config`; unsupported values are rejected by webhook enums. |
| Speculative decoding | `spec.speculativeDecoding` (`method`, optional `numTokens`, optional `draftModel`) | vLLM adapter maps `method/numTokens/draftModel` to `--spec-*` flags. |
| GPU scaling | `spec.parallelism.*` | Validator enforces power-of-two tensor, GPU-device cardinality matching tensor when set, and GPU QoS expectations. |
| Scheduler path | `spec.router.scheduler` + `spec.router.gateway/route` | Reconciliation needs EPP identity and CRDs; scheduler readiness is fail-closed and reflected in status. |

## Minimal sample (v1alpha2)

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LLMInferenceService
metadata:
  name: glm5-agentic-profile
  namespace: ckodex-inference
spec:
  model:
    uri: hf://unsloth/Qwen-QwQ-32B
    name: unsloth/Qwen-QwQ-32B
  template:
    spec:
      containers:
        - name: vllm
          image: ghcr.io/your-org/llm-vllm-runtime@sha256:...
          resources:
            limits:
              cpu: "24"
              memory: 64Gi
              nvidia.com/gpu: "4"
  replicas: 1
  parallelism:
    tensor: 2
    pipeline: 1
  prefill:
    replicas: 2
    template:
      spec:
        containers:
          - name: vllm-prefill
            image: ghcr.io/your-org/llm-vllm-runtime@sha256:...
            resources:
              limits:
                nvidia.com/gpu: "1"
  kvCache:
    dtype: fp8
    swapSpaceGB: 32
    transfer:
      connector: lmcache
      role: kv_both
      lmcache:
        mode: inProcess
      extraConfig:
        chunk_size: "256"
        remote_url: "redis://lmcache.cache.svc:6379"
  speculativeDecoding:
    method: mtp
    numTokens: 4
  router:
    gateway:
      managed:
      gatewayClassName: envoy
    route:
      httpRoute:
        hostnames:
          - glm5-agentic.example.com
    scheduler:
      pool:
        selector:
          serving.ckodex.com/role: llminferenceservice-workload
```

Apply the executable sample directly:

```bash
kubectl apply -f config/samples/llminferenceservice_agentic_profile_v1alpha2.yaml
```

Treat fields containing `your-org` or other placeholders as environment-specific
and replace them before production use.

## Preflight checks (operator-owned)

1. Validate rendered manifests include:
   - `-prefill` deployment exists with `serving.ckodex.com/role: prefill`.
   - both primary and prefill container args include `--kv-transfer-config`.
   - scheduler creates `EndpointPickerConfig`, `InferencePool`, EPP Deployment, and EPP Service.
2. Verify admission:
   - `kubectl apply` rejects missing `kvCache.transfer.connector` when `prefill` is set.
   - `lmcache` typed mode validates `engineRef` only in multiprocess mode.
3. Confirm status:
   - `LLMInferenceService.Ready` must remain false until decode and scheduler state are coherent.
   - prefill readiness uses `Prefill*` condition, and does not conflate with model readiness.

## Live acceptance gates for true llm-d-style enablement

Treat these as separate test phases, not local green signals:

1. **Cache-functional**: sustained read-heavy prompts, repeat-prefix traces, measurable KV hit-rate.
2. **Transfer-functional**: confirm successful `--kv-transfer-config` negotiation and actual transfer activity under load.
3. **Burst-functional**: sudden concurrency spikes from multi-agent style load without queue collapse.
4. **Failure-functional**: backend node failure/restart, scheduler pod recycle, and cold-to-warm transition under load.
5. **Scheduler-functional**: `EndpointPickerConfig`/InferencePool readiness plus endpoint-level routing traces.
6. **Safety-functional**: no hidden fallback to unsupported engine behavior when a capability is refused (status must explain the deny path).

## Fast preflight smokebook (same namespace, explicit)

Use this sequence before moving to long-duration acceptance:

```bash
kubectl apply -f config/samples/llminferenceservice_agentic_profile_v1alpha2.yaml
kubectl get llminferenceservice llm-d-agentic-profile -n ckodex-inference -o wide
kubectl wait --for=condition=Ready --timeout=900s llminferenceservice/llm-d-agentic-profile -n ckodex-inference
kubectl get deployment -n ckodex-inference llm-d-agentic-profile llm-d-agentic-profile-prefill
```

```bash
kubectl get pods -n ckodex-inference -l serving.ckodex.com/workload=llm-d-agentic-profile
kubectl logs -n ckodex-inference deploy/llm-d-agentic-profile -c vllm-decode --tail=80
kubectl logs -n ckodex-inference deploy/llm-d-agentic-profile-prefill -c vllm-prefill --tail=80
kubectl get httproute -n ckodex-inference | rg -n "llm-d-agentic-profile"
kubectl get inferencepool.inference.networking.k8s.io -n ckodex-inference | rg -n "llm-d-agentic-profile"
```

```bash
# Status gates that prove fail-closed semantics are visible in control plane state
kubectl get llminferenceservice llm-d-agentic-profile -n ckodex-inference -o jsonpath='{.status.conditions}' | jq '.'
kubectl get llminferenceservice llm-d-agentic-profile -n ckodex-inference -o jsonpath='{.status.conditions[?(@.type=="Ready" || @.type=="DeploymentReady" || @.type=="GatewayReady" || @.type=="SchedulerReady" || @.type=="PrefillReady")]}' | jq '.'
kubectl get configmap -n ckodex-inference llm-d-agentic-profile-scheduler-config -o yaml
kubectl get deployment -n ckodex-inference llm-d-agentic-profile-epp
kubectl get service -n ckodex-inference llm-d-agentic-profile-epp
```

Rollback-safe cleanup:

```bash
kubectl delete llminferenceservice llm-d-agentic-profile -n ckodex-inference
```

Run this only in disposable infrastructure and with change-control for non-local environments.

## Evidence to keep for go-live

- `internal/scheduler/epp_manager.go` and `internal/controller/deployment/kv_transfer.go` reconciliation evidence (from live resource audit).
- Nightly/fresh-cluster evidence:
  - operator logs showing scheduler blocked/ready transitions.
  - benchmark traces showing latency and tail-risk under bursty agentic load.
  - cache-hit and transfer-tail logs/metrics.
- Signed and reproducible run record in `docs/evidence/`.

## Non-go-live caveats

- The current runtime contract still requires explicit live proof for throughput, hit-rate, and failover in your target hardware profile.
- `LocalModelCache` remains model-weight cache ownership; it is not equivalent to KV transfer cache.
- `LMCACHE_USE_EXPERIMENTAL` handling follows documented vLLM guidance and is required for in-process connector compatibility.
