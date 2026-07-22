# Hugging Face: First Model

Use `hf://` first. The operator runs the current `hf download` client with Xet
support into the shared model volume; it does not use KServe v0.19's older
Hugging Face initializer path and does not require the Hugging Face CSI driver.

## Public model smoke test

```bash
kubectl create namespace ckodex-inference --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/samples/llminferenceservice_huggingface.yaml
kubectl get llminferenceservice,pod -n ckodex-inference -w
```

The sample uses `openai-community/gpt2` to test storage and reconciliation on
CPU. It is a plumbing check, not a performance or model-quality benchmark.

If the pod does not start, inspect the download before the model server:

```bash
POD=$(kubectl get pod -n ckodex-inference -l serving.ckodex.com/service=hf-gpt2 -o jsonpath='{.items[0].metadata.name}')
kubectl logs -n ckodex-inference "$POD" -c storage-initializer
kubectl describe pod -n ckodex-inference "$POD"
```

## Gated or private repositories

Create one namespace-local Secret. The key is `HF_TOKEN` for both `hf://` and
`hf-mount://`; the token is referenced, never embedded in the model URI.

```bash
kubectl create secret generic hf-credentials \
  --namespace ckodex-inference \
  --from-literal=HF_TOKEN="$HF_TOKEN"
```

Add the reference to the model:

```yaml
spec:
  model:
    name: my-model
    uri: hf://my-org/my-model
    storage:
      secretRef:
        name: hf-credentials
```

Confirm that the Hugging Face account has accepted any gated-model license.
A valid token does not grant access until that account has access to the repo.

## Lazy mount with `hf-mount://`

Use this only when you deliberately want the CSI/FUSE path. The driver requires
FUSE on every target node and privileged node/mount pods. The operator chart
does not install that cluster-wide prerequisite.

```bash
helm install hf-csi oci://ghcr.io/huggingface/charts/hf-csi-driver \
  --version 0.12.1 \
  --namespace kube-system
kubectl get csidriver hf.csi.huggingface.co
```

Then change the URI to `hf-mount://openai-community/gpt2`. The operator creates
a static PV/PVC pair and, when `storage.secretRef` is present, configures the
CSI driver to read the same `HF_TOKEN` key shown above.

```bash
kubectl get pv,pvc -n ckodex-inference
kubectl get hfmounts -A
kubectl get pods -n kube-system -l app.kubernetes.io/name=hf-csi-driver
```

The driver version is pinned to the current repository-tested prerequisite.
Recheck the upstream release before changing it.

## Pre-fetched PVC

For large models, a pre-populated PVC avoids repeating the network transfer
during every rollout. Both a PVC root and a directory inside the claim are
supported:

```yaml
spec:
  model:
    uri: pvc://gemma4-weights/gemma-4-26B-A4B-it-NVFP4
```

The first path segment is the claim name; the remainder becomes the
`model-store` mount's `subPath`. The vLLM container always receives
`--model /mnt/models`, including when a Well-Known model profile adds other
arguments.

## Downloader behavior

The `hf://` init container uses `hf download`, not the removed
`huggingface-cli` command. `spec.model.storage.secretRef` is projected with
`envFrom`, so an `HF_TOKEN` key reaches the client. The downloader image and
client versions are pinned in `internal/controller/api/constants.go`; changing
them requires the storage-initializer tests and a live gated/Xet model check.
