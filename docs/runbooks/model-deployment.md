# Runbook: Model Deployment & Canary Rollout

**Audience:** Platform engineers, on-call SREs
**Applies to:** `LLMInferenceService` CRD, Gateway canary HTTPRoute
**Related alerts:** `CanaryLatencyRegression`, `CanaryErrorRateHigh`, `LLMServiceNoReplicas`

---

## Standard Deployment

### 1. Apply the CRD

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: llama-3-8b
  namespace: ml-team-a
  labels:
    ckodex.com/tenant-id: ml-team-a
spec:
  model:
    uri: hf://meta-llama/Llama-3-8B-Instruct
    name: meta-llama/Llama-3-8B-Instruct
  replicas: 2
  costAllocationTags:
    team: ml-platform
    project: chatbot-v2
    cost-center: CC-9001
  slo:
    targetP99LatencyMs: 3000
    targetAvailability: 0.999
    errorBudgetDays: 30
  template:
    spec:
      containers:
        - name: vllm
          resources:
            limits:
              cpu: "8"
              memory: 32Gi
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

```bash
kubectl apply -f llama-3-8b.yaml
```

### 2. Monitor rollout

```bash
# Watch pod readiness
kubectl rollout status deployment/llama-3-8b -n ml-team-a

# Verify service URL
kubectl get llminferenceservice llama-3-8b -n ml-team-a -o jsonpath='{.status.url}'

# Check conditions
kubectl get llminferenceservice llama-3-8b -n ml-team-a -o jsonpath='{.status.conditions}'
```

### 3. Smoke test

```bash
ENDPOINT=$(kubectl get llminferenceservice llama-3-8b -n ml-team-a -o jsonpath='{.status.url}')
curl -s "${ENDPOINT}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model":"meta-llama/Llama-3-8B-Instruct","messages":[{"role":"user","content":"ping"}],"max_tokens":5}'
```

---

## Canary Rollout

Use canary to shift a percentage of traffic to a new model version before full promotion.

### Step 1 — Deploy the canary service

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: llama-3-8b-canary
  namespace: ml-team-a
spec:
  model:
    uri: hf://meta-llama/Llama-3-8B-Instruct-v2
    name: meta-llama/Llama-3-8B-Instruct
  replicas: 1
  canary:
    weight: 10        # Start with 10% of traffic
    baseModel: llama-3-8b   # Stable service gets 90%
  slo:
    targetP99LatencyMs: 3000
    targetAvailability: 0.999
  template:
    spec:
      containers:
        - name: vllm
          resources:
            limits:
              cpu: "8"
              memory: 32Gi
              nvidia.com/gpu: "1"
  router:
    gateway:
      existingRef:
        name: llama-3-8b-gateway
        namespace: ml-team-a
    route:
      httpRoute: {}
    scheduler:
      pool: {}
```

```bash
kubectl apply -f llama-3-8b-canary.yaml
```

The operator creates a weighted HTTPRoute with two backends automatically.

### Step 2 — Monitor canary health

Watch the `CanaryLatencyRegression` and `CanaryErrorRateHigh` alerts in Grafana.

```bash
# Compare P99 latency: canary vs stable
kubectl exec -it prometheus-pod -n monitoring -- promtool query instant \
  'ckodex:inference_latency_p99{serving_ckodex_com_canary="true"}'
```

### Step 3 — Progressively increase weight

```bash
# 10% → 25%
kubectl patch llminferenceservice llama-3-8b-canary -n ml-team-a \
  --type=merge -p '{"spec":{"canary":{"weight":25}}}'

# 25% → 50% after 30 minutes of clean metrics
kubectl patch llminferenceservice llama-3-8b-canary -n ml-team-a \
  --type=merge -p '{"spec":{"canary":{"weight":50}}}'

# 50% → 100% after the declared observation window
kubectl patch llminferenceservice llama-3-8b-canary -n ml-team-a \
  --type=merge -p '{"spec":{"canary":{"weight":100}}}'
```

### Step 4 — Rollback (set weight to 0)

```bash
kubectl patch llminferenceservice llama-3-8b-canary -n ml-team-a \
  --type=merge -p '{"spec":{"canary":{"weight":0}}}'
```

This routes all traffic back to the stable service without deleting either
workload.

### Step 5 — Promote the tested spec

After the canary satisfies the team-owned observation window:

1. Update the stable service manifest in Git or Helm with the tested canary
   model URI and runtime settings.
2. Apply that stable manifest and wait for its Deployment and
   `LLMInferenceService` readiness.
3. Set the canary weight to `0`.
4. Delete the canary only after the stable endpoint has passed a smoke test.

```bash
kubectl apply -f llama-3-8b-stable-v2.yaml
kubectl rollout status deployment/llama-3-8b -n ml-team-a

kubectl patch llminferenceservice llama-3-8b-canary -n ml-team-a \
  --type=merge -p '{"spec":{"canary":{"weight":0}}}'

kubectl delete llminferenceservice llama-3-8b-canary -n ml-team-a
```

---

## Troubleshooting

| Symptom | Check | Fix |
|---------|-------|-----|
| Pods not starting | `kubectl describe pod -n ml-team-a -l serving.ckodex.com/model=...` | Image pull failure, resource quota, node affinity |
| HTTPRoute not created | `kubectl get httproute -n ml-team-a` | Check gateway feature gate `CKODEX_FEATURE_ENABLE_GATEWAY=true` |
| Canary route not splitting | `kubectl get httproute -n ml-team-a -o yaml` | Verify both backend services exist |
| Model not ready | `kubectl logs -n ml-team-a deploy/llama-3-8b -c vllm` | GPU OOM, model download failure |
