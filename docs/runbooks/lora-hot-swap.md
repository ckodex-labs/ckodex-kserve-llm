# Runbook: LoRA Adapter Hot-Swap

**Audience:** ML engineers, platform engineers
**Applies to:** `LLMLoraAdapter`, `LocalModelCache`, vLLM adapter API

## Runtime Contract

The reconciler creates a `LocalModelCache` for the adapter, waits for cache
readiness and governance checks, verifies the target service is ready, then
calls the vLLM load API. Deleting the adapter triggers an unload attempt through
its finalizer.

No latency objective is asserted by this runbook. Measure adapter load time in
the target cluster.

## Apply an Adapter

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMLoraAdapter
metadata:
  name: customer-support-v3
  namespace: ml-team-a
spec:
  targetService: llama-3-8b
  adapterName: customer-support-v3
  model:
    uri: hf://acme/llama3-customer-support-v3
    name: customer-support-v3
```

```bash
kubectl apply -f customer-support-v3.yaml
kubectl get llmloraadapter customer-support-v3 -n ml-team-a -w
```

## Verify Loading

Inspect the adapter, generated cache, target model, and events:

```bash
kubectl describe llmloraadapter customer-support-v3 -n ml-team-a
kubectl get localmodelcache lora-customer-support-v3
kubectl get llminferenceservice llama-3-8b -n ml-team-a
kubectl get events -n ml-team-a --sort-by=.lastTimestamp
```

Verify the runtime model list:

```bash
kubectl exec -n ml-team-a deploy/llama-3-8b -- \
  curl -s http://localhost:8000/v1/models
```

The output must include `customer-support-v3` before clients use that model
name.

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

## Replace a Version

Use a new resource and adapter name so the previous version remains available
for rollback:

```bash
kubectl apply -f customer-support-v4.yaml
kubectl wait --for=condition=Ready \
  llmloraadapter/customer-support-v4 -n ml-team-a --timeout=120s
```

After clients have switched and the new adapter is verified:

```bash
kubectl delete llmloraadapter customer-support-v3 -n ml-team-a
```

Deletion invokes the unload path. Confirm removal through `/v1/models` and the
adapter's Kubernetes events.

## Troubleshooting

| Symptom | Check |
|---|---|
| Cache never becomes ready | `LocalModelCache` conditions and warm-up Job logs |
| Target service is missing | `spec.targetService`, namespace, service readiness |
| Governance blocks loading | adapter state planes, evidence fields, warning events |
| vLLM rejects loading | operator logs and target pod logs |
| Adapter causes memory pressure | pod events, GPU memory, adapter rank and count |

The observability package defines LoRA audit and trace event types, but the
current reconciler does not emit those helpers directly. Use adapter status,
Kubernetes events, and controller logs as the operational evidence.
