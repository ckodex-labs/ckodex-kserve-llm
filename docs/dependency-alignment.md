# Dependency alignment

This ledger records the release set selected on 2026-11-05. Upstream release
numbers belong to separate projects and do not form one shared version track.
The vLLM workload and adapter now use one vLLM release; KServe, Gateway API,
GIE, and llm-d Router retain their own compatible release versions.

| Surface | Selected version | Repository posture | Evidence |
|---|---|---|---|
| KServe | v0.21.0 | Local install uses the core CRD chart and resource chart; the release split CRDs into independent charts | [release](https://github.com/kserve/kserve/releases/tag/v0.21.0), [chart structure](https://kserve.github.io/website/docs/install/overview) |
| KServe storage initializer | v0.21.0 | Multi-architecture index pinned by digest for standard model downloads | [release](https://github.com/kserve/kserve/releases/tag/v0.21.0) |
| Gateway API | v1.6.2 Go library; v1.6.2 local CRDs | Standard bundle aligned with Envoy Gateway v1.9.2 and Agent Router v1.2.0 | [library release](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.6.2), [local bundle](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.6.2), [Envoy matrix](https://gateway.envoyproxy.io/news/releases/matrix/) |
| Gateway API Inference Extension | v1.6.2 | Supplies the GA InferencePool API; Router APIs are installed from llm-d Router | [release](https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/tag/v1.6.2) |
| Envoy Gateway | v1.9.2 | Local controller profile; coupled with Agent Router v1.2.0 | [release notes](https://gateway.envoyproxy.io/news/releases/notes/v1.9.2/), [compatibility matrix](https://gateway.envoyproxy.io/news/releases/matrix/) |
| Agent Router (formerly Envoy AI Gateway) | v1.2.0 | InferencePool extension manager and AI routing integration (candidate v1.2.1-rc2) | [release](https://github.com/theagentrouter/agent-router/releases/tag/v1.2.0) |
| llm-d core | v1.0.0 | Organization-wide unified v1.0.0 release | [release](https://github.com/llm-d/llm-d/releases/tag/v1.0.0) |
| llm-d Router EPP | v1.0.0 | Operator image pinned to the multi-architecture index digest; EPP config uses llm-d.ai/v1 | [release](https://github.com/llm-d/llm-d-router/releases/tag/v1.0.0), [chart default config](https://github.com/llm-d/llm-d-router/blob/v1.0.0/config/charts/routerlib/templates/_config.yaml) |
| vLLM runtime and adapter | v0.33.0 | One digest-pinned CUDA 13.0 image contract for both default and adapter; CUDA 12.9 dropped | [release](https://github.com/vllm-project/vllm/releases/tag/v0.33.0), [acceptance runbook](runbooks/vllm-v0.33-qualification.md) |
| vLLM CPU image | v0.33.0 | Multi-architecture CPU image index, pinned separately from the CUDA image | [CPU install docs](https://docs.vllm.ai/en/v0.33.0/getting_started/installation/cpu/) |
| Hugging Face Hub / Xet | huggingface-hub==1.28.0, hf-xet==1.6.0 | Hash-pinned in the published initializer image | [requirements](../build/huggingface-initializer-requirements.txt) |
| SeaweedFS | 4.47 | External S3/Filer target for seaweedfs:// and swfs://; no chart install | [release](https://github.com/seaweedfs/seaweedfs/releases/tag/4.47) |
| LMCache | v0.5.7 engine / operator-v0.1.1 | Non-root containers, unified API key auth, native pygrpc/grpc.aio, legacy TCP cache server removed; live acceptance tracked as L-OP-006 | [release](https://github.com/LMCache/LMCache/releases/tag/v0.5.7), [runbook](runbooks/lmcache.md) |

## What was checked

- KServe v0.21.0 is the latest stable release shown by the upstream release
  page. Its CRDs are split into kserve-crd, kserve-llmisvc-crd, and
  kserve-localmodel-crd charts. This operator uses KServe core InferenceService
  APIs and installs the core CRDs and controller resources; it does not install
  the upstream LLMInferenceService or LocalModel resources. CKODEX's own
  serving.ckodex.com/v1 resource remains a separate API and its stable storage
  version is unchanged.
- The KServe v0.21.0 kserve-crd and kserve-resources charts were fetched from
  OCI by immutable digest and rendered locally. The core CRD chart rendered six
  CRDs, and the resources chart rendered the kserve-controller-manager
  Deployment with the selected Standard/Gateway API values. The installer pins
  chart digests sha256:36385f02ef5723910c83e2e04aea6892a3a54e7bdcdd88636ef7adbf7dff61f0
  and sha256:2d189c8c9d51e1ad658f9a973eaf9ea73b251305e7f93507adb1b559303f1e9a.
- The vLLM v0.33.0 CUDA image index digest is
  sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
  and resolves to linux/amd64 and linux/arm64. The CPU image index digest is
  sha256:87167cf0ec6e1e9607bb1cf9bf618a67d04963236a34bd955ac75b7d31a6edf5
  and resolves to the same architectures.
- The KServe storage initializer v0.21.0 image index digest is
  sha256:3a3a6a8053478c8d8d90f96fc5e54c1892c07784102dda70104374d4f89a3672
  and resolves to linux/amd64 and linux/arm64.
- The llm-d Router v1.0.0 EPP image index digest is
  sha256:024188e998f516879f68532016ca24fabca00afbdcc2af9a61b17634e197df2c
  and resolves to linux/amd64 and linux/arm64.
- Router v1.0.0 promotes EndpointPickerConfig to llm-d.ai/v1. The default
  ConfigMap now follows the Router chart's standard queue, KV-utilization, and
  prefix-cache scoring pipeline. Inline CKODEX
  weights keep their existing string-shaped API for stored-object compatibility
  and are emitted as numbers at the EPP boundary. ConfigMap references remain
  pass-through and should use the v1 field placement.
- GIE v1.6.2 continues to provide GA InferencePool. Router v1.0.0 provides
  the llm-d.ai request-policy CRDs. The operator role does not need to watch
  those objects; the pre-provisioned EPP Role grants read-only access to the
  llm-d.ai resources and no longer grants access to request-policy objects in
  inference.networking.x-k8s.io. Applying the new manifests does not delete
  older CRDs or migrate their objects; existing clusters must migrate those
  objects before removing the legacy APIs.

## Remaining dependency audit — 2026-11-05

This pass checked active direct pins in the bootstrap scripts, runtime
adapters, image builders, and optional integration paths. It did not audit
every transitive module or inspect live externally managed clusters.

| Surface | Active repository pin | Current upstream / finding | Disposition |
|---|---|---|---|
| TensorFold | Adapter points to `tensorfold/tensorfold:v1.0.0@sha256:d8e5f2a1b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0`; Helm declares `ghcr.io/ckodex-labs/tensorfold-runtime:v0.31.0`; `COMPONENTS.md` previously declared `tensorfold/engine:latest` | The registry adapter overwrites the non-vLLM image from Helm, and public artifact/provenance evidence for the adapter reference is unresolved. | **Blocked:** resolve to one verified image identity and digest before claiming TensorFold support. |
| Gateway API / Envoy / Agent Router | Go module `v1.6.2`; installed CRDs `v1.6.2`; Envoy Gateway `v1.9.2`; Agent Router `v1.2.0`; GIE `v1.6.2` | Agent Router `v1.2.0` targets Envoy Gateway `v1.9.2`, Gateway API `v1.6.2`, and GIE `v1.6.2`. Candidate `v1.2.1-rc2` is under evaluation. | **Aligned stack:** Gateway API Go module and CRDs `v1.6.2` with Envoy Gateway `v1.9.2` and Agent Router `v1.2.0`; retain GIE `v1.6.2`. |
| cert-manager / MetalLB | `v1.21.2` / `v0.14.9` | cert-manager `v1.21.2` release says all users should upgrade; MetalLB `v0.16.0` is available. | **Update candidates:** refresh local bootstrap pins and smoke-test webhook/certificate issuance and KIND L2 address allocation. |
| KIND / Kubernetes | KIND `v0.33.0`; node image `v1.35.8` | KIND `v0.33.0` remains current. The Envoy Gateway v1.9 matrix covers Kubernetes `v1.33` through `v1.36`; KIND publishes a digest-pinned `v1.36.4` node image. | **Keep KIND; candidate node image:** move to Kubernetes `v1.36.4` only with the coupled Gateway stack qualification. |
| Go / Dagger / Trivy | `go.mod`: Go `1.26.0`, toolchain `1.26.7`; Docker builder Go `1.26.9`; Dagger Engine `v0.21.9`; Trivy `v0.72.0` | Go `1.27.2`, Dagger `v0.21.10`, and Trivy `v0.76.0` are current upstream releases. Cosign `v3.1.3` and Dagger GitHub Action `v8.4.1` remain current. | **Align build pins:** bring Go module/toolchain/builder onto one selected toolchain; update Dagger and Trivy only through the complete Dagger gate, including scanner DB and output review. Keep Cosign and the GitHub Action. |
| SPIRE / policy / mesh | SPIRE defaults `v1.15.4`; Helper `v0.11.0`; external targets Gatekeeper `v3.23.1`, Istio `1.30.6` | SPIRE `v1.15.4`, Helper `v0.12.2`, Gatekeeper `v3.23.1`, and Istio `1.30.6` are available. Istio `1.30.6` includes security fixes, including an HTTP/2 AuthorizationPolicy method-case bypass fix. | **Prioritize review:** align SPIRE Agent and Server together and verify SVID issuance/rotation. Update external Gatekeeper/Istio targets and confirm managed-cluster versions; this chart does not install them. |
| SGLang / Hugging Face | SGLang `v0.5.22`; Hub `1.28.0`; Xet `1.6.0`; optional HF CSI `v0.15.1` | SGLang `v0.5.22`, Hub `2.2.0` (latest v1 line `1.33.0`), Xet `1.7.0`, and CSI `v0.15.1` are available. Hub 2.x is a breaking HTTP-client migration; SGLang v0.5.19 was the final CUDA 12 image release. | **Qualify separately:** update package hashes and initializer image for Hub/Xet; validate target CUDA lane for SGLang; validate privileged FUSE mount/recovery behavior for CSI. |
| SeaweedFS / Vector | Local SeaweedFS manifest `4.47`; optional Vector sidecar `0.59.0` (tag only) | SeaweedFS `4.47` and Vector `0.59.0` are updated. Vector 0.59 lists six breaking changes and two security changes. The repository uses an HTTP implementation and AWS SDK for S3; it has no SeaweedFS Go SDK dependency. | **Update candidates:** test local S3/Filer operations against SeaweedFS 4.47; validate generated Vector config and telemetry delivery, then pin the Vector image by digest. |
| LMCache | Engine `v0.5.7`; optional upstream operator `operator-v0.1.1` | LMCache v0.5.7 introduces non-root container defaults, unified API-key auth, native pygrpc/grpc.aio, and removes legacy TCP server. | **Updated:** aligned with v0.5.7 engine and vLLM v0.33 connector behavior. |

The Gateway API v1.6.2 release is aligned with Envoy Gateway v1.9.2 and
Agent Router v1.2.0 across both Go API and installed CRDs.

The Agent Router v1.2.0 release retains the `ai-gateway` chart names and API
group. It also changes security behavior and MCP session encryption defaults;
review the release guidance for any applicable MCP configuration before an
upgrade. This operator's current release review has not demonstrated a live
Gateway-to-EPP-to-model request on the candidate stack.

Upstream references: [Gateway API v1.6.2](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.6.2),
[Envoy Gateway compatibility matrix](https://gateway.envoyproxy.io/news/releases/matrix/),
[Agent Router v1.2.0 upgrade guide](https://github.com/theagentrouter/agent-router/releases/tag/v1.2.0),
[cert-manager v1.21.2](https://github.com/cert-manager/cert-manager/releases/tag/v1.21.2),
[MetalLB v0.16.0](https://github.com/metallb/metallb/releases/tag/v0.16.0),
[Go release history](https://go.dev/doc/devel/release),
[Dagger v0.21.10](https://github.com/dagger/dagger/releases/tag/v0.21.10),
[Trivy v0.76.0](https://github.com/aquasecurity/trivy/releases/tag/v0.76.0),
[SPIRE v1.15.4](https://github.com/spiffe/spire/releases/tag/v1.15.4),
[Gatekeeper v3.23.1](https://github.com/open-policy-agent/gatekeeper/releases/tag/v3.23.1),
[Istio v1.30.6](https://istio.io/latest/news/releases/1.30.x/announcing-1.30.6/),
[SGLang v0.5.22](https://github.com/sgl-project/sglang/releases/tag/v0.5.22),
[Hugging Face Hub releases](https://github.com/huggingface/huggingface_hub/releases),
[hf-xet v1.7.0](https://github.com/huggingface/xet-core/releases/tag/v1.7.0),
[HF CSI v0.15.1](https://github.com/huggingface/hf-csi-driver/releases/tag/v0.15.1),
[SeaweedFS 4.47](https://github.com/seaweedfs/seaweedfs/releases/tag/4.47), and
[Vector v0.59.0 upgrade guide](https://vector.dev/highlights/2026-10-05-0-59-0-upgrade-guide/).

## Release-track distinction

LLM-D components are unified under the v1.0.0 organization-wide alignment. The
operator pins Router v1.0.0 directly as it carries the llm-d.ai/v1 config
contract. Hosted cross-component acceptance on a production cluster remains
open.

The local Gateway profile aligns Gateway API CRDs v1.6.2, Go library v1.6.2,
Envoy Gateway v1.9.2, and Agent Router v1.2.0.

## Runtime acceptance boundary

The active vLLM default is v0.33.0 and uses the upstream CUDA 13.0 image index
(CUDA 12.9 dropped). The adapter contract, default image, samples, and CPU
profiles now target that same upstream release. CPU images use the matching
v0.33.0 release with the multi-architecture CPU index.

The v0.31.0 and v0.32.0 version-specific conformance vectors and TensorFold
upgrade procedures are historical records. They are not active vLLM image pins
and do not serve as v0.33.0 runtime evidence.

The image digest and release metadata are verified, but this checkout contains
no v0.33.0 GPU startup, model inference, node-driver compatibility, or recovery
receipt. Treat those hardware/runtime results as unknown until the acceptance
runbook is executed. The v0.33.0 image's CUDA 13.0 base requires a compatible
host driver. The optional v0.33.0 qualification sample adds the explicit
Secret-backed API key and existing-TLS-Gateway conditions for that acceptance
path.

## Compatibility rules

1. KServe LocalModelCache is distinct from the CKODEX compatibility
   LocalModelCache; use the appropriate API group for each resource.
2. The operator emits default inline EPP config as llm-d.ai/v1. User-managed
   ConfigMap references should also use v1 and migrate the changed dataLayer
   fields described in the [Router v1 docs](https://github.com/llm-d/llm-d-router/blob/v1.0.0/docs/architecture.md).
3. Legacy InferenceObjective or InferenceModelRewrite objects in
   inference.networking.x-k8s.io are not watched by the v1 Router EPP. The
   local installer leaves existing CRDs in place; migrate data before removing
   legacy APIs.
4. The Router v1.0.0 image upgrade does not prove compatibility with a
   disaggregation sidecar that this operator does not own. Keep KV-transfer
   acceptance separate.
5. vLLM prefix caching is not LMCache. In-process mode uses
   LMCacheConnectorV1; multiprocess mode consumes the upstream
   LMCacheEngine connection ConfigMap.
6. SeaweedFS is an external storage endpoint, not a runtime dependency of the
   operator.
7. A release tag or rendered CRD proves neither hosted cross-component
   compatibility nor hardware runtime behavior. Keep those gates separate.
8. The local EPP uses TLS-backed HTTP/2 because Envoy AI Gateway's InferencePool
   extension constructs the endpoint-picker cluster with TLS and HTTP/2. The
   EPP uses an ephemeral self-signed certificate; production trust distribution
   and certificate rotation remain acceptance gates.

The live gates for HF/Xet downloads, Gateway traffic, vLLM CUDA drivers,
Router/EPP reconciliation, and multi-node NCCL serving remain separate from
this release and manifest review.
