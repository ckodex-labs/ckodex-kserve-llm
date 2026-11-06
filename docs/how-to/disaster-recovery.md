# Recover from cluster and node failures

This guide provides procedures for restoring the operator, CRD states, and managed LLM inference workloads following cluster failure, etcd loss, or accidental deletion.

## Overview

Disaster recovery for LLM serving requires restoring both declarative control plane states (CRDs, secrets, configurations) and large model storage artifacts (PVCs or node cache volumes). Kubernetes self-heals individual worker pod failures; this runbook addresses catastrophic loss.

## Prerequisites

- The `velero` CLI installed and connected to your backup storage location.
- `kubectl` with cluster-admin permissions.
- Access to your GitOps repository or declarative manifest backups.
- Helm version 3.12 or newer.

## Step 1: Verify backup availability

Check available Velero backups:

```bash
velero backup get --selector app.kubernetes.io/name=ckodex-kserve-llm-operator
```

Inspect the most recent backup details:

```bash
velero backup describe ckodex-daily-<TIMESTAMP> --details
```

Check the backup logs for warnings or partial failures:

```bash
velero backup logs ckodex-daily-<TIMESTAMP> | grep -i "error\|warn"
```

If no backup is available, proceed to [Step 6: Bare-metal GitOps recovery](#step-6-bare-metal-gitops-recovery).

## Step 2: Restore custom resource definitions

CRDs are cluster-scoped and must exist before restoring namespaced resources or installing the operator Helm chart:

```bash
velero restore create ckodex-crd-restore \
  --from-backup ckodex-daily-<TIMESTAMP> \
  --include-resources customresourcedefinitions \
  --wait
```

Verify that all operator CRDs are restored:

```bash
kubectl get crd | grep ckodex.com
```

Confirm that the output includes:
- `llminferenceservices.serving.ckodex.com`
- `aipacks.serving.ckodex.com`
- `multimodalinferenceservices.serving.ckodex.com`
- `agents.serving.ckodex.com`
- `localmodelcaches.serving.ckodex.com`

## Step 3: Restore the operator controller manager

Restore the `ckodex-system` namespace from backup:

```bash
velero restore create ckodex-operator-restore \
  --from-backup ckodex-daily-<TIMESTAMP> \
  --include-namespaces ckodex-system \
  --wait
```

Verify that the operator pod is running:

```bash
kubectl get pods -n ckodex-system -l control-plane=controller-manager
```

If the operator deployment was not backed up, reinstall it using Helm:

```bash
helm upgrade --install ckodex-operator deploy/helm \
  --namespace ckodex-system \
  --create-namespace \
  --values deploy/helm/values.yaml \
  --wait
```

## Step 4: Restore tenant workloads and trigger reconciliation

Restore tenant namespaces containing `LLMInferenceService` and `AIPack` definitions:

```bash
velero restore create ckodex-tenant-restore \
  --from-backup ckodex-daily-<TIMESTAMP> \
  --include-resources \
    llminferenceservices.serving.ckodex.com,\
    aipacks.serving.ckodex.com,\
    multimodalinferenceservices.serving.ckodex.com,\
    agents.serving.ckodex.com,\
    localmodelcaches.serving.ckodex.com \
  --wait
```

Touch an annotation on the restored resources to trigger immediate reconciliation:

```bash
kubectl annotate llminferenceservices.serving.ckodex.com --all -A \
  ckodex.com/dr-restore-trigger="$(date +%s)" --overwrite
```

## Step 5: Re-populate model weight volumes

Model weights live in PersistentVolumeClaims rather than etcd. If persistent storage was destroyed, run a one-off storage initializer Job to re-populate the volume:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: model-restore-job
  namespace: default
spec:
  template:
    spec:
      containers:
        - name: storage-init
          image: kserve/storage-initializer:v0.21.0@sha256:3a3a6a8053478c8d8d90f96fc5e54c1892c07784102dda70104374d4f89a3672
          env:
            - name: STORAGE_URI
              value: "hf://Qwen/Qwen2.5-7B-Instruct"
          volumeMounts:
            - mountPath: /mnt/models
              name: model-pvc
      volumes:
        - name: model-pvc
          persistentVolumeClaim:
            claimName: qwen-pvc
      restartPolicy: OnFailure
```

Apply the job:

```bash
kubectl apply -f model-restore-job.yaml
kubectl wait --for=condition=complete job/model-restore-job --timeout=600s
```

## Step 6: Bare-metal GitOps recovery

If no Velero backup is available, recover cluster state directly from GitOps source repositories:

1. **Apply CRD definitions from repository**:
   ```bash
   kubectl apply -f config/crd/
   ```
2. **Deploy the operator**:
   ```bash
   helm upgrade --install ckodex-operator deploy/helm \
     --namespace ckodex-system \
     --create-namespace \
     --values deploy/helm/values.yaml \
     --wait
   ```
3. **Apply tenant workloads from Git**:
   ```bash
   kubectl apply -f gitops/tenants/
   ```

## Step 7: Traffic failover via HTTPRoute weight shifting

If the primary cluster is degraded during recovery, shift ingress traffic to a secondary standby cluster using HTTPRoute weight shifting:

```bash
kubectl patch httproute qwen25-7b -n default --type=json \
  -p='[
    {"op":"replace","path":"/spec/rules/0/backendRefs/0/weight","value":0},
    {"op":"replace","path":"/spec/rules/0/backendRefs/1/weight","value":100}
  ]'
```

Verify traffic shifting in the Envoy Gateway access logs:

```bash
kubectl logs -n envoy-gateway-system -l app.kubernetes.io/name=envoy --tail=100 -f
```

## Step 8: Post-recovery verification

Verify system health across all four validation points:

1. Operator controller manager is running with ready replicas:
   ```bash
   kubectl get pods -n ckodex-system -l control-plane=controller-manager
   ```
2. All inference services report `Ready: True`:
   ```bash
   kubectl get llminferenceservices -A \
     -o custom-columns="NAMESPACE:.metadata.namespace,NAME:.metadata.name,READY:.status.conditions[?(@.type=='Ready')].status"
   ```
3. Envoy Gateway and HTTPRoute objects are synchronized:
   ```bash
   kubectl get httproutes -A
   ```
4. Send an end-to-end inference health query:
   ```bash
   curl -s -X POST http://localhost:8000/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{"model":"qwen-7b-instruct","messages":[{"role":"user","content":"ping"}]}' | jq .
   ```

## Next steps

- [Review security architecture](../explanation/security-architecture.md)
- [Troubleshoot workload failures](troubleshoot-workload-failures.md)
- [Consult the CLI cheat sheet](../reference/cheat-sheet.md)
