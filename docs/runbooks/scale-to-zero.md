# Runbook: Scale-to-Zero & Cold Start Recovery

**Audience:** Platform engineers, on-call SREs
**Applies to:** KEDA ScaledObject, vLLM Deployment
**Related alerts:** `LLMServiceNoReplicas`, `LLMServiceSLOLatencyBreach` (cold start spike)

---

## Overview

Scale-to-zero terminates all inference pods when request rate drops to zero for the
configured idle window. KEDA restores replicas when a new request arrives. Cold starts
for large models (70B+) can take 3–8 minutes while the model downloads to GPU memory.

This runbook covers: how to diagnose scale-to-zero, how to tune cold start, and how to
respond to incidents where a service is stuck at zero replicas.

---

## Normal Scale-to-Zero Flow

```
Requests → 0 for idleWindow
  └─ KEDA sets replicas: 0
     └─ vLLM pod terminates
        └─ First new request blocked (gateway returns 503)
           └─ KEDA detects queue depth > 0
              └─ Sets replicas: 1
                 └─ Pod starts → model loads (~3 min for 8B)
                    └─ Readiness probe passes
                       └─ Gateway routes traffic
```

---

## Check Current State

```bash
# See current replicas
kubectl get deployment -n ml-team-a llama-3-8b

# See KEDA ScaledObject status
kubectl get scaledobject -n ml-team-a

# See queue depth (why KEDA triggered scale-out)
kubectl get hpa -n ml-team-a

# Timeline of scale events (OTel trace events)
# In Grafana Tempo, filter:
# { resource.service.name = "ckodex-llm-operator" } | select(event.name = "ckodex.scale.from_zero")
```

---

## Tuning Scale-to-Zero

The KEDA idle window and initial cooldown are configured in `spec.scaling.keda`:

```yaml
spec:
  scaling:
    minReplicas: 0          # Allow scale to zero
    maxReplicas: 8
    keda:
      idleReplicaCount: 0
      pollingInterval: 15   # Check queue every 15s
      cooldownPeriod: 300   # Wait 5 min before scaling in
      initialCooldownPeriod: 60
      fallback:
        failureThreshold: 3
        replicas: 1         # Keep 1 replica if KEDA fails 3 consecutive polls
```

**Anti-pattern:** Setting `minReplicas: 0` on a model with an SLO of <500ms P99 — the
cold start will breach the SLO. Use `minReplicas: 1` for latency-sensitive workloads.

---

## Service Stuck at Zero Replicas (Incident)

### Symptoms

- `LLMServiceNoReplicas` alert fires
- Gateway returns 503 to all clients
- KEDA ScaledObject shows `ScaledObject is paused` or trigger polling fails

### Diagnosis

```bash
# 1. Is KEDA healthy?
kubectl get scaledobject -n ml-team-a -o yaml | grep -A5 "conditions:"

# 2. Is the KEDA trigger metric source (Prometheus) reachable?
kubectl logs -n keda deploy/keda-operator | tail -50

# 3. Is the Deployment itself blocked?
kubectl describe deployment llama-3-8b -n ml-team-a
kubectl get events -n ml-team-a --sort-by=.metadata.creationTimestamp | tail -20
```

### Fix A — Manual scale-out (immediate)

```bash
kubectl scale deployment llama-3-8b -n ml-team-a --replicas=1
```

This bypasses KEDA and brings up one replica immediately. KEDA will take back control
on the next poll cycle.

### Fix B — KEDA metric source broken

```bash
# Restart KEDA operator
kubectl rollout restart deploy/keda-operator -n keda

# Pause and resume ScaledObject
kubectl annotate scaledobject llama-3-8b-keda -n ml-team-a \
  autoscaling.keda.sh/paused-replicas=1

kubectl annotate scaledobject llama-3-8b-keda -n ml-team-a \
  autoscaling.keda.sh/paused-replicas-
```

### Fix C — Node has no GPU capacity

```bash
# Check node GPU allocations
kubectl describe nodes | grep -A5 "Allocatable:" | grep nvidia

# Add a node to the GPU node pool (cloud-provider specific)
# GKE: gcloud container node-pools update ...
# EKS: eksctl scale nodegroup ...
```

---

## Pre-warming (Prevent Cold Start)

For business-critical models, pre-warm by keeping `minReplicas: 1`. Use KEDA
`initialCooldownPeriod` to delay the first scale-in after deployment.

For cost-sensitive workloads that still need fast cold starts, use `LocalModelCache`:
the model weights are pre-loaded on every node so pod startup is <30 seconds instead
of 3–8 minutes.

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LocalModelCache
metadata:
  name: llama-3-8b-cache
spec:
  sourceModelURI: hf://meta-llama/Llama-3-8B-Instruct
  nodeSelector:
    cloud.google.com/gke-accelerator: nvidia-l4
```

---

## Monitoring

| Metric | Query |
|--------|-------|
| Current replicas | `kube_deployment_status_replicas_available{deployment="llama-3-8b"}` |
| Scale events | `ckodex:inference_latency_p99` — spike indicates cold start |
| Zero-replica windows | Grafana dashboard → "Scale-to-Zero Timeline" panel |
