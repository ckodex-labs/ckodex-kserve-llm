# Air-Gapped OCI Model Distribution

The **ckodex-kserve-llm** operator is designed to operate in 100% disconnected environments (No Internet). In this mode, the operator redirects all model and infrastructure requests to a **Local OCI Registry**.

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
```

## 2. Model URI Redirection (Auto-Convert)

When `CKODEX_AIRGAPPED_MODE` is active, the operator automatically rewrites all external model URIs to your local registry.

| Original URI | Rewritten URI (Air-Gapped) |
| :--- | :--- |
| `hf://google/gemma-4` | `oci://registry.corp.internal/hf/google/gemma-4` |
| `oci://ghcr.io/ckodex/gemma:v1` | `oci://registry.corp.internal/ghcr.io/ckodex/gemma:v1` |

> [!TIP]
> Use the **ORAS** CLI to mirror HuggingFace models to your local registry before deployment:
> `oras copy hf://google/gemma-4 oci://registry.corp.internal/hf/google/gemma-4`

## 3. Infrastructure Image Redirection

The operator also rewrites all the infrastructure images it manages to ensure they are pulled from the local registry:

- `kserve/storage-initializer:v0.17.0` → `registry.corp.internal/kserve/storage-initializer:v0.17.0`
- `vllm/vllm-openai:v0.19.0` → `registry.corp.internal/vllm/vllm-openai:v0.19.0`

## 4. Offline Security Verification

In air-gapped mode, external Sigstore (TUF) and OIDC lookups are disabled. Verification relies strictly on the provided `LocalCosignKeyPath`.

The operator will:
1. Load the public key from the mounted volume.
2. Verify the OCI artifact signature against the local registry's manifest.
3. Update the `Compliance-SR-2` status condition to reflect offline verification.

---

> [!IMPORTANT]
> Ensure your local registry supports the OCI Artifacts spec (e.g., **Harbor 2.x**, **Zot**, or **Distribution v2.7+**).
