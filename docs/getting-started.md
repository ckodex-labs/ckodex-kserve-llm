# Getting Started

Welcome! This guide will walk you through installing the CKodex KServe LLM Operator and deploying your first model in under 5 minutes.

## Prerequisites

- **Kubernetes Cluster**: v1.28 or later.
- **GPU Driver**: NVIDIA driver (with `nvidia.com/gpu` resource available).
- **Helm**: v3.10+ (recommended for installation).
- **Gateway API CRDs**: Ensure Gateway API CRDs are installed if you want automatic routing.

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

### 1. Create a local cluster
```bash
make kind-setup
```

### 2. Build and load the operator
```bash
make docker-build
make kind-load
make deploy
```

Verify the manager is running:
```bash
kubectl get pods -n ckodex-system
```

---

## 🚀 Deploying Your First Model (Gemma 4)

The operator includes a **WellKnown** model registry. This means it already knows how to optimize popular models like Gemma 4 for production performance (enabling `TurboQuant`, setting Guaranteed QoS, etc.).

### 1. Create the Inference Service
Apply the following manifest. Even with minimal configuration, the operator will apply best-practice defaults.

```yaml
apiVersion: serving.ckodex.com/v1alpha2
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
- **[Architecture Decision Records](adr/)**: Understand why we built it this way.
- **Join the Community**: Star us on GitHub or join our community Slack!
