# Disaster Recovery Runbook — CKodex KServe LLM Operator

**Scope**: This runbook covers restoring the operator and all managed LLM inference workloads
after a catastrophic cluster failure (etcd loss, accidental CRD deletion, operator namespace
wipe). It does not cover individual pod or deployment recovery — Kubernetes self-heals those.

**Prerequisites**:

- `velero` CLI installed and pointed at the cluster
- `kubectl` with cluster-admin access
- Object storage credentials (S3/GCS/Azure Blob) configured in Velero
- Helm 3.x

---

## 1. Verify Backup Exists

```bash
# List available backups — check the most recent daily backup
velero backup get --selector app.kubernetes.io/name=ckodex-kserve-llm-operator

# Inspect a specific backup
velero backup describe ckodex-daily-<TIMESTAMP> --details

# Check for any partial failures in the backup
velero backup logs ckodex-daily-<TIMESTAMP> | grep -i "error\|warn"
```

If no backup exists, skip to **Section 5: Bare-Metal Recovery**.

---

## 2. Restore CRDs

CRDs are cluster-scoped and must be restored before the operator is deployed, otherwise
`kubectl apply` of the operator Helm chart will fail on missing GVKs.

```bash
# Restore only CRD resources first (no namespace filter needed — CRDs are cluster-scoped)
velero restore create ckodex-crd-restore \
  --from-backup ckodex-daily-<TIMESTAMP> \
  --include-resources customresourcedefinitions \
  --wait

# Verify CRDs are present
kubectl get crd | grep ckodex.com
# Expected output includes:
# llminferenceservices.serving.ckodex.com
# llminferenceserviceconfigs.serving.ckodex.com
# llmloraadapters.serving.ckodex.com
# inferencesessions.serving.ckodex.com
# localmodelcaches.serving.ckodex.com
```

---

## 3. Restore the Operator

```bash
# Restore the operator namespace and all its resources
velero restore create ckodex-operator-restore \
  --from-backup ckodex-daily-<TIMESTAMP> \
  --include-namespaces ckodex-system \
  --wait

# Verify operator pod is running
kubectl -n ckodex-system get pods -l control-plane=controller-manager

# Check operator logs for reconcile errors
kubectl -n ckodex-system logs -l control-plane=controller-manager --tail=50
```

If the operator deployment is missing after the restore (e.g., the backup predates the
current operator version), reinstall via Helm:

```bash
helm upgrade --install ckodex-kserve-llm-operator deploy/helm \
  --namespace ckodex-system \
  --create-namespace \
  --values deploy/helm/values.yaml \
  --wait
```

---

## 4. Restore Tenant Namespaces and CR State

```bash
# Restore all tenant namespaces (contains LLMInferenceService CRs)
velero restore create ckodex-tenant-restore \
  --from-backup ckodex-daily-<TIMESTAMP> \
  --include-resources \
    llminferenceservices.serving.ckodex.com,\
    llmloraadapters.serving.ckodex.com,\
    inferencesessions.serving.ckodex.com,\
    localmodelcaches.serving.ckodex.com \
  --wait

# Verify CRs are present in tenant namespaces
kubectl get llminferenceservices --all-namespaces

# Trigger reconciliation by touching a CR annotation
kubectl -n <tenant-ns> annotate llminferenceservice <name> \
  ckodex.com/dr-restore-trigger="$(date +%s)" --overwrite
```

---

## 5. Re-download Model Artifacts (if PVCs were lost)

Model weights are not stored in etcd — they live in PVCs or object storage. If PVCs were
lost with the cluster (no CSI snapshots), re-download from source.

### From HuggingFace (or internal mirror)

```bash
# Run the storage initializer as a one-off Job to re-populate a PVC
kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: model-restore-<MODEL_NAME>
  namespace: <TENANT_NS>
spec:
  template:
    spec:
      containers:
        - name: storage-init
          image: kserve/storage-initializer:v0.20.0
          env:
            - name: STORAGE_URI
              value: "hf://<ORG>/<REPO>"
            - name: HF_TOKEN
              valueFrom:
                secretKeyRef:
                  name: hf-token
                  key: token
          volumeMounts:
            - mountPath: /mnt/models
              name: model-pvc
      volumes:
        - name: model-pvc
          persistentVolumeClaim:
            claimName: <EXISTING_PVC_NAME>
      restartPolicy: OnFailure
EOF
```

### From S3/OCI

```bash
# If model was previously pushed to a registry
kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
...
          env:
            - name: STORAGE_URI
              value: "s3://<BUCKET>/models/<MODEL_NAME>"
EOF
```

---

## 6. Traffic Failover via HTTPRoute Weight Shifting

If the primary cluster is still partially functional but the gateway or EPP scheduler is
degraded, shift all traffic to a DR cluster using HTTPRoute weight splitting.

```bash
# Inspect current HTTPRoute weights
kubectl -n <TENANT_NS> get httproute <MODEL_NAME> -o yaml | grep -A5 backendRefs

# Shift 100% traffic to DR cluster backend
kubectl -n <TENANT_NS> patch httproute <MODEL_NAME> \
  --type=json \
  -p='[{"op":"replace","path":"/spec/rules/0/backendRefs/0/weight","value":0},
       {"op":"replace","path":"/spec/rules/0/backendRefs/1/weight","value":100}]'

# Monitor inference traffic (check Envoy access logs)
kubectl -n <TENANT_NS> logs -l gateway.networking.k8s.io/gateway-name=<GATEWAY_NAME> \
  --tail=100 -f
```

---

## 7. Post-Recovery Verification

```bash
# 1. All operator pods running and ready
kubectl -n ckodex-system get pods -l control-plane=controller-manager
# READY should be 2/2 (or configured replicaCount)

# 2. All LLMInferenceServices reconciled (no error conditions)
kubectl get llminferenceservices --all-namespaces \
  -o custom-columns="NAMESPACE:.metadata.namespace,NAME:.metadata.name,READY:.status.conditions[?(@.type=='Ready')].status"

# 3. EPP scheduler pods running
kubectl get pods --all-namespaces -l app.kubernetes.io/component=epp-scheduler

# 4. Gateway routes functional — send a test inference request
curl -s -X POST http://<GATEWAY_IP>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <TOKEN>" \
  -d '{"model":"<MODEL_NAME>","messages":[{"role":"user","content":"ping"}]}' \
  | jq .choices[0].message.content

# 5. Confirm backup schedule is still active
velero schedule get ckodex-daily
```

---

## 5 (alt). Bare-Metal Recovery (No Backup Available)

If no Velero backup exists, reconstruct from GitOps source:

```bash
# 1. Apply CRDs from source
kubectl apply -f config/crd/bases/

# 2. Install operator
helm upgrade --install ckodex-kserve-llm-operator deploy/helm \
  --namespace ckodex-system --create-namespace \
  --values deploy/helm/values.yaml --wait

# 3. Re-apply tenant CRs from Git
kubectl apply -f gitops/tenants/

# 4. Re-download all model artifacts (see Section 5 above)
```

**RTO/RPO estimates (with daily Velero backups)**:

- RTO (Recovery Time Objective): ~45 minutes (CRD restore + operator install + model PVC re-mount)
- RPO (Recovery Point Objective): ~24 hours (last successful daily backup)

For shorter RPO, increase backup frequency in `velero-backup-schedule.yaml` (e.g., `0 */6 * * *` for 6-hour backups).
