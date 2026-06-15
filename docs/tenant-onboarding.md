# Tenant Onboarding Guide

This guide describes the standard procedure for setting up a new multi-tenant environment within the CKodex KServe cluster.

## Tenant Isolation Strategy

CKodex implements multi-tenancy through a combination of Kubernetes native features and custom CRDs.

### 1. Namespace-Based Isolation

Every tenant must occupy one or more dedicated Kubernetes namespaces.

- **`ckodex.com/tenant-id`**: All tenant namespaces must be labeled with this ID. This label is used by the **LLMModelAccess** OPA Gatekeeper constraint to audit and enforce model-to-tenant permission bindings.

### 2. Multi-Tenant Identity (SPIRE)

The operator automatically manages **SPIFFE** identities for all inference workloads.

- **Workload ID**: `spiffe://ckodex.com/ns/{ns}/sa/{sa}/model/{model}`
- **mTLS Enforcement**: The Gateway and sidecars use these SVIDs to guarantee that only authorized clients (from the allowed tenant ID) can communicate with the inference backend.

## Security & Compliance Silos

Use `LLMInferenceServiceConfig` to apply pre-validated security profiles (Compliance Profiles) for each tenant.

### Standard Compliance Profiles

- **`hipaa`**: Enforces JWT-based auth and disables local model caching (PCI-DSS mode).
- **`soc2`**: Enforces eBPF-based security monitoring and durable audit sinks.
- **`fedramp`**: Restricts model downloads to FedRAMP-authorized OCI registries only.

### Example Tenant Config

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LLMInferenceServiceConfig
metadata:
  name: healthcare-tenant-std
  namespace: healthcare-prod
spec:
  complianceProfiles:
    - hipaa
    - soc2
  vllmDefaults:
    args:
      - "--disable-log-stats"
```

## Resource Quotas & Scheduling

To prevent a single tenant from exhausting cluster-wide GPU resources, apply standard `ResourceQuota` and `LimitRange` objects in the tenant namespace.

### Recommended Quota Pattern

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: gpu-quota
  namespace: tenant-a
spec:
  hard:
    requests.nvidia.com/gpu: 4
    limits.nvidia.com/gpu: 4
```

> [!TIP]
> **PriorityClasses**: For critical production tenants, assign a higher `PriorityClass` to their inference services to ensure they are scheduled before experimental or batch workloads during resource contention.

## Workflow: Onboarding a New Tenant

1. **Create Namespace**: `kubectl create namespace tenant-a`.
2. **Apply Tenant Label**: `kubectl label namespace tenant-a ckodex.com/tenant-id=t-12345`.
3. **Configure Identity**: Assign a `ServiceAccount` with the appropriate SPIRE-aware permissions.
4. **Deploy Base Config**: Apply the tenant's `LLMInferenceServiceConfig` for compliance enforcement.
5. **Verify Access**: Attempt an inference request using the `InferenceSession` API to confirm end-to-end connectivity.
