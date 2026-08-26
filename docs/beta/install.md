# Beta install profile

The stable `LLMInferenceService` v1 contract is supported only by the controlled
beta profile below. The profile uses a fixed release name, namespace, webhook
Service name, and cert-manager CA-injection reference so the CRD conversion
client configuration and Helm-rendered TLS resources cannot drift apart.

Prerequisites:

- cert-manager is installed and its CRDs/controllers are ready;
- the cluster has the Gateway API and the runtime dependencies required by the
  selected workload profile;
- the operator and console images referenced by the published chart are
  available to the cluster.

Install the published chart first, using the fixed identity:

```bash
helm upgrade --install ckodex-kserve-llm-operator \
  oci://ghcr.io/ckodex-labs/charts/ckodex-kserve-llm-operator \
  --namespace ckodex-system --create-namespace \
  --set fullnameOverride=ckodex-kserve-llm-operator \
  --set webhook.enabled=true \
  --set certManager.enabled=true \
  --set console.enabled=true
```

Wait for the webhook certificate and Deployment, then apply the checksummed CRD
bundle from the same release:

```bash
kubectl -n ckodex-system wait --for=condition=Ready \
  certificate/ckodex-kserve-llm-operator-webhook-cert --timeout=5m
kubectl -n ckodex-system rollout status \
  deployment/ckodex-kserve-llm-operator-controller-manager --timeout=5m
kubectl apply --server-side -f ckodex-crds.yaml
```

The release bundle is rendered from `kubectl kustomize config/crd`; it includes
the `Webhook` conversion strategy for `llminferenceservices.serving.ckodex.com`
and targets `/convert` on the fixed webhook Service. Verify the binding before
creating a v1 resource:

```bash
kubectl get crd llminferenceservices.serving.ckodex.com \
  -o jsonpath='{.spec.conversion.webhook.clientConfig.service.name}{"\n"}'
kubectl get crd llminferenceservices.serving.ckodex.com \
  -o jsonpath='{.spec.conversion.webhook.clientConfig.service.path}{"\n"}'
```

The raw files under `config/crd/` remain available for the webhook-disabled
development profile. That path does not provide the beta conversion guarantee;
use the checksummed release bundle or `make beta-crds` for beta acceptance.
