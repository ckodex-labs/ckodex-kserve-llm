# Runbook: LoRA Adapter Hot-Swap

**Audience:** ML engineers, platform engineers
**Applies to:** `LLMLoraAdapter` CRD, vLLM dynamic adapter loading
**Related alerts:** None (hot-swap is fast; alert if base service becomes unavailable)

---

## Overview

LoRA (Low-Rank Adaptation) adapters extend a base model's behavior without reloading the
full model weights. The operator manages `LLMLoraAdapter` resources and triggers hot-swaps
via the vLLM `/v1/load_lora_adapter` API endpoint — no pod restart required.

Hot-swap latency is typically 200–800 ms depending on adapter size and GPU memory bandwidth.

---

## Apply a LoRA Adapter

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMLoraAdapter
metadata:
  name: customer-support-v3
  namespace: ml-team-a
spec:
  baseServiceRef:
    name: llama-3-8b
  adapterURI: hf://acme/llama3-customer-support-v3
  adapterName: customer-support-v3
```

```bash
kubectl apply -f customer-support-v3.yaml
```

The operator reconciler detects the new `LLMLoraAdapter`, downloads the adapter weights,
and calls the vLLM load API on each pod replica.

---

## Verify the Adapter is Loaded

```bash
# Check adapter status
kubectl get llmloraadapter customer-support-v3 -n ml-team-a

# Verify adapter is registered on vLLM
kubectl exec -n ml-team-a deploy/llama-3-8b -- \
  curl -s http://localhost:8000/v1/models | jq '.data[].id'
```

Expected output includes `customer-support-v3`.

---

## Test the Adapter

```bash
ENDPOINT=$(kubectl get llminferenceservice llama-3-8b -n ml-team-a -o jsonpath='{.status.url}')

curl -s "${ENDPOINT}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "customer-support-v3",
    "messages": [{"role": "user", "content": "How do I reset my password?"}],
    "max_tokens": 100
  }'
```

---

## Swap to a New Adapter Version

```bash
# 1. Apply the new adapter version
kubectl apply -f customer-support-v4.yaml

# 2. Wait for reconcile
kubectl wait --for=condition=Ready llmloraadapter/customer-support-v4 -n ml-team-a --timeout=120s

# 3. Update inference clients to use the new adapter name
# (or rename the adapter by patching spec.adapterName)

# 4. Remove the old adapter (unloads from vLLM automatically)
kubectl delete llmloraadapter customer-support-v3 -n ml-team-a
```

---

## Rollback

```bash
# Re-apply the previous version
kubectl apply -f customer-support-v2.yaml

# Remove the failed version
kubectl delete llmloraadapter customer-support-v3 -n ml-team-a
```

---

## Troubleshooting

| Symptom | Check | Fix |
|---------|-------|-----|
| Adapter not loading | `kubectl describe llmloraadapter -n ml-team-a` | Check `hf://` URI, Vault token for private repos |
| vLLM returns 404 for adapter | `kubectl logs -n ml-team-a deploy/llama-3-8b -c vllm \| grep lora` | Re-apply LLMLoraAdapter to trigger reload |
| OOM after adapter load | `kubectl top pod -n ml-team-a` | Reduce `gpu_memory_utilization` in base service template, or use a smaller adapter rank |
| Adapter loads but quality degraded | Compare outputs with base model | Roll back adapter, validate training data |

---

## OTel Trace Events

The operator emits `ckodex.lora.swap_start` and `ckodex.lora.swap_done` span events on
each reconcile. Filter in Grafana Tempo:

```
{ resource.service.name = "ckodex-llm-operator" } | select(event.name = "ckodex.lora.swap_done")
```

The audit log also records `AuditLoraSwap` events:
```bash
kubectl logs -n ckodex-system deploy/ckodex-kserve-llm-operator | \
  grep '"action":"LoraSwap"'
```
