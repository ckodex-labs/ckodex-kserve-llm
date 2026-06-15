# Model Onboarding Guide

This guide provides a step-by-step workflow for onboarding a new Large Language Model (LLM) into the CKodex KServe environment, from initial binary acquisition to production promotion.

## Phase 1: Model Acquisition & URIs

CKodex supports multiple model distribution schemes. Select the one best suited for your infrastructure:

- **`hf://<repo-id>`**: Direct download from Hugging Face Hub (requires `HUGGING_FACE_HUB_TOKEN` if private).
- **`oci://<registry>/<image>:<tag>`**: Models packaged as OCI artifacts.
- **`ocis://<registry>/<image>:<tag>`**: Same OCI transport, but with explicit secure-OCI intent. It routes through the same runtime signature, provenance, and SBOM attestation verification path as `oci://`.
- **`s3://<bucket>/<path>`**: Models stored in S3-compatible object storage.
- **`pvc://<claim-name>/<path>`**: Pre-existing models on a PersistentVolumeClaim.
- **`modelpack://<name>`**: CKodex-optimized model packages with built-in metadata.

## Phase 2: Zero-Latency Startup via `LocalModelCache`

To avoid multi-gigabyte downloads during pod startup, use `LocalModelCache` to pre-warm nodes.

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LocalModelCache
metadata:
  name: llama-3-8b-cache
spec:
  sourceModelUri: "hf://meta-llama/Meta-Llama-3-8B-Instruct"
  modelSize: "15Gi"
  warmNodes:
    - "gpu-node-01"
    - "gpu-node-02"
```

The operator will create a `storage-initializer` Job on each target node to populate a node-local PVC.

## Phase 3: Service Deployment

Define the `LLMInferenceService` to manage the serving workload.

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: llama-3-8b
spec:
  model:
    uri: "pvc://llama-3-8b-cache/model" # Points to the local cache
    name: "llama-3-8b"
  parallelism:
    tensor: 1
    data: 2
  template:
    spec:
      containers:
        - name: vllm
          resources:
            limits:
              nvidia.com/gpu: 1
```

> [!TIP]
> **Guaranteed QoS**: The operator automatically synchronizes resource requests to match limits for optimal scheduling performance.

## Phase 4: Automated Promotion Pipeline

For production workloads, use the `ModelOnboarding` CRD to automate the promotion cycle.

```yaml
apiVersion: serving.ckodex.com/v1
kind: ModelOnboarding
metadata:
  name: llama-3-8b-promotion
spec:
  modelRef: "llama-3-8b"
  stages:
    - name: "static-validation"
      type: "validation"
    - name: "canary-rollout"
      type: "canary"
      gate:
        minSuccessRate: 99
        maxLatencyP99: 500
    - name: "promote-to-stable"
      type: "promotion"
```

### Pipeline Stages

1. **Validation**: Checks model integrity and configuration compatibility.
2. **Canary**: Routes a small percentage of traffic (configured via `CanarySpec` in the service) and monitors metrics.
3. **Gate**: Blocking stage that requires success metrics to be met (e.g., MinSuccessRate).
4. **Promotion**: Updates the stable Gateway route to point entirely to the new model version.

---

## Troubleshooting

- **Phase 2 Fails**: Check `LocalModelCache` node statuses and Job logs for storage credential issues.
- **Phase 4 Fails**: If `rollbackOnFailure` is true (default), the operator will automatically revert the Gateway route to the previous stable version.
