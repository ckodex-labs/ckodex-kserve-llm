# Getting Started

Welcome! This guide will walk you through installing the CKodex KServe LLM Operator and deploying your first model in under 5 minutes.

## Prerequisites

- **Kubernetes Cluster**: v1.28 or later.
- **Docker**: Required for the local KIND bootstrap.
- **kind**: Used by the repo-native local E2E path.
- **kubectl**: Required for cluster inspection and cleanup.
- **Helm**: v3.10+.
- **curl** and **jq**: Used by the local inference probe.
- **Network access**: Needed to fetch charts and container images.

GPU-backed deployments still need GPU nodes and the matching driver stack, but
the default local KIND E2E path is intentionally CPU-capable and does not
require a GPU driver.

## 🌈 The Community Production Route (Recommended)

To install the operator into a production cluster, use the official Helm chart:

```bash
# Add the CKodex Labs repo
helm repo add ckodex https://ckodex-labs.github.io/ckodex-kserve-llm
helm repo update

# Install the operator into a dedicated namespace
helm install kserve-llm-operator ckodex/ckodex-kserve-llm-operator \
  --namespace ckodex-system \
  --create-namespace \
  --set crds.install=true
```

## 🛠 The Local Development Route (Kind)

For testing locally on your laptop, we provide a streamlined setup using `kind`.

### 1. Run the full local E2E flow

```bash
./run/e2e.sh
```

This command:

- creates the KIND cluster if it does not already exist
- installs cert-manager, Gateway API, Envoy Gateway, MetalLB, and the HuggingFace CSI driver
- builds and loads the operator image into KIND
- applies CRDs and deploys the controller
- installs the sample `LLMInferenceService`
- runs the live inference probe against `/v1/chat/completions`

For sizing larger models without downloading or running them, see
[Frontier Model Capacity Planning](model-capacity.md) or run
`./run/capacity-plan.sh`.

### 2. Inspect the cluster

Verify the manager is running:

```bash
kubectl get pods -n ckodex-system
```

### 3. Clean up when finished

```bash
./run/cleanup.sh
```

---

## 🚀 Deploying Your First Model (Gemma 4)

The operator includes a **WellKnown** model registry. This means it already knows how to optimize popular models like Gemma 4 for production performance (enabling `TurboQuant`, setting Guaranteed QoS, etc.).

### 1. Create the Inference Service

Apply the following manifest. Even with minimal configuration, the operator will apply best-practice defaults.

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: gemma-4-e2b
spec:
  model:
    uri: "hf://google/gemma-4-E2B-it" # You may need an HF_TOKEN secret
  replicas: 1
```

### 2. Monitor Readiness

```bash
kubectl get llminferenceservice gemma-4-e2b -w
```

### 3. Send a Request

Once ready, the operator automatically creates a Kubernetes Service (and optionally an `HTTPRoute`). You can port-forward to test it:

```bash
kubectl port-forward svc/gemma-4-e2b 8000:8000 &

curl -X POST http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gemma-4-e2b","messages":[{"role":"user","content":"Hello!"}]}'
```

---

## CRD Reference

| CRD | Kind | Purpose |
|---|---|---|
| `LLMInferenceService` | `LLMInferenceService` | Core model serving CR — model URI, replicas, routing, scaling |
| `LLMInferenceServiceConfig` | `LLMInferenceServiceConfig` | Cluster-wide operator defaults and compliance profiles |
| `LocalModelCache` | `LocalModelCache` | Node-local model weight cache |
| `ModelOnboarding` | `ModelOnboarding` | Promotion gate workflow for new models |

---

## Next Steps

- **[Gemma 4 Deployment Guide](gemma4-deployment-guide.md)**: Deep dive into the Gemma 4 optimizations.
- **[Frontier Model Capacity Planning](model-capacity.md)**: Static fit assessment for GLM-5.2 and Kimi K2.7 Code.
- **[Architecture Decision Records](adr/)**: Understand why we built it this way.
- **Join the Community**: Star us on GitHub or join our community Slack!
