# Open Loops — ckodex-kserve-llm-operator

Structured tracker for incomplete work, pending decisions, and tracked deferrals.
Each entry carries an ID, status, priority, and owner note.

Status values: `open` | `in-progress` | `blocked` | `deferred` | `done`
Priority: `P0` (release-blocking) | `P1` (GA-quality) | `P2` (improvement) | `P3` (nice-to-have)

---

## CI/CD

### L-CI-001 — Migrate GHA workflows to `dagger call`

- **Status:** in-progress
- **Priority:** P2
- **Context:** Both workflows migrated (2026-06-14):
  - `ci.yml`: replaced `go run ./ci/main.go` with Dagger hosted fast gate (`dagger call all --source=.`: lint + non-release compile check) and vulnerability scan. Coverage remains available via `dagger call coverage --source=.` outside the hosted fast path.
  - `release.yml` `image-release` job: replaced `go run ./ci/main.go --skip-lint --skip-tests --image ... --push --sign` with `dagger call publish` + `cosign sign` (OIDC) + `dagger call sbom`.
  All `${{ }}` expressions routed through `env:` blocks per GHA injection-safety policy.
- **Reference:** ADR-008, `.github/workflows/ci.yml`, `.github/workflows/release.yml`

### L-CI-002 — Retire `ci/main.go` standalone path

- **Status:** in-progress
- **Priority:** P3
- **Context:** Removed `ci/main.go`, `ci/pkg/`, and the root standalone Dagger
  SDK dependency on 2026-07-04. The typed Dagger Module is now the only CI
  implementation, eliminating duplicate tool pins and policy logic.
- **Reference:** `dagger/module.go`, `dagger.json`, ADR-008

### L-CI-003 — Add `dagger call lula` for OSCAL validation

- **Status:** in-progress
- **Priority:** P2
- **Context:** `dagger/module.go:Lula` added (2026-06-13). Downloads binary,
  verifies checksum via sha256sum, runs `lula validate`, returns assessment file.
- **Reference:** `dagger/module.go`

### L-CI-004 — Coverage gate in module `Test` function

- **Status:** done
- **Priority:** P1
- **Context:** `coverageGateScript()` uses statement-weighted Go coverage and
  enforces an 80% floor for every governed package family.
- **Reference:** `dagger/policy.go`, `dagger/constants.go`

---

## Operator

### L-OP-007 — Remove the Nightly KIND pod-readiness race

- **Status:** in-progress
- **Priority:** P0
- **Context:** The latest old-head Nightly run [31771937532](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/31771937532) failed immediately after applying `llama3-8b` with `no matching resources found`. The pod selector was evaluated before reconciliation created a pod. `local/05-test-inference.sh` now polls for the first matching pod and then waits on that object; the hosted rerun on the fixed head remains required.
- **Reference:** `local/05-test-inference.sh`, `docs/beta/plan.md`, `docs/beta/readiness-ledger.md`

### L-OP-008 — Qualify the Nightly KIND inference probe

- **Status:** in-progress
- **Priority:** P0
- **Context:** The sample HTTPRoute declares `llama3-8b.ckodex.com`, but the old Gateway probe addressed the allocated IP without a matching `Host` header and accepted any parseable `jq` output. The probe now binds the declared hostname, fails on HTTP errors, requires a non-empty `choices` array, retries transient startup failures, and cleans up port-forwarding; hosted runtime evidence remains required.
- **Reference:** `local/05-test-inference.sh`, `local/04-llm-inference-service.yaml`, `internal/gateway/httproute_builder.go`

### L-OP-009 — Make the repo-native image build reproducible

- **Status:** in-progress
- **Priority:** P0
- **Context:** `run/e2e.sh` previously called the legacy Docker builder even though `Dockerfile` uses BuildKit automatic platform arguments; the same invocation transferred a 6.6 GB context because `console/.next` and `console/node_modules` were not excluded. The root `.dockerignore` now bounds the context and Makefile image targets use `docker buildx build --load`; a complete KIND rerun remains required.
- **Reference:** `.dockerignore`, `Dockerfile`, `Makefile`, `run/e2e.sh`

### L-OP-010 — Close the stable v1 admission route

- **Status:** in-progress
- **Priority:** P0
- **Context:** The CRD conversion profile and v1 API types existed before the v1 webhook validator/defaulter was registered. The v1 handler and dedicated cert-manager routes now exist and pass local admission checks; hosted exact-head v1 create/default/convert evidence remains required.
- **Reference:** `internal/webhook/webhook.go`, `internal/webhook/llminferenceservice_v1.go`, `deploy/helm/templates/cert-manager.yaml`

### L-OP-011 — Prove the scheduler dependency and RBAC contract

- **Status:** in-progress
- **Priority:** P0
- **Context:** The scheduler requires Gateway API Inference Extension CRDs and a pre-provisioned shared EPP identity per managed namespace. Helm renders the EPP ServiceAccount/Role/RoleBinding, while the operator only validates and consumes it; fresh hosted-cluster evidence remains required.
- **Reference:** `local/02-prereqs.sh`, `internal/scheduler/epp_manager.go`, `deploy/helm/templates/epp-rbac.yaml`, `deploy/helm/templates/rbac.yaml`

### L-OP-012 — Keep the default CPU proof independent of optional CSI/FUSE

- **Status:** in-progress
- **Priority:** P0
- **Context:** The default sample used `hf-mount://`, which couples the primary CPU proof to a privileged, environment-sensitive CSI/FUSE sidecar. The default is now `hf://` through the signed storage initializer; `hf-mount://` remains an explicit profile with `INSTALL_HF_CSI=1` and needs separate acceptance.
- **Reference:** `local/04-llm-inference-service.yaml`, `local/02-prereqs.sh`, `local/README.md`, `docs/beta/plan.md`

### L-REL-004 — Align packaged chart defaults with the release tag

- **Status:** in-progress
- **Priority:** P0
- **Context:** RC6 was published, but its OCI chart (`sha256:f3794c02e29d27e7d17c80a49d942399c46a1fe60ab8043f0dcd525f04dc4da9`) retained beta8 defaults for the operator and Hugging Face initializer. Empty chart tags now resolve from `Chart.appVersion`, the release workflow passes the leading `v` into the packaged app version, and both release-readiness and hosted packaging render the archive before push.
- **Reference:** `deploy/helm/values.yaml`, `deploy/helm/templates/_helpers.tpl`, `.github/workflows/release.yml`, `hack/helm-contract/main.go`

### L-REL-005 — Pin the disposable KIND node image

- **Status:** in-progress
- **Priority:** P0
- **Context:** The default KIND node image drifted to `v1.36.1`, while the current Docker/KIND environment exposed only a 1,024-file-descriptor node limit; systemd failed before any product resource was installed. The test cluster is now pinned to `kindest/node:v1.35.0` with a 65,536-descriptor preflight; hosted parity and later Kubernetes upgrades require an intentional compatibility run.
- **Reference:** `deploy/kind/kind-config.yaml`, `local/01-kind-setup.sh`, `docs/beta/plan.md`

### L-OP-006 — Validate LMCache live behavior without conflating it with model-weight caching

- **Status:** implementation-complete; live acceptance open
- **Priority:** P1
- **Context:** Typed in-process and upstream `LMCacheEngine` multiprocess configuration are reconciled while the original low-level connector contract remains compatible. A live cache-hit, transfer-tail-latency, image/ABI, and failover run remains required before promotion. `LocalModelCache` continues to own model-weight placement.
- **Reference:** ADR-009, `internal/scheduler/epp_manager.go`

### L-OP-005 — Align LoRA cache ownership with cluster-scoped LocalModelCache

- **Status:** done
- **Priority:** P1
- **Context:** LoRA caches now use collision-resistant cluster names and
  explicit owner annotations instead of invalid namespaced owner references.
  The adapter finalizer deletes its cache before completing deletion, cache
  events map back through owner annotations, and the workload namespace is
  shared by PVC/Job creation and evidence lookup.
- **Reference:** `internal/controller/llmloraadapter_controller.go`,
  `api/v1alpha2/localmodelcache_types.go`

### L-OP-001 — Add `kustomization.yaml` to `config/crd/` and `config/rbac/`

- **Status:** done
- **Priority:** P2
- **Context:** `config/crd/kustomization.yaml` added (2026-06-13): lists all 16 CRDs.
  `config/rbac/kustomization.yaml` added (2026-06-13): lists role.yaml + tenant-role.yaml.
  Both verified: `kustomize build config/crd/` emits 16 CRDs;
  `kustomize build config/rbac/` emits ClusterRole + ClusterRoleBinding + RoleBinding.
- **Reference:** `config/crd/kustomization.yaml`, `config/rbac/kustomization.yaml`

### L-OP-002 — Inline ClusterRole in Helm chart

- **Status:** done
- **Priority:** P2
- **Context:** ClusterRole `ckodex-operator-cluster-role` added to
  `deploy/helm/templates/rbac.yaml` (2026-06-13). Rules mirror `config/rbac/role.yaml`
  exactly. `helm template` verified: ClusterRole + ClusterRoleBinding emitted,
  roleRef name matches ClusterRole name. Note: update Helm template whenever
  `make manifests` regenerates `config/rbac/role.yaml`.
- **Reference:** `deploy/helm/templates/rbac.yaml:6`

### L-OP-003 — AIPack → LLMInferenceService governance integration test

- **Status:** done
- **Priority:** P1
- **Context:** 7 fake-client integration tests added (2026-06-13) in
  `internal/controller/aipack_governance_integration_test.go`. Tests verify:
  binding label filter, fully-attested / missing-predicate / nil-attestation /
  mixed-state paths, and exclusion of unbound/wrong-label AIPacks.
  Also fixed: `ReconcileAIPacks` now always sets `Compliance-SR-2-AIPack`
  condition (previously returned early for empty pack list, leaving condition absent).
- **Reference:** `internal/controller/aipack_governance_integration_test.go`,
  `internal/controller/evidence/aipack_reconciler.go:28`

### L-OP-004 — Fix IPv6 listener in `TestRegisterWithTargetService_Success`

- **Status:** done
- **Priority:** P2
- **Context:** All 5 tests (`TestRegisterWithTargetService_Success`, `TestVaultHealthCheck_Active_200`,
  `TestInferenceFullPipeline`, `TestServerLive_True`, `TestFetchFileChecksums_NonOKStatus_Error`)
  were confirmed passing (2026-06-14) outside the Claude Code sandbox. The failures
  were sandbox-imposed socket restrictions, not code defects. `httptest.NewServer`
  already binds to IPv4 (`127.0.0.1`) — no code change required. CI (linux/amd64)
  has always passed cleanly.
- **Reference:** `internal/controller/llmloraadapter_controller_test.go:385`

---

## Supply Chain

### L-SC-001 — Generate `dagger develop` output in CI and commit `dagger/internal/`

- **Status:** done
- **Priority:** P1
- **Context:** `dagger develop` run successfully (2026-06-14) outside sandbox.
  `dagger/internal/dagger` is excluded by Dagger's generated `.gitignore` — this
  is the canonical Dagger pattern (regenerated per clone). `dagger/go.sum` is
  committed. Both workflows now run `dagger develop` before `dagger call` commands.
- **Reference:** `dagger/go.mod`, `dagger/.gitignore`, `docs/adr/008-dagger-module.md`

### L-SC-002 — Wire `dagger call sbom` into release workflow

- **Status:** done
- **Priority:** P1
- **Context:** `dagger call sbom` wired into `release.yml` `image-release` job (2026-06-14).
  Generates CycloneDX SBOM via Trivy for the published image ref. Output uploaded
  as `sbom/sbom.cdx.json` artifact (90-day retention). Runs after `dagger call publish`
  and `cosign sign`. Depends on L-CI-001 (now done).

### L-SC-003 — Upgrade ORAS after a patched v2 release

- **Status:** done
- **Priority:** P1
- **Context:** `oras-go/v2` is pinned to the patched `v2.6.2` release. OCI pulls
  continue to set `file.Store.SkipUnpack = true`, so registry-controlled
  archives cannot reach the automatic tar extraction path. A module-file test
  binds the containment review to the dependency version.
- **Reference:** `internal/storage/oci_client.go`,
  `internal/storage/storage_extra_test.go`,
  `https://github.com/oras-project/oras-go/security/advisories/GHSA-fxhp-mv3v-67qp`

---

## Documentation

### L-DOC-001 — Fix L|T|R NIST control table (SECURITY_ARCHITECTURE.md)

- **Status:** done
- **Priority:** P1
- **Context:** Replaced CP-2→CA-7, IA-2→IA-9, Trust(T)→SI-7+SR-4. AC-4 anchored to Isolation pillar. (2026-06-15)
- **Reference:** `docs/SECURITY_ARCHITECTURE.md`

### L-DOC-002 — Expand COMPLIANCE.md with IA-9, SR-4, CA-7 rows

- **Status:** done
- **Priority:** P1
- **Context:** Added 3 rows; fixed NIST 800-53r5 canonical control names (SI-7 firmware, SR-2 Plan, SR-4 Provenance); added implementation-status annotation for beta. (2026-06-15)
- **Reference:** `COMPLIANCE.md`

### L-DOC-003 — Add Lula validator for IA-9 (SPIFFE/SPIRE SVID issuance)

- **Status:** done
- **Priority:** P2
- **Context:** Added `spire-identity-validation.yaml` (2026-06-27). The validator
  requires every LLMInferenceService to have exactly one SPIRE registration
  ConfigMap with the expected SPIFFE ID, workload selectors, bounded SVID TTL,
  and DNS SAN.
- **Reference:** `lula/spire-identity-validation.yaml`, `internal/security/spire_registration.go`

### L-DOC-004 — Rewire Mermaid architecture diagram

- **Status:** done
- **Priority:** P1
- **Context:** Added WH→CM, SS-.->V1/V2, CM→PROM, GR→V1/V2, LWS---V1/V2, CON→PROM. Removed subgraph-targeting RT→DP and GR→DP edges. Added neo/elk theme. (2026-06-15)
- **Reference:** `README.md`

### L-DOC-005 — Flesh out OSCAL SI-7 observation with subjects

- **Status:** done
- **Priority:** P1
- **Context:** Added statements, by-components, set-parameters to SI-7 requirement in lula-component.yaml. Fixed relevant-evidence nesting inside subjects entry per OSCAL schema. (2026-06-15)
- **Reference:** `lula/lula-component.yaml`

---

## API surface

### L-API-001 — Wire the CRD conversion webhook

- **Status:** in-progress
- **Priority:** P0
- **Context:** The CRD conversion profile, CA injection, and v1/v1alpha2
  conversion handlers are implemented and pass local round-trip/admission
  checks. Hosted exact-head create/default/convert evidence remains open and is
  also tracked by L-OP-010.
- **Reference:** [remediation-plan.md](remediation-plan.md) items 1.1–1.4

### L-API-002 — Complete conversion for all divergent fields

- **Status:** done
- **Priority:** P0
- **Context:** All eight divergent fields are mapped through the v1
  experimental/stable surfaces. Unit, round-trip, and fuzz tests cover the
  conversion path; hosted storage migration remains part of L-API-001.
- **Reference:** `api/v1alpha2/conversion.go`

### L-API-003 — Register per-version webhook paths

- **Status:** done
- **Priority:** P1
- **Context:** Dedicated v1 validator/defaulter handlers and cert-manager routes
  are registered, with shared validation exercised by local admission tests.
- **Reference:** `deploy/helm/templates/cert-manager.yaml`

## Runtime

### L-RT-001 — Introduce the engine adapter seam

- **Status:** in-progress
- **Priority:** P0
- **Context:** `internal/runtime` now defines the adapter contract and a total
  vLLM capability matrix. The single-node builder renders governed
  parallelism, cache, speculative-decoding, and quantization arguments and
  rejects unsupported engines before reconciliation. Metrics, receipts,
  images, health, and additional engines remain outside the seam.
- **Reference:** [ADR-010](adr/010-runtime-engine-contract.md),
  [engine-contract.md](engine-contract.md)

### L-RT-002 — Remove the unreferenced LWS reconciler

- **Status:** in-progress
- **Priority:** P1
- **Context:** The advanced flags have moved into the vLLM adapter and are
  covered on the single-node path. The now-redundant LWS reconciler and its
  remaining compatibility tests still need removal; multi-node continues to
  delegate to KServe `workerSpec` per ADR-009.
- **Reference:** `internal/kserve/multinode.go`

### L-RT-003 — Replace the unresolvable GGUF runtime image

- **Status:** in-progress
- **Priority:** P1
- **Context:** `ckodex/quant-cpp:v0.1.0` is the declared GGUF runtime. It is not
  built in this repository — absent from `Dockerfile`, `.goreleaser.yaml`,
  `build/`, and `Makefile` — it is tag-pinned rather than digest-pinned unlike
  every other component, and its registry tag API returns 404. Repoint at a
  digest-pinned upstream `llama.cpp` server image.
- **Reference:** `internal/controller/api/constants.go`, `COMPONENTS.md`

### L-RT-004 — Add SGLang and llama.cpp adapters

- **Status:** open
- **Priority:** P2
- **Context:** The Gateway API Inference Extension already ships built-in
  EndpointPicker metric specifications for SGLang, selected by a pod engine
  label. llm-d v0.9.0 ships SGLang and TensorRT-LLM images. Blocked on L-RT-001.
- **Reference:** [engine-contract.md](engine-contract.md)

## Observability and evidence

### L-OBS-001 — Audit writes fail silently

- **Status:** in-progress
- **Priority:** P0
- **Context:** Kubernetes Event creation failures are now logged with action and
  resource identity, events use the audited namespace/involved object, and a
  configured but unavailable direct OTLP export reports an explicit error.
  Profile-specific graded failure semantics remain open.
- **Reference:** [ADR-011](adr/011-canonical-observability-planes.md) item 4.10

### L-OBS-002 — No integrity primitives on the evidence path

- **Status:** open
- **Priority:** P0
- **Context:** There is no signature, hash chain, sequence number, or Merkle
  commitment anywhere on the audit path. Producer identity is nearly free —
  SPIRE already issues workload SVIDs.
- **Reference:** [ADR-011](adr/011-canonical-observability-planes.md) item 4.5

### L-OBS-003 — Receipt types are content-bearing

- **Status:** open
- **Priority:** P1
- **Context:** `ContentMessage` and `ContentPart` carry raw text and base64
  payloads by construction. CKC-OBS §7 requires
  `content -> canonicalize -> hash/commit -> reference`. Redaction after the
  fact is not minimization; the type must not be able to hold the content.
- **Reference:** `internal/observability/ois_v01.go`

### L-OBS-004 — Metric and event namespace drift

- **Status:** open
- **Priority:** P2
- **Context:** Four Prometheus prefixes are in use — `ckodex_lmc_*`,
  `ckodex_governance_*`, `ckodex_inference_*`, `ckodex_resilience_*` — and
  events split between `ckodex.infer.*` and `ckodex.inference.*` for the same
  concept. CKC-OBS §9 requires `exp.*` / `infer.*` / `ops.*`.
- **Reference:** `internal/observability/telemetry.go`

### L-OBS-005 — Missing evidence is not detectable

- **Status:** in-progress
- **Priority:** P1
- **Context:** A deterministic fail-closed evidence-health tracker detects
  missing, unexpected, duplicate, unsigned, and out-of-sequence receipts. It is
  not yet wired to the runtime evidence plane and does not model broken causal
  edges.
- **Reference:** [ADR-011](adr/011-canonical-observability-planes.md) item 4.9

## Admission

### L-ADM-001 — Unknown onboarding stage types pass

- **Status:** done
- **Priority:** P1
- **Context:** The stage dispatch rejects unrecognised stage types and the
  regression test proves the fail-closed path.
- **Reference:** `internal/controller/modelonboarding_controller.go`

### L-ADM-002 — Build the two-plane model admission controller

- **Status:** open
- **Priority:** P2
- **Context:** Reaching a terminal onboarding phase changes status but does not
  gate serving. Blocked on L-RT-001 for the capability hook and L-PACK-001 for
  the provenance hook.
- **Reference:** [ADR-012](adr/012-model-admission-planes.md)

## AIPack

### L-PACK-001 — Attestation verification does not verify

- **Status:** in-progress
- **Priority:** P0
- **Context:** Predicate presence no longer produces a positive cryptographic
  verdict: verification stays false until cosign integration exists. Cosign,
  Rekor inclusion, digest binding, payload-schema validation, and a TTL cache
  remain open.
- **Reference:** `internal/governance/aipack_evidence.go`

### L-PACK-002 — Unimplemented validators fail open

- **Status:** in-progress
- **Priority:** P0
- **Context:** Pattern, VAD, lineage, and air-gap validators now return explicit
  fail-closed implementation errors; policy evaluation enforces families,
  artifact kinds, required predicates, and risk bands. `InferPattern` and
  `ManifoldDistance` remain sentinels and the full admission wiring is open.
- **Reference:** `internal/aipack/`

### L-PACK-003 — Implement AIPACK-SPEC §§11–22

- **Status:** open
- **Priority:** P2
- **Context:** Roughly 25 `TODO(ckodex)` stubs across lineage, blast radius,
  risk valence signals, outlier detection, TEA, deprecation, air gap, patterns,
  policy, quarantine triggers, and VAD. Parallelizable by section after
  L-PACK-002.
- **Reference:** [remediation-plan.md](remediation-plan.md) Phase 6 table

### L-PACK-004 — Enforce the shipped JSON Schemas

- **Status:** open
- **Priority:** P2
- **Context:** Twenty-five schemas exist under `schema/`, `schema/ext/`, and
  `schema/predicates/`. Nothing validates a custom resource against them.
- **Reference:** `schema/`

## Operations

### L-OP-013 — Consolidate the two Helm chart sources

- **Status:** open
- **Priority:** P2
- **Context:** Both chart install contracts now derive release images from each
  chart's `appVersion`, and the legacy initializer beta.7 drift is fixed.
  `deploy/helm/` remains authoritative and full source consolidation is open.
- **Reference:** `hack/helm-contract/main.go`

### L-OP-014 — Scheduler config still on the pre-GA API version

- **Status:** open
- **Priority:** P2
- **Context:** `internal/scheduler/config.go` emits
  `inference.networking.x-k8s.io/v1alpha1` for `EndpointPickerConfig` while the
  `InferencePool` it feeds is already GA `inference.networking.k8s.io/v1`.
- **Reference:** `internal/scheduler/config.go`

### L-OP-015 — Restrict dynamic RBAC mutation authority

- **Status:** in-progress
- **Priority:** P1
- **Context:** The EPP manager no longer creates Roles, RoleBindings, or EPP
  ServiceAccounts. Helm pre-provisions one shared identity per managed
  namespace; manager cache scoping and hosted proof remain open.
- **Reference:** `deploy/helm/templates/epp-rbac.yaml`, `internal/scheduler/epp_manager.go`

## Toolchain and CI

### L-CI-005 — Prove the hosted unit-test gate

- **Status:** in-progress
- **Priority:** P0
- **Context:** `ci.yml` invokes the race-enabled Dagger test gate. Coverage is
  calculated from covered statements rather than averaged function
  percentages, and every governed package family has an 80% floor. Local
  exact-head evidence is green; hosted exact-head evidence remains open.
- **Reference:** `.github/workflows/ci.yml`, `dagger/policy.go`

### L-CI-006 — Lint excludes tests and the injection rule families

- **Status:** done
- **Priority:** P1
- **Context:** `.golangci.yml` enables test analysis and the required gosec
  families. Full configured lint runs in Dagger and release verification.
- **Reference:** `.golangci.yml`

### L-CI-007 — Exact-patch Go directive forces a toolchain download

- **Status:** done
- **Priority:** P2
- **Context:** `go.mod` uses the `go 1.26` language directive plus an explicit
  `toolchain go1.26.6` pin.
- **Reference:** `go.mod`

### L-CI-008 — No surface-conformance gate

- **Status:** done
- **Priority:** P1
- **Context:** Direct fields, generated OpenAPI properties, and primary LLM
  nested local fields are structurally pinned; behavior mapping and API-server
  conversion evidence remain open beyond LLM.
- **Evidence:** `test/conformance/crd_schema_contract_test.go`, `internal/validation/crd_surface_inventory_test.go`
- **Reference:** `internal/validation/surface.go`, `surface_test.go`, [remediation-plan.md](remediation-plan.md) items 0.5–0.6
