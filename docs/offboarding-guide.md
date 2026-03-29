# Model Offboarding Guide

This guide describes the recommended procedure for decommissioned models and their associated resources to ensure high availability and resource efficiency.

## Overview
Offboarding involves more than simply deleting a workload. It requires graceful session draining, Gateway route cleanups, and node-local storage deprovisioning.

## Step 1: Graceful Shutdown & Session Draining

Before deleting a service, its pods must finish processing active inference requests.

- **`TerminationGracePeriod`**: The operator defaults this to **30 seconds** for all inference services. This provides `vLLM` or `faster-whisper` runtimes enough time to finish the current token generation or transcription.
- **`InferenceSession` Cleanup**: If a model uses stateful sessions (KV-cache affinity), these will naturally time out based on their `IdleTimeout` (default 5m) once the model is unreachable.

## Step 2: Service Deletion

Deleting the `LLMInferenceService` (or ASR/Multimodal variant) triggers a series of automated cleanups managed by the operator's finalizers.

```bash
kubectl delete llminferenceservice llama-3-8b
```

### Automated Cleanup Actions:
1. **Gateway Routes**: The operator removes the `HTTPRoute` or `GRPCRoute` associated with the service to stop traffic.
2. **InferencePool**: The internal `InferencePool` mapping is deleted.
3. **LeaderWorkerSet (LWS)**: The underlying distributed GPU pods are terminated according to their grace periods.
4. **Resilience Policies**: Circuit breakers and bulkheads for the service are deprovisioned.

## Step 3: Cache Eviction via `LocalModelCache`

Cached model weights occupy significant node-local storage (often 20Gi–100Gi+). These must be explicitly evicted when a model is no longer required in a specific node group.

### Option A: Partial Eviction (Safe Mode)
Remove specific node names from the `warmNodes` list in the `LocalModelCache` CR. The operator will delete the PVCs on those nodes.

### Option B: Full Decommissioning
Delete the `LocalModelCache` resource itself:

```bash
kubectl delete localmodelcache llama-3-8b-cache
```

> [!IMPORTANT]
> **Finalizer Safety**: The `LocalModelCache` finalizer ensures that all node-local PVCs are deleted before the CR is removed from the Kubernetes API, preventing orphaned storage volumes.

## Step 4: Identity Decommissioning

If the service used **SPIRE** for identity, its SVID entries will automatically expire. However, you should remove any specific `ckodex.com/tenant-id` labels from the namespace if that tenant is also being offboarded.

---

## Best Practices
- **Verify Traffic**: Check Prometheus metrics for `llm_inference_request_count` to ensure zero active traffic before beginning a manual offboarding.
- **Audit Logs**: Review the `Controller Manager` logs to confirm that all finalizers completed without errors.
