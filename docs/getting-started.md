# Getting Started

Read [the big picture](overview.md) first if you do not yet know which parts are
owned by the scientist, the platform team, and the operator.

## Choose a Path

| Goal | Path |
|---|---|
| Prove the repository works on a laptop | Local KIND proof |
| Deploy a model on an existing CKodex cluster | Scientist path |
| Integrate the operator into a cluster | Platform developer path |

## Prerequisites

For the local proof:

- Docker
- KIND
- Kubernetes CLI (`kubectl`)
- Helm
- `curl` and `jq`
- network access for charts and images

GPU-backed models additionally require compatible nodes, drivers, runtime
configuration, and model images. The local proof uses a small CPU-capable model
and does not prove GPU performance.

## Local KIND Proof

Run:

```bash
./run/e2e.sh
```

The script:

1. creates or reuses the `kserve-017` KIND cluster;
2. installs cert-manager, Gateway API, Envoy Gateway, MetalLB, KServe, and the
   Gateway API Inference Extension CRDs;
3. builds and loads the operator and storage initializer images;
4. installs CRDs and RBAC;
5. applies `local/04-llm-inference-service.yaml`;
6. waits for readiness and sends an OpenAI-compatible chat request.

The local manifest uses the served stable `v1` API and the signed `hf://`
storage-initializer path. The optional `hf-mount://` path uses the privileged
Hugging Face CSI/FUSE dependency and is exercised separately with
`INSTALL_HF_CSI=1` and `local/04-llm-inference-service-hfmount.yaml`. Fields that
remain explicitly experimental are documented under `spec.experimental` in v1,
while v1alpha2 remains available for compatibility during the deprecation
window.

Inspect the control plane:

```bash
kubectl get pods -n ckodex-system
kubectl get llminferenceservice llama3-8b -n default -o wide
kubectl describe llminferenceservice llama3-8b -n default
```

Inspect generated resources:

```bash
kubectl get deployment,service -n default
kubectl get gateway,httproute -n default
kubectl get events -n default --sort-by=.lastTimestamp
```

Clean up:

```bash
./run/cleanup.sh
```

## Scientist Path

Assumption: the platform team has installed the operator and its cluster
dependencies and has given you a namespace.

1. Start from the stable maintained example:

   ```bash
   cp config/samples/llminferenceservice_basic.yaml /tmp/my-model.yaml
   ```

2. Change:

   - `metadata.name` and `metadata.namespace`;
   - `spec.model.name`, which clients send in inference requests;
   - `spec.model.uri`, which tells the runtime where weights live;
   - container resources and parallelism for the target hardware.

3. Apply and observe:

   ```bash
   kubectl apply -f /tmp/my-model.yaml
   kubectl get llminferenceservice -n <namespace> -w
   ```

4. Diagnose readiness from the custom resource, Deployment, and events:

   ```bash
   kubectl describe llminferenceservice <name> -n <namespace>
   kubectl get pods -n <namespace>
   kubectl get events -n <namespace> --sort-by=.lastTimestamp
   ```

5. Use the route or port-forward supplied by the platform team, then send
   `spec.model.name` in the request's `model` field.

For capacity planning and promotion, continue with
[Model Onboarding](onboarding-guide.md).

## Platform Developer Path

The repository-native bootstrap in `run/e2e.sh` is the maintained integration
reference. For another cluster, reproduce its dependency contract rather than
assuming the Helm chart installs every prerequisite.

At minimum:

1. install Gateway API and the selected Gateway implementation;
2. install the storage path required by model URIs;
3. apply CRDs from `config/crd/`;
4. apply cluster and namespace RBAC;
5. deploy the operator with feature gates matching installed dependencies;
6. configure Prometheus before using metrics-backed promotion gates;
7. verify with a small model before introducing GPU or distributed inference.

The chart under `deploy/helm/` does not install CRDs or all external
dependencies. Review `deploy/helm/values.yaml` before use.

For a concrete public-model smoke test, gated-repository credentials, and the
optional CSI/FUSE path, follow [Hugging Face: First Model](huggingface.md).

## Repository Verification

```bash
dagger call all --source=.
dagger call test --source=.
dagger call scan --source=.
```

See [CI State](ci/current-state.md) for function contracts and
[Release Verification](release-verification.md) for published artifacts.

## Next Steps

- [Big Picture](overview.md)
- [Model Onboarding](onboarding-guide.md)
- [Model Capacity](model-capacity.md)
- [Tenant Onboarding](tenant-onboarding.md)
- [Model Deployment Runbook](runbooks/model-deployment.md)
