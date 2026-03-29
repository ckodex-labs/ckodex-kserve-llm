# Runbook: Security Incident Response

**Audience:** Security engineers, on-call SREs, incident commanders
**Applies to:** OPA policies, Vault, SPIRE, network policy, eBPF/Tetragon
**Related alerts:** `OPAPolicyViolation`, `VaultSecretRotationOverdue`, `TenantTokenBudgetExceeded`

---

## Severity Classification

| Severity | Examples | Response SLA |
|----------|----------|-------------|
| P0 — Critical | Active credential exfiltration, cross-tenant data access, model weight exfil | Immediate (< 5 min) |
| P1 — High | OPA policy bypass, Vault unsealed without expected reason, SPIRE agent compromise | 15 min |
| P2 — Medium | Repeated auth failures (brute force), policy violation by known tenant | 1 hour |
| P3 — Low | Secret rotation overdue, misconfigured network policy, debug log leakage | Next business day |

---

## EP-001: Immediate Containment (Active Exfiltration)

If you suspect active credential exfiltration or cross-tenant access:

```bash
# 1. Quarantine the affected namespace (block all egress immediately)
kubectl label namespace ml-team-a ckodex.com/quarantine=true

# 2. Delete active sessions (forces JWT re-validation)
kubectl delete pods -n ml-team-a -l serving.ckodex.com/model=...

# 3. Revoke all API keys for the tenant (Vault)
vault kv delete secret/ckodex/api-keys/ml-team-a

# 4. Block the tenant's network policy
kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: quarantine-block
  namespace: ml-team-a
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
  # No rules = block everything
EOF
```

Then escalate to the security team immediately. Document time of containment.

---

## OPA Policy Violation (`OPAPolicyViolation` alert)

### What it means

The OPA Gatekeeper constraint rejected a resource (image from unallowed registry,
GPU count above namespace quota, or cross-tenant model access attempt).

### Investigation

```bash
# See which resource was rejected
kubectl get events -n ckodex-system --field-selector reason=FailedAdmission

# See all Gatekeeper constraint violations
kubectl get constrainttemplate
kubectl get constraint -A

# Get violation details
kubectl describe llmregistrycheck -A | grep -A10 "Violations:"
```

### Response

1. If the violation is legitimate (attacker trying to use an unregistered image): log and monitor.
2. If the violation is a false positive (new registry not in allowlist):
   ```bash
   # Add registry to operator config
   kubectl patch configmap ckodex-operator-config -n ckodex-system \
     --type=merge -p '{"data":{"CKODEX_SECURITY_ALLOWED_REGISTRIES":"ghcr.io,gcr.io,quay.io,new-registry.example.com"}}'
   kubectl rollout restart deploy/ckodex-kserve-llm-operator -n ckodex-system
   ```

---

## Vault Secret Rotation Overdue (`VaultSecretRotationOverdue` alert)

### Investigation

```bash
# List secrets past rotation deadline
vault kv list secret/ckodex/ | while read path; do
  vault kv metadata get "secret/ckodex/$path" | grep -E "created_time|updated_time"
done

# Check the ckodex.com/rotate-after annotation
kubectl get secret -A -l ckodex.com/api-key=true -o jsonpath=\
  '{range .items[*]}{.metadata.namespace}/{.metadata.name}: {.metadata.annotations.ckodex\.com/rotate-after}{"\n"}{end}'
```

### Rotation procedure

```bash
# 1. Generate new API key
NEW_KEY=$(openssl rand -hex 32)

# 2. Create new secret (bcrypt-hashed)
HASH=$(python3 -c "import bcrypt; print(bcrypt.hashpw(b'${NEW_KEY}', bcrypt.gensalt()).decode())")
kubectl create secret generic api-key-ml-team-a-new \
  -n ml-team-a \
  --from-literal=key="${HASH}" \
  --from-literal=tenant_id="ml-team-a"

# 3. Label it as a CKodex API key
kubectl label secret api-key-ml-team-a-new -n ml-team-a \
  ckodex.com/api-key=true

kubectl annotate secret api-key-ml-team-a-new -n ml-team-a \
  ckodex.com/rotate-after=$(date -v+30d +%Y-%m-%dT%H:%M:%SZ)

# 4. Distribute new key to tenant out-of-band
# 5. Delete old secret after tenant confirms migration
kubectl delete secret api-key-ml-team-a -n ml-team-a
```

---

## Auth Failure Spike (Brute Force / Credential Stuffing)

```bash
# Count auth failures per subject in last 5 min
kubectl logs -n ckodex-system deploy/ckodex-kserve-llm-operator --since=5m | \
  grep '"action":"CredentialAccess"' | \
  grep '"outcome":"Failure"' | \
  jq -r '.actor' | sort | uniq -c | sort -rn | head -20
```

If a single subject appears >50 times:

```bash
# Block via NetworkPolicy (replace CLIENT_IP with the offending IP)
kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: block-brute-force
  namespace: ml-team-a
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/managed-by: ckodex-kserve-llm-operator
  policyTypes: [Ingress]
  ingress:
  - from:
    - ipBlock:
        cidr: 0.0.0.0/0
        except: [CLIENT_IP/32]
EOF
```

---

## eBPF / Tetragon Forensics

If Tetragon detected a suspicious syscall (alert from Tetragon TracingPolicy):

```bash
# View recent Tetragon events
kubectl exec -n kube-system daemonset/tetragon -- tetra getevents -o compact | head -100

# Export events for forensics (JSONL format)
kubectl exec -n kube-system daemonset/tetragon -- \
  tetra getevents -o json --namespace ml-team-a > /tmp/tetragon-forensics.jsonl

# Filter for specific model pod
cat /tmp/tetragon-forensics.jsonl | \
  jq 'select(.process.pod.name | startswith("llama-3-8b"))' | head -50
```

---

## Post-Incident

Within 24 hours of incident closure:

1. **Audit log export:** `kubectl logs -n ckodex-system deploy/ckodex-kserve-llm-operator --since=24h | grep audit_event > incident-$(date +%Y%m%d).log`
2. **Policy review:** Was the violated/triggered policy correctly scoped?
3. **ADR:** Create an ADR in `docs/adr/` if a policy or architecture change is needed.
4. **Evidence bundle:** Collect logs, Tetragon events, Vault audit logs into a signed bundle per `docs/api-deprecation-policy.md` retention class guidelines.
