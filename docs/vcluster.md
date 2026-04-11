# Deploying in a Virtual Cluster (vcluster)

The **ckodex-kserve-llm** operator supports "Virtual Multi-Tenancy" by running inside a **vcluster**. This allows teams to have true autonomy over their AI workloads (managing their own CRDs and lifecycles) while safely sharing host-cluster GPUs and a centralized service mesh.

## 1. Prerequisites

- **Host Cluster**: A Kubernetes cluster with NVIDIA GPUs and the NVIDIA Device Plugin installed.
- **Service Mesh**: Istio installed in the **Host Cluster**.
- **Security**: SPIRE installed in the **Host Cluster** (including the SPIFFE CSI driver).

## 2. Setting Up the VCluster

Create a vcluster for your tenant (e.g., `team-alpha`):

```bash
vcluster create team-alpha -n team-alpha-host
```

## 3. Sync Configuration

To ensure that the host-cluster Istio and SPIRE agents can see the virtual workloads, we must "sync" the `InferenceService` and associated resources back to the host cluster.

Add the following to your `vcluster.yaml` or use a plugin:

```yaml
# vcluster-sync-config.yaml
sync:
  generic:
    config: |-
      - apiVersion: serving.ckodex.com/v1alpha2
        kind: LLMInferenceService
      - apiVersion: serving.ckodex.com/v1alpha2
        kind: LLMLoraAdapter
```

## 4. Deploying the Operator

Install the operator **inside** the vcluster. Configure it using these environment variables to handle the virtual-to-physical mapping:

```yaml
env:
  - name: CKODEX_VCLUSTER_MODE
    value: "true"
  - name: CKODEX_VCLUSTER_HOST_NAMESPACE
    value: "team-alpha-host" # The shadow namespace in the host cluster
```

## 5. How it Works (Under the Hood)

- **Identity Mapping**: When you deploy an `InferenceService` in the vcluster namespace `default`, the operator generates a SPIFFE ID like `spiffe://ckodex.com/ns/default/sa/gemma-4/model/gemma-4`.
- **Selector Alignment**: The SPIRE registration entry created in the host cluster will use the identifier `k8s:ns:team-alpha-host` (the host shadow namespace), ensuring the SPIRE Agent on the physical node can attest the pod.
- **Socket Projection**: The SPIFFE CSI driver mounts the host-cluster SPIRE socket into the pods running in the tenant's namespace, enabling zero-trust mTLS without compromising isolation.

---

> [!IMPORTANT]
> Ensure that the `SPIRERegistrationNamespace` (default: `spire`) in your host cluster has appropriate RBAC to allow the vcluster's synced resources to write ConfigMaps or communicate with the SPIRE server.
