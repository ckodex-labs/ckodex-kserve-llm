# Air-Gapped OCI Model Distribution

The **ckodex-kserve-llm** operator supports disconnected environments by redirecting model and infrastructure requests to a **Local OCI Registry**.

## 1. Enabling Air-Gapped Mode

Configure the operator via environment variables:

```yaml
env:
  - name: CKODEX_AIRGAPPED_MODE
    value: "true"
  - name: CKODEX_LOCAL_REGISTRY
    value: "registry.corp.internal"
  - name: CKODEX_LOCAL_COSIGN_KEY_PATH
    value: "/etc/cosign/cosign.pub"
  # Alternative when mounting a public-key file is inconvenient:
  - name: CKODEX_LOCAL_COSIGN_PUBLIC_KEY
    valueFrom:
      secretKeyRef:
        name: cosign-public-key
        key: cosign.pub
```

## 2. Model URI Redirection (Auto-Convert)

When `CKODEX_AIRGAPPED_MODE` is active, the operator automatically rewrites all external model URIs to your local registry.

| Original URI | Rewritten URI (Air-Gapped) |
| :--- | :--- |
| `hf://google/gemma-4` | `oci://registry.corp.internal/hf/google/gemma-4` |
| `oci://ghcr.io/ckodex/gemma:v1` | `oci://registry.corp.internal/ghcr.io/ckodex/gemma:v1` |
| `ocis://ghcr.io/ckodex/gemma:v1` | `ocis://registry.corp.internal/ghcr.io/ckodex/gemma:v1` |

> [!TIP]
> Use the **ORAS** CLI to mirror HuggingFace models to your local registry before deployment:
> `oras copy hf://google/gemma-4 oci://registry.corp.internal/hf/google/gemma-4`

## 3. Infrastructure Image Redirection

The operator also rewrites all the infrastructure images it manages to ensure they are pulled from the local registry:

- `kserve/storage-initializer:v0.17.0` → `registry.corp.internal/kserve/storage-initializer:v0.17.0`
- `vllm/vllm-openai:v0.19.0` → `registry.corp.internal/vllm/vllm-openai:v0.19.0`

## 4. Offline Security Verification

In air-gapped mode, external Sigstore (TUF) and OIDC lookups are disabled. `CKODEX_LOCAL_COSIGN_KEY_PATH` is the primary offline verification contract. `CKODEX_LOCAL_COSIGN_PUBLIC_KEY` is supported as an inline fallback for warm-up Jobs or other environments where mounting a file is awkward.

Current behavior:

1. For `oci://` and `ocis://` artifacts, the storage initializer runs `cosign verify`, `cosign verify-attestation --type slsaprovenance1`, and `cosign verify-attestation --type cyclonedx`.
2. The init container writes a machine-readable runtime verification record to its termination log and to the model cache, and the controllers only report `Compliance-SR-2=True` when that record shows signature, provenance, and SBOM attestation verification succeeded.
3. If the destination cache is already populated but has no matching verified cache record, the initializer refuses to reuse it.
4. Non-OCI sources may still download, but they do not qualify for cryptographic provenance verification and should not be treated as `verified` promotion inputs.

---

> [!IMPORTANT]
> Ensure your local registry supports the OCI Artifacts spec (e.g., **Harbor 2.x**, **Zot**, or **Distribution v2.7+**).
