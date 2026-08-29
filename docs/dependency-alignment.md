# Dependency alignment

This is the compatibility ledger for the operator's serving path. It records
the upstream versions checked on 2026-08-27 and separates shipped defaults from
optional infrastructure that the operator does not install or reconcile.

| Surface | Upstream version checked | Repository posture | Evidence |
|---|---|---|---|
| KServe | v0.20.0 | Serving substrate and storage initializer contract | [release](https://github.com/kserve/kserve/releases/tag/v0.20.0), [multi-node docs](https://kserve.github.io/website/docs/model-serving/generative-inference/multi-node) |
| Gateway API | v1.6.1 latest; v1.5.1 local profile | Go client library is current at v1.6.1; the installed CRD bundle remains v1.5.1 for the tested Envoy/AI Gateway profile | [latest release](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.6.1), [local profile](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.5.1), [compatibility matrix](https://gateway.envoyproxy.io/news/releases/matrix/) |
| Gateway API Inference Extension | v1.5.0 | GA `InferencePool` CRDs; EPP executable is supplied by llm-d Router | [release](https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/tag/v1.5.0), [ADR-009](adr/009-kserve-019-and-llm-d-boundary.md) |
| Envoy Gateway | v1.8.1 | Gateway API controller and proxy fleet; InferencePool extension enabled by values overlay | [release](https://github.com/envoyproxy/gateway/releases/tag/v1.8.1), [compatibility matrix](https://gateway.envoyproxy.io/news/releases/matrix/) |
| Envoy AI Gateway | v1.1.0 | InferencePool extension manager and AI routing integration | [release](https://github.com/envoyproxy/ai-gateway/releases/tag/v1.1.0), [HTTPRoute + InferencePool](https://aigateway.envoyproxy.io/docs/capabilities/inference/httproute-inferencepool/) |
| llm-d core | v0.9.0 | Compatibility baseline for the Router family | [release](https://github.com/llm-d/llm-d/releases/tag/v0.9.0) |
| llm-d Router EPP | v0.10.0 | Executable endpoint picker, pinned by OCI manifest digest | [release](https://github.com/llm-d/llm-d-router/releases/tag/v0.10.0), [release manifests](https://github.com/llm-d/llm-d-router/releases/download/v0.10.0/manifests.yaml) |
| vLLM | v0.28.0 | Default runtime image, selected explicitly for NVFP4 support | [release](https://github.com/vllm-project/vllm/releases/tag/v0.28.0) |
| Hugging Face Hub / Xet | `huggingface-hub==1.28.0`, `hf-xet==1.6.0` | Hash-pinned in the published initializer image | [requirements](../build/huggingface-initializer-requirements.txt) |
| SeaweedFS | 4.40 | External S3/Filer target for `seaweedfs://`/`swfs://`; no chart install | [release](https://github.com/seaweedfs/seaweedfs/releases/tag/4.40) |
| LMCache | operator-v0.1.1 | Typed in-process configuration and optional upstream multiprocess `LMCacheEngine`; live acceptance tracked as L-OP-006 | [release](https://github.com/LMCache/LMCache/releases/tag/operator-v0.1.1), [runbook](runbooks/lmcache.md) |

## Compatibility rules

1. KServe `LocalModelCache` is distinct from the CKodex compatibility
   `LocalModelCache`; use KServe's `localModelDownloadJob` storage-container
   workload type for upstream cache jobs.
2. llm-d Router `v0.10.0` consumes the new `llm-d.ai/v1alpha1` EPP config
   contract. The operator emits that version and keeps the deprecated
   `inference.networking.x-k8s.io/v1alpha1` RBAC during the transition.
3. GIE `v1.5.0` remains installed for the GA `InferencePool` CRD and the old
   request-policy API. GIE `v1.6.0` is not a drop-in upgrade: its release moves
   the full EPP implementation to llm-d Router and removes the alpha EPP
   configuration resources used by the old image.
4. The llm-d Router `v0.10.0` release requires the EPP and disaggregation
   sidecar to be upgraded together when KV-transfer is enabled. This operator
   does not own that sidecar, so it does not claim disaggregated-serving
   compatibility from the EPP image bump alone.
5. vLLM prefix caching is not LMCache. In-process mode uses
   `LMCacheConnectorV1`; multiprocess mode consumes the upstream
   `LMCacheEngine` connection ConfigMap.
6. SeaweedFS is a storage endpoint, not a runtime dependency of the operator.
   Upgrade the external SeaweedFS deployment only after its S3/Filer contract
   is exercised by the storage integration tests.
7. No automatic upgrade is inferred from a newer upstream tag. Runtime image
   changes require a compatibility test, a release artifact, and live serving
   evidence for the affected hardware profile.
8. The local EPP uses TLS-backed HTTP/2 because Envoy AI Gateway's InferencePool
   extension constructs the endpoint-picker cluster with TLS and HTTP/2. The
   local EPP uses an ephemeral self-signed certificate; production trust
   distribution and certificate rotation remain an acceptance gate.

The live cluster acceptance gates for HF/Xet downloads, Gateway traffic, and
multi-node NCCL serving remain separate from this documentation audit.
