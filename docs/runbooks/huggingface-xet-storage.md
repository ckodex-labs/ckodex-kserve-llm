# Hugging Face Xet Storage Paths

This repository's Hugging Face initializer implements KServe's custom storage
container interface:

```text
<source-uri> <destination-path>
```

It translates `hf://`, `huggingface://`, and `hf-mirror://` sources to the
hash-locked `hf download` client. The same image is therefore usable by the
CKodex compatibility controller and KServe `ClusterStorageContainer`.

## KServe LocalModelCache Downloads

Use
[`config/samples/kserve-hf-cluster-storage-container.yaml`](../../config/samples/kserve-hf-cluster-storage-container.yaml)
for KServe's native `serving.kserve.io/v1alpha1 LocalModelCache` download Jobs.
Before applying it:

1. Replace the image tag with the published digest for the release.
2. Create `hf-secret` in the namespace configured by
   `localModel.jobNamespace` (normally `kserve-localmodel-jobs`).
3. Keep `workloadType: localModelDownloadJob`.

The workload type is material. If it is omitted, KServe defaults it to
`initContainer`; a second `hf://` match then competes with KServe's default
storage container, and selection is not deterministic.

Do not reuse this `localModelDownloadJob` resource for a normal KServe
`InferenceService` init container. Replacing KServe's default handler requires a
separate migration that removes its existing `hf://` match before registering a
new `initContainer` handler. This initializer currently implements the
single-source `<source-uri> <destination-path>` contract; it does not advertise
KServe multi-model download support.

## Separate Cache Concerns

- `LocalModelCache` caches model weights on node storage to reduce cold-start
  download time.
- LMCache offloads and shares inference KV tensors. It requires a vLLM
  `LMCacheConnectorV1` configuration and a backing store; enabling vLLM prefix
  caching is not equivalent.
- KServe multi-node inference is a different topology. KServe 0.19 requires
  Standard mode, disabled autoscaling, and an RWX PVC; tensor and pipeline
  parallelism are configured through `workerSpec`, not environment variables.

The CKodex `serving.ckodex.com/v1alpha2 LocalModelCache` is a compatibility API,
not the KServe resource with the same kind name. Migration to the upstream
KServe cache API must be explicit; the two controllers do not share status,
PVC ownership, or download-job configuration.
