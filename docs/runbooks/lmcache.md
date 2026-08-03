# LMCache setup

CKodex exposes two LMCache paths under `spec.experimental.kvCache.transfer`.
Both cache attention key/value blocks; neither caches model weights. Use
`LocalModelCache` when the goal is to place model weights on nodes.

## In-process

Run the non-mutating renderer:

```bash
run/setup-lmcache.sh --mode inProcess --namespace inference
```

The typed block defaults to a 256-token chunk, local CPU caching enabled, and a
20 GiB local CPU cache. The operator emits the corresponding LMCache variables
and a `LMCacheConnectorV1` `--kv-transfer-config`. Values already declared in
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
  --size-gb 60 --apply --ack-privileged-namespace
```

The script downloads LMCache `operator-v0.1.1`, verifies the published
installer SHA-256, creates the namespaced `LMCacheEngine`, waits for Ready and
its `<engine>-connection` ConfigMap, then prints the matching CKodex fragment.
It does not create credentials. Multiprocess pods use `hostIPC: true`,
`PYTHONHASHSEED=0`, and the upstream ConfigMap value. The pinned upstream
operator has no mutating webhook, so injection is explicit and inspectable.

## Verify cache use

1. Confirm the pod contains `--kv-transfer-config`. In multiprocess mode, its
   value must come from `CKODEX_LMCACHE_KV_TRANSFER_CONFIG` and the referenced
   ConfigMap.
2. Send the same long-prefix request twice and compare LMCache lookup/hit
   counters and time-to-first-token. A running pod alone does not prove a hit.
3. For multiprocess mode, inspect `kubectl get lmcacheengine,configmap,pods -n
   <namespace>` and verify the engine has a local endpoint on every target
   node.

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
