# Deploy the operator in an air-gapped environment

This guide describes how to install and operate the CKodex KServe LLM Operator in disconnected, sovereign environments without outbound Internet connectivity.

## The five sovereignty tiers

The operator supports five progressive tiers of operational sovereignty:

| Tier | Name | Architectural boundary | Verification requirements |
|---|---|---|---|
| **Tier 1** | Local registry mirror | External Internet reachable for build, all deployment images pulled from local OCI registry | Local container image pull verification |
| **Tier 2** | Local model artifact store | Model weights mirrored to local S3 (SeaweedFS/MinIO) or OCI registry | Digest verification and storage initializer compatibility |
| **Tier 3** | Offline cryptographic verification | Sigstore TUF disabled; local Cosign public keys verify signatures and attestations | Ed25519 offline signature and SLSA provenance checks |
| **Tier 4** | Isolated private network | Strict Kubernetes network policies; zero egress to external IP ranges; local DNS only | Automated zero-phone-home test passing |
| **Tier 5** | Strict physical air-gap | Completely severed physical network; data imported exclusively via encrypted transfer capsules | Immutable offline evidence bundles and Lula OSCAL verification |

## Prerequisites

- Access to a connected staging machine with `skopeo`, `oras`, `cosign`, and `helm` installed.
- An enterprise private OCI registry inside the air-gapped network (for example, Harbor or an internal mirror).
- A private S3-compatible storage system inside the air-gapped network (such as SeaweedFS or MinIO).
- Offline Ed25519 cryptographic signing keys.

## Step 1: Mirror container images to private registry

On the connected staging workstation, mirror all operator and runtime container images:

```bash
LOCAL_REGISTRY="registry.corp.internal"

# Operator controller manager and initializers
skopeo copy docker://ghcr.io/runbase-ai/ckodex-kserve-llm-operator:v0.20.0 \
  docker://${LOCAL_REGISTRY}/ckodex/operator:v0.20.0

skopeo copy docker://kserve/storage-initializer:v0.21.0 \
  docker://${LOCAL_REGISTRY}/kserve/storage-initializer:v0.21.0

# vLLM runtime engine
skopeo copy docker://vllm/vllm-openai:v0.33.0 \
  docker://${LOCAL_REGISTRY}/vllm/vllm-openai:v0.33.0

# Envoy Gateway
skopeo copy docker://docker.io/envoyproxy/gateway:v1.2.0 \
  docker://${LOCAL_REGISTRY}/envoyproxy/gateway:v1.2.0
```

## Step 2: Package and mirror model weights via ORAS

Package model weights into OCI artifacts and push to the private registry:

```bash
cd /staging/models/qwen2.5-7b

oras push registry.corp.internal/models/qwen25-7b:v1 \
  --config /dev/null:application/vnd.ckodex.model.config.v1+json \
  ./*:application/vnd.ckodex.model.layer.v1+tar
```

Sign the mirrored artifact with your offline private key:

```bash
cosign sign --key /keys/cosign-offline.key \
  registry.corp.internal/models/qwen25-7b:v1
```

## Step 3: Configure operator air-gap settings

Create a secret containing your Cosign public key in the operator namespace:

```bash
kubectl create namespace ckodex-system
kubectl create secret generic cosign-airgap-public-key \
  --namespace ckodex-system \
  --from-file=cosign.pub=/keys/cosign-offline.pub
```

Configure operator Helm values (`airgap-values.yaml`) to enforce local mirrors and offline verification:

```yaml
env:
  CKODEX_AIRGAPPED_MODE: "true"
  CKODEX_LOCAL_REGISTRY: "registry.corp.internal"
  CKODEX_LOCAL_COSIGN_KEY_PATH: "/etc/cosign/cosign.pub"
  CKODEX_RUNTIME_IMAGE: "registry.corp.internal/vllm/vllm-openai:v0.33.0"
  CKODEX_CUSTOM_STORAGE_INITIALIZER_IMAGE: "registry.corp.internal/kserve/storage-initializer:v0.21.0"

volumes:
  - name: cosign-key
    secret:
      secretName: cosign-airgap-public-key

volumeMounts:
  - name: cosign-key
    mountPath: /etc/cosign
    readOnly: true
```

Install the operator with these values:

```bash
helm upgrade --install ckodex-operator deploy/helm \
  --namespace ckodex-system \
  --values airgap-values.yaml \
  --wait
```

When `CKODEX_AIRGAPPED_MODE` is enabled, the operator rewrites public model URIs and container images to the local mirror automatically:

| Public URI | Air-gapped rewritten URI |
|---|---|
| `hf://Qwen/Qwen2.5-7B-Instruct` | `oci://registry.corp.internal/models/qwen25-7b:v1` |
| `hf://baidu/Unlimited-OCR-2` | `oci://registry.corp.internal/models/unlimited-ocr-2:v1` |
| `vllm/vllm-openai:v0.33.0` | `registry.corp.internal/vllm/vllm-openai:v0.33.0` |

## Step 4: Pre-stage weights with LocalModelCache

To prevent bandwidth saturation across the private registry during scaling events, pre-stage weights to local node NVMe storage using `LocalModelCache`:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LocalModelCache
metadata:
  name: qwen25-7b-node-cache
spec:
  modelName: qwen2.5-7b-instruct
  source:
    uri: oci://registry.corp.internal/models/qwen25-7b:v1
  nodeSelector:
    accelerator: nvidia-rtx-ada
  storage:
    size: 50Gi
    path: /var/lib/ckodex/models/qwen25-7b
  retentionPolicy: Retain
```

The operator manages a daemon synchronization Job. Once cached, model initialization binds the hostPath volume directly, reducing pod startup from minutes to under 5 seconds.

## Step 5: Execute the zero-phone-home test checklist

Execute these verification commands from inside a test workload pod to prove network isolation:

```bash
# 1. Verify external public DNS is blocked
kubectl exec -it deploy/qwen25-7b -c vllm -- \
  nslookup huggingface.co || echo "DNS egress blocked as expected"

# 2. Verify external HTTPS connections time out
kubectl exec -it deploy/qwen25-7b -c vllm -- \
  curl -m 3 https://huggingface.co || echo "HTTPS egress blocked as expected"

# 3. Verify internal registry connection succeeds
kubectl exec -it deploy/qwen25-7b -c vllm -- \
  curl -s -o /dev/null -w "%{http_code}\n" http://registry.corp.internal/v2/
```

## Next steps

- [Consult security architecture](../explanation/security-architecture.md)
- [Review compliance mappings with OSCAL](../explanation/compliance-oscal.md)
- [Plan disaster recovery procedures](disaster-recovery.md)
