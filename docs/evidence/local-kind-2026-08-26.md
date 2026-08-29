# Local KIND Acceptance Record — 2026-08-26

This record captures the local acceptance slice completed on the
`ckodex/reconcile-hardening-2026-08-26` branch. It is local evidence only. It
does not replace hosted CI, GPU, production PKI, durable audit-sink, or signed
release-artifact acceptance.

## Environment

| Item | Observed value |
| --- | --- |
| Kubernetes context | `kind-kserve-017` |
| KIND node image | `kindest/node:v1.35.0` |
| Kubernetes node | `kserve-017-control-plane` (`Ready`) |
| Host architecture | Apple Silicon / `linux/arm64` image build |
| Operator image | `ckodex/kserve-llm-operator:dev`, manifest `sha256:e98befa6c70db20a0b0fa38019b1fcdae68dbb2bd56a4748b9cce05520f818b4` |

The cluster was reused rather than created from an empty host. The evidence is
therefore a bounded local runtime acceptance, not a fresh-cluster reproducibility
claim.

## Pinned serving stack

| Component | Pin used by the local profile | Evidence |
| --- | --- | --- |
| KServe | `v0.19.0` | Helm release and local prerequisite script |
| Gateway API | `v1.5.1` | `standard-install.yaml` release asset; aligned with Envoy Gateway 1.8.x |
| Gateway API Inference Extension | `v1.5.0` | CRD manifest and EPP image |
| Envoy Gateway | `v1.8.1` | Helm release; chart digest `sha256:f46b2f38b695279fce81dced26d97724c3445fcccb0488aaa28ec5ef963a6181` |
| Envoy AI Gateway | `v1.0.0` | Helm release; chart digest `sha256:093b6b9caec92675e24dbe8a2825c1e908ea18fc28e66d46a7e6622aeb79c085` |
| EPP | `v1.5.0` digest-pinned image | `registry.k8s.io/gateway-api-inference-extension/epp@sha256:86c679b057298e68c6e65ff5603e92066d432e77b11f1f81f0a06399694810bc` |
| vLLM CPU | `v0.25.1` | `vllm/vllm-openai-cpu:v0.25.1`, manifest `sha256:6b301f040db8152dfb8ff55e06fd348aa5d0d9a311f58118160c7058262c8628` |

The version relationship is recorded in [dependency-alignment.md](../dependency-alignment.md)
and [COMPONENTS.md](../../COMPONENTS.md). The local installer is
[local/02-prereqs.sh](../../local/02-prereqs.sh).

## Changes exercised

1. EPP transport was aligned with Envoy AI Gateway's generated endpoint-picker
   cluster: the EPP runs `--secure-serving`, and its Service declares
   `appProtocol: http2`. The proxy's endpoint-picker cluster showed HTTP/2 over
   TLS, and the request reached the vLLM endpoint without the prior TLS/plaintext
   protocol failure.
2. Gateway, HTTPRoute, EPP Service, and EPP Deployment reconciliation now skips
   unchanged writes and uses merge patches for deltas. An 8-second live check
   held HTTPRoute `resourceVersion/generation` at `30364/426 -> 30364/426`, with
   no current `Operation cannot be fulfilled` reconcile errors.
3. The HTTPRoute contract now includes `/v1/completions` for standard and canary
   routes. The GPT-2 fixture uses this endpoint because GPT-2 has no model chat
   template; chat-capable models retain `/v1/chat/completions`.
4. The active `deploy/helm` manager chart mounts its configured audit path on a
   writable `emptyDir` for local runs and can select a PVC when persistence is
   enabled. The manager retains a read-only root filesystem.
5. GoReleaser now compiles `cmd/storage-initializer` as a package, so the new
   transaction/recovery helpers are present in release binaries.

## Acceptance trace

The maintained command below exited `0` twice after the final code/image
reload:

```text
bash local/05-test-inference.sh
Serving via Gateway at http://172.19.0.200
Gateway address is not reachable from this host; using a port-forward to the Gateway proxy
{
  "object": "text_completion",
  "model": "gpt2",
  "choices": [{"index": 0, "finish_reason": "length"}],
  "usage": {"prompt_tokens": 8, "completion_tokens": 50, "total_tokens": 58}
}
```

The direct MetalLB address was unreachable from the Docker Desktop host; the
probe's maintained proxy-service port-forward fallback preserved the declared
`Host: llama3-8b.ckodex.com` header. This is an environment transport boundary,
not a successful external LoadBalancer test.

After the live probe, the source-only ownership ordering correction was covered
by the final focused tests, full race suite, and vet run. A byte-identical image
reload was not repeated because Docker became unavailable while a pre-existing
Dagger job was using the shared engine; the correction affects persistence of a
missing owner reference and not the already-owned live workload.

The live control-plane assertions were:

```text
LLMInferenceService Ready=True, SchedulerReady=True
Gateway Accepted=True, Programmed=True
HTTPRoute Accepted=True, ResolvedRefs=True
InferencePool Accepted=True, ResolvedRefs=True
EPP args include --secure-serving
EPP Service appProtocol=http2
```

## Verification results

| Check | Result |
| --- | --- |
| `go test -race ./...` | PASS |
| `go vet -a ./...` | PASS |
| `go test ./internal/gateway ./internal/scheduler` | PASS |
| `go run ./hack/helm-contract` | PASS — `helm install contracts passed` |
| `helm lint deploy/helm` | PASS |
| `make release-readiness` | PASS — GoReleaser archives, checksums, CRD bundle, Helm package, and packaged image tags verified |
| `git diff --check` | PASS |
| `dagger call lint --source=.` | INCONCLUSIVE — stopped after more than 8 minutes; a pre-existing long-running Dagger job was using the shared local engine and telemetry timed out |
| `dagger call test --source=.` | INCONCLUSIVE — stopped after more than 8 minutes; same shared-engine contention |

The first release rehearsal failure was retained as a finding rather than
discarded: GoReleaser reported undefined transaction helpers because its
initializer build target named `main.go` instead of the package. The corrected
rehearsal passed.

## C/S/A boundary

- **C — local:** the maintained CPU fixture initialized through `hf://`, became
  ready, passed through Envoy Gateway + InferencePool + EPP, and returned a
  validated OpenAI-compatible completion response.
- **C — local:** route/resource reconciliation is covered by focused tests and
  the live unchanged-resource observation.
- **S — production EPP trust:** the local EPP uses an ephemeral self-signed
  certificate, while the Envoy AI Gateway v1.0.0 cluster accepts untrusted
  certificates. Production CA distribution, rotation, and trust verification
  remain open.
- **S — audit durability:** local audit events are emitted to structured logs
  and Kubernetes Events; the default local volume is ephemeral. Durable sink
  retention and evidence verification remain open.
- **S — release/hosted:** hosted exact-head CI, published chart/image digests,
  SBOM/provenance/signature verification, fresh-cluster acceptance, GPU
  acceptance, and external storage/cache acceptance remain separate gates.

## Upstream references consulted

- [Envoy AI Gateway HTTPRoute + InferencePool guide](https://aigateway.envoyproxy.io/docs/capabilities/inference/httproute-inferencepool/)
- [Envoy AI Gateway v1.0.0 release](https://github.com/envoyproxy/ai-gateway/releases/tag/v1.0.0)
- [Envoy AI Gateway InferencePool addon values](https://raw.githubusercontent.com/envoyproxy/ai-gateway/main/examples/inference-pool/envoy-gateway-values-addon.yaml)
- [Gateway API Inference Extension v1 API reference](https://gateway-api-inference-extension.sigs.k8s.io/reference/spec/)
- [Envoy Gateway compatibility matrix](https://gateway.envoyproxy.io/news/releases/matrix/)
- [vLLM CPU installation guidance](https://docs.vllm.ai/en/latest/getting_started/installation/cpu/)
