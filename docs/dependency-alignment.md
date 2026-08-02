# Dependency alignment

This is the compatibility ledger for the operator's serving path. It records
the upstream versions checked on 2026-08-02 and separates shipped defaults from
optional infrastructure that the operator does not install or reconcile.

| Surface | Upstream version checked | Repository posture | Evidence |
|---|---|---|---|
| KServe | v0.19.0 | Serving substrate and storage initializer contract | [release](https://github.com/kserve/kserve/releases/tag/v0.19.0), [multi-node docs](https://kserve.github.io/website/docs/model-serving/generative-inference/multi-node) |
| Gateway API Inference Extension | v1.5.0 | GA `InferencePool` and digest-pinned EPP; CRD installed separately | [release](https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/tag/v1.5.0), [ADR-009](adr/009-kserve-019-and-llm-d-boundary.md) |
| vLLM | v0.25.1 | Default runtime image, selected explicitly for NVFP4 support | [release](https://github.com/vllm-project/vllm/releases/tag/v0.25.1) |
| Hugging Face Hub / Xet | `huggingface_hub==1.24.0`, `hf-xet==1.5.2` | Pinned in the published initializer image | [requirements](../build/huggingface-initializer-requirements.txt) |
| SeaweedFS | 4.40 | External S3/Filer target for `seaweedfs://`/`swfs://`; no chart install | [release](https://github.com/seaweedfs/seaweedfs/releases/tag/4.40) |
| LMCache | operator-v0.1.1 | Typed in-process configuration and optional upstream multiprocess `LMCacheEngine`; live acceptance tracked as L-OP-006 | [release](https://github.com/LMCache/LMCache/releases/tag/operator-v0.1.1), [runbook](runbooks/lmcache.md) |

## Compatibility rules

1. KServe `LocalModelCache` is distinct from the CKodex compatibility
   `LocalModelCache`; use KServe's `localModelDownloadJob` storage-container
   workload type for upstream cache jobs.
2. vLLM prefix caching is not LMCache. In-process mode uses
   `LMCacheConnectorV1`; multiprocess mode consumes the upstream
   `LMCacheEngine` connection ConfigMap.
3. SeaweedFS is a storage endpoint, not a runtime dependency of the operator.
   Upgrade the external SeaweedFS deployment only after its S3/Filer contract
   is exercised by the storage integration tests.
4. No automatic upgrade is inferred from a newer upstream tag. Runtime image
   changes require a compatibility test, a release artifact, and live serving
   evidence for the affected hardware profile.

The live cluster acceptance gates for HF/Xet downloads, Gateway traffic, and
multi-node NCCL serving remain separate from this documentation audit.
