# Upgrade the operator to v0.20.0

This guide explains how to upgrade the CKodex KServe LLM Operator to release `v0.20.0` (commit `9da7f18`) with zero inference downtime.

## What is new in v0.20.0

- Updates default inference runtime to vLLM v0.33.0 (CUDA 13.0, PyTorch 2.14.0).
- Pure-domain Universal Tool Calling Protocol (UTCP v1.0.1) execution.
- Multimodal foundation models: Baidu Unlimited-OCR-2 and Meta SAM 3.
- LMCache v0.5.6 shared-memory prefix caching.
- Normalized pod and container field diffs (`L-OP-018`) eliminating reconciliation drift loops.
- Canonical Helm chart structure in `deploy/helm` with byte-parity contract checks.

## Pre-upgrade checklist

Complete these pre-flight checks before upgrading:

1. **Kubernetes version**: Verify the cluster runs Kubernetes v1.30 or newer:
   ```bash
   kubectl version
   ```
2. **GPU host driver**: If using GPUs, verify host drivers support CUDA 13.0 (driver version 580.178.04 or compatible):
   ```bash
   kubectl get nodes -l nvidia.com/gpu.present=true \
     -o custom-columns=NAME:.metadata.name,KERNEL:.status.nodeInfo.kernelVersion
   ```
3. **Backup active custom resources**: Export snapshots of all active custom resources:
   ```bash
   mkdir -p /tmp/ckodex-backup-pre-v020
   kubectl get llminferenceservices.serving.ckodex.com -A -o yaml > /tmp/ckodex-backup-pre-v020/llminferenceservices.yaml
   kubectl get aipacks.serving.ckodex.com -A -o yaml > /tmp/ckodex-backup-pre-v020/aipacks.yaml
   kubectl get agents.serving.ckodex.com -A -o yaml > /tmp/ckodex-backup-pre-v020/agents.yaml
   kubectl get multimodalinferenceservices.serving.ckodex.com -A -o yaml > /tmp/ckodex-backup-pre-v020/multimodal.yaml
   ```

## Step 1: Apply custom resource definitions

Apply the updated CRDs before deploying the new controller manager:

```bash
kubectl apply -f config/crd/serving.ckodex.com_llminferenceservices.yaml
kubectl apply -f config/crd/serving.ckodex.com_aipacks.yaml
kubectl apply -f config/crd/serving.ckodex.com_multimodalinferenceservices.yaml
kubectl apply -f config/crd/serving.ckodex.com_agents.yaml
kubectl apply -f config/crd/serving.ckodex.com_localmodelcaches.yaml
kubectl apply -f config/crd/serving.ckodex.com_modelservingprofiles.yaml
```

Confirm that the CRDs reflect generation updates:

```bash
kubectl get crd llminferenceservices.serving.ckodex.com -o jsonpath='{.metadata.generation}'
```

## Step 2: Upgrade operator controller manager via Helm

Upgrade the Helm release in `ckodex-system`:

```bash
helm upgrade ckodex-operator deploy/helm \
  --namespace ckodex-system \
  --reuse-values \
  --set image.repository=ghcr.io/runbase-ai/ckodex-kserve-llm-operator \
  --set image.tag=v0.20.0
```

Wait for the controller manager rollout to complete:

```bash
kubectl rollout status deployment/ckodex-operator-controller-manager \
  --namespace ckodex-system \
  --timeout=120s
```

Inspect the operator startup log to confirm version v0.20.0:

```bash
kubectl logs -n ckodex-system -l control-plane=controller-manager -c manager --tail=30
```

The log output reports:
```text
{"level":"info","msg":"Starting controller-runtime manager","version":"v0.20.0","commit":"9da7f18"}
```

## Step 3: Validate reconciler diff normalization

The `v0.20.0` operator includes diff normalization (`L-OP-018`) in `internal/controller/reconciler/diff.go`. Existing deployments do not restart unless their specification was modified.

Verify that existing workloads remain running without pod restarts:

```bash
kubectl get pods -l app.kubernetes.io/managed-by=ckodex-operator -A
```

Verify that reconcile error counts remain zero:

```bash
kubectl exec -n ckodex-system deploy/ckodex-operator-controller-manager -c manager -- \
  curl -s http://localhost:8080/metrics | grep controller_runtime_reconcile_errors_total
```

## Step 4: Upgrade workloads to vLLM v0.33.0

Once the operator is stable, upgrade individual inference workloads:

1. Update the image reference in your service manifest:
   ```yaml
   spec:
     template:
       spec:
         containers:
           - name: vllm
             image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
   ```
2. Apply the manifest:
   ```bash
   kubectl apply -f my-workload.yaml
   ```
3. Observe the zero-downtime rolling update:
   ```bash
   kubectl rollout status deployment/my-workload
   ```
4. Query the health check:
   ```bash
   kubectl exec -it deploy/my-workload -c vllm -- curl -s http://localhost:8000/health
   ```

## Rollback procedure

If unrecoverable errors occur during the operator upgrade:

1. Roll back the Helm release to the previous revision:
   ```bash
   helm rollback ckodex-operator -n ckodex-system
   ```
2. Verify that the previous controller manager pod resumes running:
   ```bash
   kubectl rollout status deployment/ckodex-operator-controller-manager -n ckodex-system
   ```
3. If necessary, restore CRD schemas from the pre-upgrade backup:
   ```bash
   kubectl apply -f /tmp/ckodex-backup-pre-v020/
   ```

## Next steps

- [Review operator defaults and configuration](../reference/operator-defaults.md)
- [Consult runtime engine contracts](../reference/engine-contract.md)
- [Troubleshoot workload failures](troubleshoot-workload-failures.md)
