# Dependency refresh evidence — 2026-08-27

## Scope and verdict

This record captures the LLM-D-focused dependency refresh and the coordinated
language, frontend, Python, Kubernetes, CI, and observability dependency
updates applied to the working tree. It is a local source-and-contract
verification record, not a hosted release or production acceptance claim.

**Verdict:** the executable scheduler path is migrated to llm-d Router EPP
`v0.10.0`, pinned by OCI manifest digest. The GIE `v1.5.0` CRD bundle remains
required for the GA `InferencePool` API. The GIE `v1.6.0` bundle is not a
drop-in replacement for the current scheduler contract.

## Upstream evidence consulted

| Surface | Release checked | Relevant evidence |
|---|---|---|
| llm-d core | `v0.9.0` | [release](https://github.com/llm-d/llm-d/releases/tag/v0.9.0) |
| llm-d Router | `v0.10.0` | [release](https://github.com/llm-d/llm-d-router/releases/tag/v0.10.0), [release manifests](https://github.com/llm-d/llm-d-router/releases/download/v0.10.0/manifests.yaml) |
| Gateway API Inference Extension | `v1.6.0` latest checked; `v1.5.0` selected | [v1.6.0 release](https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/tag/v1.6.0), [v1.5.0 release](https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/tag/v1.5.0) |
| Envoy AI Gateway | `v1.1.0` | [release](https://github.com/envoyproxy/ai-gateway/releases/tag/v1.1.0), [InferencePool guide](https://aigateway.envoyproxy.io/docs/capabilities/inference/httproute-inferencepool/) |

## Selected pins and repository wiring

| Contract | Selected pin | Source of truth in this checkout |
|---|---|---|
| Router EPP image | `ghcr.io/llm-d/llm-d-router-endpoint-picker@sha256:2e516fa1310da7be59b82beb1445362139597d6d553ef04d546716abe3aaaa70` | `internal/scheduler/epp_manager.go`, `internal/config/operator_config.go`, `deploy/helm/values.yaml` |
| Router EPP config | `llm-d.ai/v1alpha1/EndpointPickerConfig` | `internal/scheduler/config.go`, `internal/scheduler/config_test.go` |
| Router policy CRDs | release manifest `v0.10.0` | `local/02-prereqs.sh` |
| InferencePool CRD | GIE release manifest `v1.5.0` | `local/02-prereqs.sh` |
| EPP identity | namespace-scoped pre-provisioned `ckodex-epp` with read-only policy access | `deploy/helm/templates/epp-rbac.yaml`, `deploy/helm/templates/rbac.yaml` |
| Go and Kubernetes client graph | Go `v1.27.0`, Kubernetes modules `v0.36.4`, controller-runtime `v0.24.1` | `go.mod`, `go.sum` |
| OpenTelemetry graph | OTel `v1.46.0`, contrib `v0.71.0`, log `v0.22.0`, OTLP `v1.11.0` | `go.mod`, `go.sum` |
| Console dependency graph | lockfile-resolved compatible updates; production audit clean | `console/package.json`, `console/package-lock.json` |
| Initializer dependency graph | Python 3.12-compatible hash-pinned requirements | `build/huggingface-initializer-requirements.txt` |

The image digest was resolved from the public GHCR OCI manifest index for the
`v0.10.0` tag. The release source confirms that the Router retains the EPP
flags used by this operator, including secure serving, `9002` ext-proc, and
`9003` gRPC health. The Router release also documents a required coordinated
upgrade of the EPP and disaggregation sidecar when KV-transfer is enabled; the
sidecar is outside this operator's ownership boundary.

## Compatibility decisions

1. The operator now emits `llm-d.ai/v1alpha1`, the Router's current config API.
   The Router still decodes the old `inference.networking.x-k8s.io/v1alpha1`
   form as deprecated, so old referenced ConfigMaps remain readable during
   rollout. New generated configurations no longer use the deprecated form.
2. GIE `v1.5.0` remains installed because it supplies the GA `InferencePool`
   CRD used by the Envoy Gateway integration. Router `v0.10.0` supplies the
   EPP implementation and the `llm-d.ai` request-policy CRDs.
3. GIE `v1.6.0` is held. Its release moves the full EPP implementation into
   llm-d Router and removes alpha request-policy/configuration resources; a
   blind CRD/image replacement would leave the current operator contract
   unverified.
4. The local Envoy AI Gateway profile keeps TLS-backed HTTP/2 for the EPP
   service. This is a deliberate profile choice; the upstream Router chart's
   default service is h2c for deployments that do not enable secure serving.
5. The root Go Gateway API library remains checked at `v1.6.1`, while the
   installed local CRD bundle remains `v1.5.1` to match the tested Envoy
   Gateway `v1.8.1` and AI Gateway `v1.1.0` profile. This split is documented
   rather than hidden as an accidental version mismatch.

## Verification matrix

| Gate | Status | Evidence or remaining boundary |
|---|---|---|
| Source pins are internally consistent | `C` | EPP image, defaults, Helm values, config API, RBAC, and prerequisite manifests updated together |
| Router image tag resolves to an immutable digest | `C` | GHCR OCI `Docker-Content-Digest` lookup recorded above |
| Local unit/contract tests | `C` | `go test -race ./...`, `go vet -a ./...`, `make manifests`, `go run ./hack/helm-contract`, and `helm lint` passed after the final graph refresh |
| Console dependency/build gates | `C` | `npm ci --ignore-scripts`, `npm audit --omit=dev --audit-level=high`, `npm test` (43/43), `npm run lint`, `npx tsc --noEmit`, `npm run build`, `npm run verify:ssr`, and `npm run verify:populated` passed |
| Initializer dependency gate | `C` | Temporary Python 3.12 virtual environment installed the requirements with `--require-hashes`; `pip check` passed |
| Release artifact rehearsal | `C` | `make release-readiness` passed GoReleaser snapshot archives, CRD checksum, Helm packaging, and image-tag contract checks |
| Documentation integrity | `C` | Markdown lint passed for the modified Markdown surfaces; `git diff --check` passed |
| Fresh KIND prerequisite and scheduler readiness | `pending` | Requires Docker/KIND availability; historical local proof remains in `local-kind-2026-08-26.md` |
| Hosted CI exact-head gate | `pending` | Requires hosted GitHub Actions execution and green Dagger gates |
| Dagger v0.21.9 engine gate | `pending` | The host still has a pre-existing long-running Dagger process; it was preserved and not treated as a refresh result |
| Disaggregated KV-transfer compatibility | `A` | Requires coordinated Router sidecar/engine deployment and live P/D acceptance |

## Explicit compatibility holds

- Kubernetes `v0.37.0` is visible as a newer module release, but the selected
  `controller-runtime v0.24.1` compatibility line is Kubernetes `v0.36`; the
  `v0.37` graph is deferred to a coordinated controller-runtime upgrade.
- Helm 4 and the console's major-version candidates are not silently mixed into
  this refresh. ESLint 10, `@kubernetes/client-node` 2, AI SDK major upgrades,
  and the other major frontend migrations require separate API and browser
  acceptance slices.
- The Router release documents a coordinated EPP/sidecar upgrade for KV-transfer
  key changes. The operator does not own that sidecar and therefore leaves the
  disaggregated path explicitly unaccepted.

## Change inventory

- Runtime: Router EPP image digest and `llm-d.ai/v1alpha1` configuration.
- Cluster bootstrap: Router `v0.10.0` CRD release manifest in the default path.
- Authorization: `llm-d.ai` read/list/watch rules retained alongside the
  deprecated GIE policy group for the rollout window.
- Documentation: component inventory, dependency ledger, remediation plan,
  beta acceptance records, and open-loop status updated.
- Historical evidence: the dated KIND acceptance record was not rewritten;
  this refresh has its own dated evidence record.
