# Air-gap deployment and offline operations

The CKodex KServe LLM Operator supports disconnected, air-gapped sovereign environments. The operator operates with zero outbound Internet connectivity, redirecting all model downloads, container images, cryptographic verifications, and compliance assessments to local enterprise infrastructure.

## Five sovereignty tiers

The operator defines five progressive tiers of operational sovereignty:

| Tier | Name | Architectural boundary | Verification requirements |
|---|---|---|---|
| **Tier 1** | Local registry mirror | External Internet reachable for build, all deployment images pulled from local OCI registry | Local container image pull verification |
| **Tier 2** | Local model artifact store | Model weights mirrored to local S3 (SeaweedFS/MinIO) or OCI registry | Digest verification and storage initializer compatibility |
| **Tier 3** | Offline cryptographic verification | Sigstore TUF disabled; local Cosign public keys verify signatures and attestations | Ed25519 offline signature and SLSA provenance checks |
| **Tier 4** | Isolated private network | Strict Kubernetes network policies; zero egress to external IP ranges; local DNS only | Automated zero-phone-home test passing |
| **Tier 5** | Strict physical air-gap | Completely severed physical network; data imported exclusively via encrypted transfer capsules | Immutable offline evidence bundles and Lula OSCAL verification |

## Air-gap configuration

Configure the operator controller manager with air-gap environment variables:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ckodex-operator-controller-manager
  namespace: ckodex-system
spec:
  template:
    spec:
      containers:
        - name: manager
          image: registry.corp.internal/ckodex/operator:v0.20.0
          env:
            - name: CKODEX_AIRGAPPED_MODE
              value: "true"
            - name: CKODEX_LOCAL_REGISTRY
              value: "registry.corp.internal"
            - name: CKODEX_LOCAL_COSIGN_KEY_PATH
              value: "/etc/cosign/cosign.pub"
            - name: CKODEX_CUSTOM_STORAGE_INITIALIZER_IMAGE
              value: "registry.corp.internal/kserve/storage-initializer:v0.21.0"
            - name: CKODEX_RUNTIME_IMAGE
              value: "registry.corp.internal/vllm/vllm-openai:v0.33.0"
          volumeMounts:
            - name: cosign-key
              mountPath: /etc/cosign
              readOnly: true
      volumes:
        - name: cosign-key
          secret:
            secretName: cosign-airgap-public-key
```

When `CKODEX_AIRGAPPED_MODE` is enabled, the operator automatically rewrites all external model URIs and container images to the local mirror registry:

| Standard URI | Air-gapped rewritten URI |
|---|---|
| `hf://tiny-random/kimi-k3.1-pro` | `oci://registry.corp.internal/models/kimi-k31-pro:v1` |
| `hf://baidu/Unlimited-OCR-2` | `oci://registry.corp.internal/models/unlimited-ocr-2:v1` |
| `hf://facebook/sam3` | `oci://registry.corp.internal/models/sam3:v1` |
| `vllm/vllm-openai:v0.33.0` | `registry.corp.internal/vllm/vllm-openai:v0.33.0` |

## Image and artifact mirroring procedures

Before deploying into an air-gapped environment, mirror all required container images and model artifacts using `skopeo` or `oras`.

### 1. Mirror container images

```bash
REGISTRY="registry.corp.internal"

# Operator and initializers
skopeo copy docker://ghcr.io/runbase-ai/ckodex-kserve-llm-operator:v0.20.0 \
  docker://${REGISTRY}/ckodex/operator:v0.20.0

skopeo copy docker://kserve/storage-initializer:v0.21.0 \
  docker://${REGISTRY}/kserve/storage-initializer:v0.21.0

# Inference engine runtimes
skopeo copy docker://vllm/vllm-openai:v0.33.0 \
  docker://${REGISTRY}/vllm/vllm-openai:v0.33.0

skopeo copy docker://docker.io/envoyproxy/gateway:v1.2.0 \
  docker://${REGISTRY}/envoyproxy/gateway:v1.2.0
```

### 2. Package and mirror model weights via ORAS

Package Hugging Face weights into OCI artifacts and push to the private registry:

```bash
cd /staging/models/kimi-k3.1-pro
oras push registry.corp.internal/models/kimi-k31-pro:v1 \
  --config /dev/null:application/vnd.ckodex.model.config.v1+json \
  ./*:application/vnd.ckodex.model.layer.v1+tar
```

Sign the mirrored artifact with the private offline signing key:

```bash
cosign sign --key /keys/cosign-offline.key \
  registry.corp.internal/models/kimi-k31-pro:v1
```

## Pre-staging weights with LocalModelCache

To avoid saturating the local registry during autoscaling, use `LocalModelCache` to pre-stage weights directly onto cluster worker nodes:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LocalModelCache
metadata:
  name: kimi-k31-pro-cache
spec:
  modelName: tiny-random-kimi-k31-pro
  source:
    uri: oci://registry.corp.internal/models/kimi-k31-pro:v1
  nodeSelector:
    accelerator: nvidia-rtx-ada
  storage:
    size: 50Gi
    path: /var/lib/ckodex/models/kimi-k31-pro
  retentionPolicy: Retain
```

The operator schedules a daemon-style cache synchronization Job. Once cached, the storage initializer binds the local hostPath mount directly, reducing model initialization time from minutes to under 5 seconds.

## Offline verification and attestation

In air-gapped mode, external certificate authorities, transparency logs (Rekor), and OpenID Connect providers are unreachable. The operator enforces offline verification:

1. **Local Cosign public key**: Mounted via `CKODEX_LOCAL_COSIGN_KEY_PATH` or provided as an inline secret via `CKODEX_LOCAL_COSIGN_PUBLIC_KEY`.
2. **Signature verification**: Storage initializers execute `cosign verify --key /etc/cosign/cosign.pub` against the local OCI artifact.
3. **Attestation and provenance**: Verifies offline in-toto attestations (`--type slsaprovenance1`) and CycloneDX SBOMs (`--type cyclonedx`).
4. **Cache integrity locking**: If a cache directory is present on a node but lacks a valid cryptographic receipt from a prior verified download, the initializer rejects the cache and performs a full re-verification.

## Zero-phone-home test checklist

Before declaring an installation Tier 4 or Tier 5 compliant, execute the following validation commands from inside an inference pod:

```bash
# 1. Verify DNS does not resolve public domains
kubectl exec -it deploy/kimi-k31-specimen -c vllm -- \
  nslookup huggingface.co || echo "DNS egress successfully blocked"

# 2. Verify external HTTP egress fails immediately
kubectl exec -it deploy/kimi-k31-specimen -c vllm -- \
  curl -m 3 https://huggingface.co || echo "HTTPS egress successfully blocked"

# 3. Verify internal registry and model download succeed
kubectl exec -it deploy/kimi-k31-specimen -c vllm -- \
  curl -s -o /dev/null -w "%{http_code}\n" http://registry.corp.internal/v2/
```
