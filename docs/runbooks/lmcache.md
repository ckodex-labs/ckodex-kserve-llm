# LMCache setup

CKodex exposes two LMCache paths under `spec.experimental.kvCache.transfer`.
Both cache attention key/value blocks; neither caches model weights. Use
`LocalModelCache` when the goal is to place model weights on nodes.

**Acceptance blocked:** the tested in-process configuration failed output parity
after an external reload, and the default multiprocess engine failed CUDA IPC
mapping. A local, unsigned same-UID engine image passed a bounded multiprocess
reload and post-restart test, but it is local, unsigned, and not a production
profile. See [the live verification record](../beta/lmcache-kind-verification-20261101.md).
Do not promote these configurations until the required parity and recovery checks pass.

## In-process

The tested unpatched v0.5.6 in-process build failed cache-reload parity; the
restriction is not enforced by admission or on already running workloads. A
local v0.5.6 backport passed the bounded CUDA and cache-reload checks recorded
in the [verification record](../beta/lmcache-kind-verification-20261101.md).
Its source patch and build guidance are under `hack/lmcache/`; it is unsigned
and not an upstream release. The pinned wheel can be assembled locally using:

```bash
dagger call build-lmcache-v-056-backport-diagnostic \
  --wheel /path/to/lmcache-0.5.6+ckodex.pr4467-cp312-cp312-linux_x86_64.whl \
  export --path /path/to/lmcache-v056-backport.oci.tar
```

This assembles an image from the specific tested wheel; it does not compile,
sign, publish, or establish source provenance for it. The wheel targets CUDA
compute capability 8.9 and retains the normal `LMCacheConnectorV1` selection.
An unmerged
development build passed bounded diagnostic cases but is not a v0.5.6
backport. Do not substitute it silently for the default runtime. Its exact
wheel can be assembled into a local diagnostic image with Dagger:

```bash
dagger call build-lmcache-pr-4467-diagnostic \
  --wheel /path/to/lmcache-0.0.0+pr4467.e05c2ea-cp312-cp312-linux_x86_64.whl \
  export --path /path/to/lmcache-diagnostic.oci.tar
```

The function checks the wheel SHA-256 before installing it with no dependency
resolution. The input wheel is not bundled or fetched by the function. This
reassembles an image from the exact tested binary; it does not verify its
source provenance, reproduce its compilation, sign it, publish it, or qualify
it for production. The wheel was compiled for CUDA compute capability 8.9.
An experimental workload must select that image by its own digest and use
vLLM's `LMCacheConnectorV1Dynamic` with module path
`lmcache.inprocess.integration.vllm.lmcache_connector_v1`; the operator's
default `LMCacheConnectorV1` does not start with this development build. See
the [verification record](../beta/lmcache-kind-verification-20261101.md) for
the tested configuration and limitations.

Run the non-mutating renderer:

```bash
run/setup-lmcache.sh --mode inProcess --namespace inference
```

The typed block defaults to a 256-token chunk, local CPU caching enabled, and a
20 GiB local CPU cache; `--size-gb` selects a different size. Rendering does
not verify the image, connector compatibility, or cache behavior. The operator
emits the corresponding LMCache variables and a `LMCacheConnectorV1`
`--kv-transfer-config`. Values already declared in
the pod template or `transfer.env` win. Removing the `lmcache` block returns to
the original low-level `extraConfig` and `env` contract.

## Multiprocess

Inspect the dry run first:

```bash
run/setup-lmcache.sh --mode multiprocess --namespace inference --engine shared-kv
```

Apply only to a non-production cluster after accepting the namespace security
change:

```bash
run/setup-lmcache.sh --mode multiprocess --namespace inference --engine shared-kv \
  --size-gb 60 --image-digest "${LMCACHE_IMAGE_DIGEST}" --apply --ack-privileged-namespace
```

Set `LMCACHE_IMAGE_DIGEST` to the `sha256:` digest for the intended
`lmcache/vllm-openai` image. The script does not select, resolve, or verify an
image digest, its signature, or its provenance; it requires the caller to
provide a digest before applying the multiprocess resource.

To use a different engine image, supply its repository and tag separately:

```bash
run/setup-lmcache.sh --mode multiprocess --namespace inference --engine shared-kv \
  --image-repository "${LMCACHE_IMAGE_REPOSITORY}" --image-tag "${LMCACHE_IMAGE_TAG}" \
  --image-digest "${LMCACHE_IMAGE_DIGEST}"
```

This example renders a dry run. The rendered image has the form
`repository:tag@sha256:digest`; the digest identifies the image and the tag
is retained for the upstream operator's image format. The repository must be
a lowercase OCI-style name without a tag or digest; a registry port is allowed.
The installer does not build, publish, load, or establish compatibility of the
image. A caller choosing the local diagnostic derivative must ensure it is
available on every target node and separately evaluate its UID, privileged
execution, host IPC, GPU behavior, provenance, and recovery. The script keeps
the default repository `lmcache/vllm-openai` and tag `latest` unless overridden.

Before changing the cluster, the script checks that RuntimeClass `nvidia`
declares handler `nvidia` and that at least one node labeled
`nvidia.com/gpu.present=true` reports an allocatable NVIDIA GPU. These are
declarations; a successful preflight does not prove the runtime handler or
GPU workload functions. The script does not configure the handler or node.

The script downloads LMCache `operator-v0.1.1`, verifies the published
installer SHA-256, creates the namespaced `LMCacheEngine`, waits up to ten
minutes for `ConfigValid`, `Available`, and `AllInstancesReady` to be `True` in
the same observation, with each condition and status observed at the current
engine generation. It then waits up to two minutes for its
`<engine>-connection` ConfigMap. With `jq`, it checks that the embedded JSON
specifies `LMCacheMPConnector`, `kv_both`, the expected engine service host,
and port `5555`, before printing the CKodex fragment. This validates the
connection configuration; it does not prove that a client can connect or that
cache operations succeed.

The pinned connection configuration uses `kv_both`. Multiprocess mode rejects
explicit producer or consumer roles and disaggregated prefill, because their
directional roles cannot be represented by this connection. Multiprocess mode
also rejects KServe multi-node workloads (`worker` or pipeline parallelism
greater than one), whose runtime path does not configure this connector. It
also reserves
`CKODEX_LMCACHE_KV_TRANSFER_CONFIG` and rejects an explicit
`--kv-transfer-config` in the runtime container so that it uses the referenced
engine connection. Nonempty `transfer.extraConfig` is rejected because this
mode does not apply those entries. These admission checks do not authenticate
the ConfigMap or prove that the GPU data path works.

It does not create credentials. Multiprocess pods use `hostIPC: true`,
`PYTHONHASHSEED=0`, and the upstream ConfigMap value. The pinned upstream
operator has no mutating webhook, so injection is explicit and inspectable.
The generated engine resource sets `LMCACHE_TRACK_USAGE=false` and
`DO_NOT_TRACK=1` as usage-reporting opt-outs. Verify that the chosen image
honors them; they do not establish whether an earlier instance sent data.

## Verify cache use

1. Confirm the pod contains `--kv-transfer-config`. In multiprocess mode, its
   value must come from `CKODEX_LMCACHE_KV_TRANSFER_CONFIG` and the referenced
   ConfigMap.
2. Run the [external-cache parity diagnostic](../../hack/kv-cache/README.md)
   against an isolated workload configured to force eviction. Check its token
   comparisons and external-hit increments against engine logs, then compare
   representative results with a no-LMCache control. Two identical requests or
   time-to-first-token alone do not prove external-cache retrieval or parity.
3. For multiprocess mode, inspect `kubectl get lmcacheengine,configmap,pods -n
   <namespace>` and verify the engine has a local endpoint on every target
   node.

## Upstream contract and recovery evidence

The [vLLM disaggregated-prefill guide](https://docs.vllm.ai/en/stable/features/disagg_prefill/)
defines separate prefill and decode instances connected by KV transfer, and
marks the feature experimental. Its role contract is directional: prefill is
the producer and decode is the consumer. The
[NIXL compatibility guide](https://docs.vllm.ai/en/latest/features/nixl_connector_compatibility/)
also requires compatible vLLM/connector versions, model layout, attention
backend, cache dtype, and transfer mode across the pair. The operator's
producer/consumer mapping is covered by deployment-builder tests; those tests
do not prove GPU transfer, accuracy, or recovery on a live cluster.

The pinned upstream operator-v0.1.1 publishes `<engine>-connection` with the
key `kv-transfer-config.json`. The installer checks its `LMCacheMPConnector`
payload, but that does not prove a live client connection or cache correctness.
Upstream LMCache v0.5.7 introduces non-root container defaults, unified API-key
authentication, native pygrpc/grpc.aio multiprocess execution, DeepSeek V4
compressed KV groups, and removal of the legacy TCP cache server; live cluster
acceptance of these features remains a separate qualification gate.

Model-weight recovery is separate. When a node leaves a CKodex
`LocalModelCache` target set, reconciliation deletes that node's warm-up Job and
PVC before removing its status entry. A deletion/read failure leaves the status
entry intact and retries, preventing the control plane from reporting cleanup
that did not occur. Invalid cache-size quantities fail before any PVC or Job is
created.

## Troubleshooting and rollback

- Injection is skipped when the typed block is absent, the connector is not
  `lmcache`, or no primary runtime container exists. Admission rejects an
  `engineRef` outside multiprocess mode and multiprocess mode without one.
- Match the vLLM and LMCache image line, CUDA runtime, driver, GPU architecture,
  Python ABI, and LMCache connector version. CUDA IPC mapping failures usually
  mean `hostIPC` is missing, `/dev/shm` was shadowed, or the image/driver ABI is
  incompatible.
- To roll back multiprocess mode, remove the typed block or switch to
  `inProcess`, apply the service, confirm pods no longer consume the connection
  ConfigMap, then delete the `LMCacheEngine`. Remove the privileged Pod Security
  label only after every hostIPC workload has left the namespace.
- Cache-hit, cache failover, and multi-node GPU behavior remain live-cluster
  acceptance gates. This runbook does not authorize production mutation.

Samples:
[`config/samples/serving_v1_llminferenceservice_lmcache_inprocess.yaml`](../../config/samples/serving_v1_llminferenceservice_lmcache_inprocess.yaml)
and
[`config/samples/serving_v1_llminferenceservice_lmcache_multiprocess.yaml`](../../config/samples/serving_v1_llminferenceservice_lmcache_multiprocess.yaml).
