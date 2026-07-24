# Hugging Face Xet Storage Paths

This repository's Hugging Face initializer implements KServe's custom storage
container interface:

```text
<source-uri> <destination-path>
```

It translates the KServe-standard `hf://` source and CKodex's `hf-mirror://`
source to the hash-locked `hf download` client. The same image is therefore
usable by the CKodex compatibility controller and KServe
`ClusterStorageContainer`.

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
  Standard mode, disabled autoscaling, and a `pvc://` model URI backed by a
  `ReadWriteMany` PVC. Tensor and pipeline parallelism are
  configured through `workerSpec`, not environment variables. See
  [KServe v0.19 Multi-Node Serving](kserve-multinode.md).

The CKodex `serving.ckodex.com/v1alpha2 LocalModelCache` is a compatibility API,
not the KServe resource with the same kind name. Migration to the upstream
KServe cache API must be explicit; the two controllers do not share status,
PVC ownership, or download-job configuration.

## Distributed prefill/decode and KV transfer

The operator materializes `spec.prefill` as a separate Deployment named
`<service>-prefill`. The primary Deployment is assigned `kv_consumer` and the
prefill Deployment is assigned `kv_producer`. A prefill block without a KV
connector is rejected by admission.

Example using LMCache:

```yaml
spec:
  kvCache:
    transfer:
      connector: lmcache
      extraConfig:
        chunk_size: "256"
        remote_url: "redis://lmcache.cache.svc:6379"
  prefill:
    replicas: 2
    template:
      spec:
        containers:
          - name: vllm-prefill
            image: registry.example/vllm@sha256:<validated-digest>
            resources:
              limits:
                nvidia.com/gpu: "1"
```

The connector is rendered into vLLM's `--kv-transfer-config` JSON. Supported
connectors are `nixl`, `lmcache`, and `mooncake`; connector-specific settings
remain in `extraConfig` so the CRD does not hard-code a backend version. Live
validation must still prove cache hits, transfer tail latency, and failover on
the target cluster before this feature is treated as production-ready.
