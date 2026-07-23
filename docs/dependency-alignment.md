# Dependency alignment

This is the compatibility ledger for the operator's serving path. It records
the upstream versions checked on 2026-07-23 and separates shipped defaults from
optional infrastructure that the operator does not install or reconcile.

| Surface | Upstream version checked | Repository posture | Evidence |
|---|---|---|---|
| KServe | v0.19.0 | Serving substrate and storage initializer contract | [release](https://github.com/kserve/kserve/releases/tag/v0.19.0), [multi-node docs](https://kserve.github.io/website/docs/model-serving/generative-inference/multi-node) |
| llm-d | v0.8.1 | Router EPP image only; not full llm-d deployment | [release](https://github.com/llm-d/llm-d/releases/tag/v0.8.1), [ADR-009](adr/009-kserve-019-and-llm-d-boundary.md) |
| vLLM | v0.25.1 | Default runtime image, selected explicitly for NVFP4 support | [release](https://github.com/vllm-project/vllm/releases/tag/v0.25.1) |
| Hugging Face Hub / Xet | `huggingface_hub==1.24.0`, `hf-xet==1.5.2` | Pinned in the published initializer image | [requirements](../build/huggingface-initializer-requirements.txt) |
| SeaweedFS | 4.40 | External S3/Filer target for `seaweedfs://`/`swfs://`; no chart install | [release](https://github.com/seaweedfs/seaweedfs/releases/tag/4.40) |
| LMCache | operator-v0.5.1 | Not installed or reconciled; tracked as L-OP-006 | [release](https://github.com/LMCache/LMCache/releases/tag/operator-v0.5.1), [KServe KV-cache guide](https://kserve.github.io/website/docs/model-serving/generative-inference/kvcache-offloading) |

## Compatibility rules

1. KServe `LocalModelCache` is distinct from the CKodex compatibility
   `LocalModelCache`; use KServe's `localModelDownloadJob` storage-container
   workload type for upstream cache jobs.
2. vLLM prefix caching is not LMCache. LMCache requires a vLLM
   `LMCacheConnectorV1` configuration and an independently operated backend.
3. SeaweedFS is a storage endpoint, not a runtime dependency of the operator.
   Upgrade the external SeaweedFS deployment only after its S3/Filer contract
   is exercised by the storage integration tests.
4. No automatic upgrade is inferred from a newer upstream tag. Runtime image
   changes require a compatibility test, a release artifact, and live serving
   evidence for the affected hardware profile.

The live cluster acceptance gates for HF/Xet downloads, Gateway traffic, and
multi-node NCCL serving remain separate from this documentation audit.
