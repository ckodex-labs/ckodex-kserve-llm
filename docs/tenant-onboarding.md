# Tenant Onboarding Guide

This guide defines the platform-owned steps for adding a tenant namespace.

## 1. Create and Label the Namespace

```bash
kubectl create namespace tenant-a
kubectl label namespace tenant-a ckodex.com/tenant-id=t-12345
```

The tenant label is consumed by quota and policy paths. A label alone does not
create isolation; RBAC, NetworkPolicy, identity, and Gateway policy must also be
configured.

## 2. Grant Operator Access

Add the namespace to `managedNamespaces` in the Helm values so the operator
service account receives the namespace-scoped RoleBinding:

```yaml
managedNamespaces:
  - tenant-a
```

Verify:

```bash
kubectl auth can-i create deployments \
  --as=system:serviceaccount:ckodex-system:ckodex-kserve-llm-operator \
  -n tenant-a
```

Use the actual service-account name from the installed release if it differs.

The same Helm profile pre-provisions the shared EPP identity used by scheduler
pods in `tenant-a`. Verify the namespace-scoped objects before enabling a
scheduler workload:

```bash
kubectl get serviceaccount,role,rolebinding ckodex-epp -n tenant-a
```

## 3. Apply Resource Limits

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: gpu-quota
  namespace: tenant-a
spec:
  hard:
    requests.nvidia.com/gpu: "4"
    limits.nvidia.com/gpu: "4"
```

Also apply a `LimitRange`, storage quotas, and any organization-specific
PriorityClasses.

## 4. Configure Optional Security

SPIFFE/SPIRE and OPA integration are disabled by default. Enable security only
after SPIRE, its CSI driver, policy dependencies, RBAC, and health checks are
available:

```text
CKODEX_FEATURE_ENABLE_SECURITY=true
```

When enabled, the operator creates SPIRE registration entries with IDs shaped
as:

```text
spiffe://ckodex.com/ns/{namespace}/sa/{service-account}/model/{model}
```

The registration entry enables SVID issuance. End-to-end mTLS still depends on
the workload API socket, sidecars or clients, trust bundles, and Gateway policy.

## 5. Configure Compliance Profiles

Runtime profile enforcement is configured on the operator process:

```text
CKODEX_COMPLIANCE_PROFILES=hipaa,soc2
```

The startup validator checks the corresponding operator feature gates, audit
sink, retention, redaction, and cache posture. The
`LLMInferenceServiceConfig.spec.complianceProfiles` field exists in the alpha
API but is not currently consumed by a reconciler; do not treat that field as
active enforcement.

## 6. Deploy and Verify a Tenant Workload

Use a stable v1 `LLMInferenceService`, then inspect:

```bash
kubectl get llminferenceservice -n tenant-a
kubectl get deployment,service,gateway,httproute -n tenant-a
kubectl get networkpolicy -n tenant-a
kubectl get events -n tenant-a --sort-by=.lastTimestamp
```

For security-enabled clusters, also inspect the SPIRE registration entry and
verify an actual SVID through the SPIFFE workload API.

## Acceptance Checklist

- operator RBAC is limited to the intended namespace;
- tenant labels and quotas are present;
- default-deny and required allow policies are active;
- model storage credentials are namespace-scoped;
- routes do not expose unintended hostnames;
- Prometheus and audit data carry tenant identity;
- optional compliance profiles pass startup validation;
- an inference request succeeds using the tenant's authorized path.

See [Security Architecture](SECURITY_ARCHITECTURE.md) for the control model and
[Model Onboarding](onboarding-guide.md) for workload creation.
