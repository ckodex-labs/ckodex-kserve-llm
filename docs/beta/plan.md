# Beta product assessment and execution plan

**Scope:** `ckodex-kserve-llm-operator` and its observe-only operator console
**Assessment date:** 2026-08-14
**Target:** controlled beta for platform teams, not general availability

## 1. Beta outcome

The beta is successful when a platform operator can install the published release,
authenticate to the console, declare a supported CPU or GPU inference workload,
understand its observed state, investigate a failure, and recover or escalate it
without the product implying authority or evidence it does not possess.

The beta is not a promise of automated promotion, model-agent tool execution,
public multi-tenancy, LMCache performance, cryptographic verification, or every
specialized inference CRD. Those capabilities remain excluded or separately gated
until their acceptance evidence exists.

## 2. Assessment method

The assessment used four views:

1. **Product contract:** README, overview, API policy, runbooks, samples, beta
   matrix, and release metadata were compared for contradictory promises.
2. **Implementation path:** API types, conversion methods, controllers, webhook
   registration, Helm resources, console data adapters, and CI/release wiring were
   traced to their actual consumers.
3. **Artifact path:** a fresh-checkout source layout, generated CRDs, chart renders,
   GoReleaser rehearsal, console standalone image, SBOM/provenance hooks, and
   vulnerability gates were inspected.
4. **Experience proof:** unit, race, integration, conformance, SSR, populated-state,
   accessibility-contract, packaged-image browser, and image-scan checks were run
   locally. The packaged image was opened at desktop and 390px mobile widths;
   CSS delivery, heading order, target sizes, overflow, command-palette focus and
   escape return, theme switching, and browser warning/error logs were checked.
   Cluster, authenticated-ingress, assistive-technology, GPU, hosted-release, and
   human-approval evidence remains explicitly separate.
5. **Hosted signal:** main-head Nightly run
   ([33081386331](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33081386331))
   failed before the API server started because KIND v0.32/Kubernetes 1.35 used
   kubeadm v1beta3 while the committed etcd patch used the v1beta4 list shape.
   The local contract aligns KIND v0.33.0 with a digest-pinned Kubernetes v1.36.4
   image and one shared node-image source; hosted acceptance remains pending.

## 3. Findings and disposition

| ID | Finding | User impact | Disposition | Beta gate |
| --- | --- | --- | --- | --- |
| BETA-P0-001 | The console was not reliably part of the operator checkout/package boundary. | A published chart could render a console that a release artifact did not contain. | Fixed locally: console source is ordinary tracked source, standalone build is wired, and release checks require it. | Hosted fresh-checkout and tagged-release proof pending |
| BETA-P0-002 | The two chart trees can drift. | Operators may validate one chart and publish another. | Partially contained: release path is `deploy/helm`; root chart remains an explicit open loop. | Retire/consolidate duplicate chart or document a single authoritative publish path |
| BETA-P0-003 | Stable v1 and served v1alpha2 had no CRD conversion stanza. | v1 create/read/update could fail or behave differently from the documented migration path. | Fixed locally: beta Kustomize profile and checksummed bundle bind `/convert` to the fixed webhook identity. | Live cluster conversion acceptance pending |
| BETA-P0-004 | Existing conversion code dropped alpha2-only fields and status. | An old resource could round-trip through v1 with silent behavior loss. | Fixed locally: v1 experimental fields and lossless typed mappings plus round-trip tests. | Live conversion and storage-version proof pending |
| BETA-P0-005 | Authentication and human attribution are not a completed product contract. | Operators cannot yet rely on the console boundary for attributable access. | Open; choose and test OIDC/authenticated ingress before promotion. | Human-access gate |
| BETA-P0-006 | Runtime proof is not a single supported profile. | CPU, GPU, KServe, Gateway, storage, and optional systems can be mistaken for one validated path. | Open; define one CPU profile and one GPU profile with explicit dependencies and exclusions. | Runtime acceptance gate |
| BETA-P0-007 | Nightly KIND acceptance waited on a pod selector before reconciliation created a pod. | The hosted test exited with `no matching resources found` before workload readiness could be evaluated, producing a false-negative runtime signal. | Fixed locally: `local/05-test-inference.sh` now polls for the first matching pod before waiting for readiness and reports the selector on timeout. The latest old-head evidence is [run 31771937532](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/31771937532). | Hosted nightly rerun on the fixed head |
| BETA-P0-008 | The runtime probe did not bind the Gateway hostname or strictly qualify the inference response. | A healthy route could be reported as failed because the request Host did not match the HTTPRoute, while a non-JSON or error response could be treated as a successful probe. | Fixed locally: the probe sends the declared hostname, fails on HTTP errors, retries transient startup failures, validates `choices`, and cleans up port-forwarding. | Hosted runtime rerun on the fixed head |
| BETA-P0-009 | The repo-native operator build used the legacy Docker builder while the Dockerfile requires BuildKit platform arguments. | KIND setup could fail before image loading with an empty platform error, and generated console artifacts could inflate the context to multiple gigabytes. | Fixed locally: root `.dockerignore` excludes generated frontend/build output and Makefile image targets use `docker buildx build --load`. | Local KIND rerun and hosted build verification |
| BETA-P0-010 | The stable v1 CRD advertised conversion and defaulting, but the webhook registration only handled v1alpha2. | A v1 declaration could reach the API server without the validator/defaulting behavior used by the supported profile. | Fixed locally: v1 validator/defaulter registration, dedicated cert-manager webhook routes, and v1 admission tests are present. | Hosted exact-head v1 admission and conversion rerun |
| BETA-P0-011 | The Helm RBAC contract omitted resources required by the EPP scheduler and controller reconciliation. | A chart install could appear healthy while scheduler or workload reconciliation failed with authorization errors. | Fixed locally: Helm pre-provisions one shared, namespace-scoped EPP ServiceAccount/Role/RoleBinding per managed namespace; the operator validates and consumes it without dynamic RBAC mutation. | Hosted exact-head chart install and runtime rerun |
| BETA-P0-012 | The local prerequisite script did not install the external Gateway API Inference Extension and llm-d Router CRDs required by the EPP path. | The scheduler could not create or observe InferencePool or Router policy resources in a fresh cluster. | Fixed locally: the official GIE v1.5.0 and llm-d Router v0.10.0 manifests are installed by the default prerequisite path. | Hosted fresh-cluster prerequisite and runtime rerun |
| BETA-P0-013 | The default CPU journey depended on the optional privileged Hugging Face CSI/FUSE sidecar path. | A supported-looking sample could stall in model initialization for environment-specific CSI reasons before inference was tested. | Fixed locally: the default sample uses the signed `hf://` storage-initializer path; `hf-mount://` remains an explicit opt-in profile requiring `INSTALL_HF_CSI=1`. | Rerun the default CPU profile; keep CSI/FUSE acceptance separate |
| BETA-P0-014 | RC6 chart metadata was `0.18.0-rc.6` while default operator and initializer values still referenced `v0.18.0-beta.8`. | Installing the RC chart without overrides could pull a different release unit than the chart and release notes describe. | Fixed locally: empty image tags resolve from `Chart.appVersion`, the release workflow preserves the leading `v` in packaged `appVersion`, and both local and hosted release paths render the packaged chart before push. | Hosted package render and anonymous artifact contract on the next RC |
| BETA-P0-015 | The local and hosted KIND profiles selected different node images, and the host exposed only a 1,024-file-descriptor node limit. The hosted mismatch paired kubeadm v1beta3 with a v1beta4-shaped patch. | The default fresh-cluster proof could fail before cert-manager, CRDs, or the operator were evaluated and provide no actionable diagnosis. | Fixed locally: one digest-pinned `kindest/node:v1.36.4` source feeds Make, local setup, and Nightly; the v1beta4 patch keeps list-shaped arguments; local setup fails preflight below 65,536 descriptors. | Fresh KIND rerun on a host meeting the preflight plus hosted exact-head rerun |
| BETA-P1-001 | Readiness can be observed, but causal qualification is not proven across live dependencies. | “Ready”, “partial”, “stalled”, and “unavailable” can still require operator inference. | Partially fixed in console contracts; add live dependency-failure journeys and evidence correlation. | Qualified-readiness gate |
| BETA-P1-002 | Browser proof must cover the packaged image, not only SSR/static contracts. | A direct standalone launch omitted the runtime static-asset copy and rendered unstyled HTML; the packaged image was then verified at desktop and mobile widths. Assistive-technology, reduced-motion, and forced-colors behavior are not fully proven. | Fixed locally: packaged browser pass confirms CSS delivery, valid heading order, 44px targets, no mobile overflow, command-palette focus/escape return, theme switching, and no browser warnings/errors. Open: hosted browser gate plus manual/assistive-technology evidence. | Browser-accessibility gate |
| BETA-P1-003 | Production dependency audit is clear, but the full development tree still reports advisories. | Release risk can be misread if production and development findings are conflated. | Production audit fixed locally with npm overrides; keep dev findings visible and bound to build tooling. | Hosted npm audit and image scan |
| BETA-P1-004 | Provenance, SBOM, signature, and vulnerability scan are wired but hosted artifact acceptance is not complete. | Local green status does not prove the public release is signed, discoverable, and exact-head aligned. | Open until tag workflow, registry digests, attestations, and public contract are inspected. | Hosted release gate |
| BETA-P1-005 | LMCache, GPU, multi-node, and external storage integrations have code paths but not beta-grade live evidence. | Feature presence may be mistaken for supported runtime behavior. | Keep `S` or explicitly exclude each capability; do not promote from unit tests. | Runtime capability gates |
| BETA-P1-006 | Cryptographic evidence language is stronger than the currently demonstrated verification path. | UI or docs could imply attestation validity from metadata alone. | Keep evidence claims qualified until cosign verification and UI binding are demonstrated. | Evidence-verification gate |
| BETA-P2-001 | Agent/SkillRegistry runtime execution is outside the tested console/operator beta. | Users may infer governed tool execution from resource schemas. | Explicitly exclude from beta and keep mutation authority absent. | Product boundary |
| BETA-P2-002 | Promotion and rollback semantics are represented in the repository but not accepted as a live, signed workflow. | “Ready” could be read as “approved to promote”. | Keep console observe-only and promotion excluded. | Product boundary |

## 4. Execution sequence

### Phase 0 — Freeze the beta contract

**Goal:** remove ambiguity before more feature work.

Deliverables:

- acceptance matrix and readiness ledger remain the release review source;
- every public capability is labelled `C`, `S`, or `A`;
- supported profiles, required dependencies, data boundary, authentication posture,
  retention, and excluded capabilities are written down;
- release owner, runtime owner, security owner, and console owner are named for
  every non-green gate.

Exit criteria:

- no README, chart, sample, or console label claims production, promotion,
  cryptographic validity, or runtime support without an evidence reference;
- beta approval cannot be inferred from local tests alone.

### Phase 1 — Make the release unit reproducible

**Goal:** a fresh checkout and published chart produce the same operator/console
release unit.

Deliverables:

- keep `console/` ordinary tracked source with lockfile and standalone Dockerfile;
- run console install, production audit, tests, lint, typecheck, build, SSR, and
  populated-state checks in CI;
- build, scan, sign, attest, and publish operator and console images with immutable
  digests;
- upload the checksummed beta CRD bundle and chart from the same tag;
- make `deploy/helm` the only publish source, or retire the duplicate root chart;
- add exact-tag public release contract checks for image names, digests, chart,
  CRDs, SBOM, provenance, signature, and scan results.

Exit criteria:

- a fresh checkout passes the complete local gate;
- hosted CI passes on the exact tag head;
- the public chart renders the console and beta webhook profile;
- registry artifacts are discoverable by digest and match the release metadata.

### Phase 2 — Close the stable API boundary

**Goal:** v1 is a real storage and migration contract, not only a Go package.

Deliverables:

- apply the beta CRD profile with `strategy: Webhook`, conversion review version,
  fixed Service identity, `/convert`, and cert-manager CA injection;
- install the chart with the fixed beta release name/namespace/fullname override;
- preserve every alpha2 field and status value through v1 hub conversion;
- test v1 declaration, v1alpha2 declaration, conversion round-trip, defaulting,
  validation, status update, and storage-version behavior;
- record an API server event/log trace showing conversion webhook calls succeed;
- update migration docs with the exact install profile and rollback procedure.

Exit criteria:

- live cluster can create in v1, read in v1alpha2, update in v1, and read back
  without field loss;
- `v1` is storage and `v1alpha2` is served but non-storage;
- webhook certificate, CA bundle, Service, and controller endpoint are healthy;
- the test fails if the Helm identity or CRD patch drifts.

### Phase 3 — Prove one runtime profile end to end

**Goal:** establish a narrow, supportable inference journey.

CPU profile first (default `hf://` initializer path):

- install the chart and beta CRDs;
- declare a small public or local model through a supported storage scheme;
- use the signed Hugging Face storage-initializer path by default; treat
  `hf-mount://` plus the privileged CSI/FUSE driver as a separate opt-in profile;
- verify admission, reconciliation, Deployment, Service/Gateway, readiness, and
  OpenAI-compatible inference;
- make the acceptance harness wait for the controller-created workload object before
  evaluating pod readiness, so a scheduling or reconciliation delay is not reported
  as an immediate resource lookup failure;
- send the Gateway probe with the declared HTTPRoute hostname and require a successful
  JSON completion response; direct port-forward fallback must be cleaned up on exit;
- build the operator and storage-initializer images through the BuildKit path with a
  bounded source context, then load the exact image tags into KIND;
- restart the operator and workload; verify recovery and status continuity;
- test invalid model URI, missing dependency, failed readiness, and deletion;
- capture events, conditions, logs, request result, and recovery timestamps.

GPU profile second:

- prove node labels, device plugin, runtime class, GPU requests/limits, model image,
  scheduling, model load, inference, restart, and recovery on real hardware;
- measure cold start, model load, steady-state latency, throughput, and failure
  behavior using a fixed workload and environment fingerprint;
- keep all performance claims tied to artifacts and a baseline.

Exit criteria:

- at least one profile is beta-supported with complete evidence;
- GPU, multi-node, LMCache, and external-secret paths are either separately green
  or explicitly excluded from the beta support table;
- the console renders the same observed state as the API and event evidence.

### Phase 4 — Finish the operator experience

**Goal:** an operator can understand and act within the product’s authority.

Deliverables:

- authenticated ingress with attributable identity and namespace/tenant boundary;
- readiness explanation journeys for healthy, stalled, partial, unavailable,
  dependency-failed, stale, and permission-denied states;
- investigation links from workload to events, telemetry, identity, source, and
  relevant runbook without inventing severity or causal certainty;
- explicit unavailable/permission/stale states rather than empty success states;
- browser tests for keyboard traversal, focus restoration, skip link, reduced motion,
  forced colors, high contrast, screen reader landmarks, responsive tables, and
  error recovery;
- retain observe-only behavior: no console mutation endpoints, promotion buttons,
  delete controls, or hidden cluster tools.

Exit criteria:

- human operator test completes the supported journeys without guessing whether a
  value is observed, inferred, claimed, attested, contradicted, or unavailable;
- a permission failure is distinguishable from an empty cluster;
- every assertion has a source or qualification visible at the point of use.

### Phase 5 — Security, evidence, and operations

**Goal:** security and release evidence are operational controls.

Deliverables:

- production dependency audit and container scan remain release-blocking;
- operator and console images publish SBOM, provenance, signature, and immutable
  digest metadata;
- verify signatures and attestations in a clean environment, including failure
  cases for wrong digest, missing attestation, and untrusted identity;
- run API, RBAC, network-egress, secret-handling, SSRF, URI, webhook, and tenant
  boundary tests against the supported profiles;
- define incident, rollback, certificate rotation, upgrade, backup/restore, and
  CRD conversion recovery runbooks;
- retain evidence artifacts with release ID, commit SHA, image digest, cluster
  version, runtime version, test command, timestamp, and operator identity.

Exit criteria:

- a reviewer can reconstruct the exact artifact and test environment from the
  release record;
- security failures fail closed and are visible in the ledger;
- rollback and certificate rotation have been rehearsed on the beta profile.

### Phase 6 — Beta decision and controlled rollout

**Goal:** release only what the evidence supports.

Decision package:

- green acceptance matrix and signed readiness ledger;
- exact-tag hosted CI and release verification links;
- runtime evidence for each supported profile;
- browser/accessibility report;
- security/provenance verification report;
- known-limitations and exclusion list;
- explicit human approver and rollback owner.

Rollout:

1. publish a release candidate from the exact verified head;
2. install into one controlled environment;
3. observe a fixed soak window with no unqualified claims;
4. expand only after error, readiness, latency, recovery, and security thresholds
   remain within the approved profile;
5. keep promotion/agent execution disabled unless separately approved.

## 5. Local versus external gates

| Gate | Local evidence in this checkout | Still required before beta promotion |
| --- | --- | --- |
| Source/package | Console tracked, lockfile, standalone build, release-readiness rehearsal | Hosted fresh-checkout and exact-tag proof |
| Go/API | Full Go tests, vet, race conformance, lossless conversion test, generated CRDs | Live API server conversion/storage test |
| Helm/CRDs | Both chart lint paths, Helm contract, beta Kustomize render, checksummed CRD bundle | Published OCI chart install with cert-manager |
| Console | 43 tests, lint, typecheck, build, SSR, populated-state, packaged-image browser pass | Hosted browser/Playwright/axe/manual AT evidence; reduced-motion and forced-colors confirmation |
| Security | Production npm audit zero, operator Dagger scan zero, console image Trivy zero H/C | Hosted scan, SBOM/provenance/signature verification |
| Runtime | Dagger lint and local Go/runtime gates pass; the full Dagger build timed out in its isolated build stage after 7m44s, the isolated Dagger test was stopped after 10m26s without a result, and the fresh KIND rerun is blocked on this host because Docker reports `systemd: Too many open files` while booting a new node | Re-run the fixed Dagger build/test/E2E harness on a host with sufficient Docker resources or hosted KIND, then execute the CPU profile and real GPU profile if supported |
| Identity | Static adapters and identity qualification | Authenticated ingress, human attribution, boundary tests |
| Promotion | Observe-only UI and explicit exclusion | No beta promotion gate; separate future package |

## 6. Definition of beta done

The beta is done only when all of these are true:

- required evidence in `acceptance-matrix.yaml` is green;
- no `S` gate is being represented as `C` through copy, UI color, chart defaults,
  or release metadata;
- a live supported profile has passed declaration, readiness, inference, restart,
  failure, and recovery tests;
- v1 conversion has passed against a real API server;
- access is authenticated and attributable;
- the public chart and images are exact-head artifacts with verifiable provenance;
- browser accessibility and the known-limitations report are complete;
- a human approver accepts the remaining risks and owns rollback.
